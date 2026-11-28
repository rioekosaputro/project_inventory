package usecase

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/storage"
)

const podDistanceWarningMeters = 500.0

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// TrackingUsecase melayani seluruh aksi driver mobile & riwayat rute pengiriman (T3.3).
type TrackingUsecase struct {
	tx       domain.TxManager
	tracking domain.TrackingRepository
	lpb      domain.LPBRepository
	stock    domain.StockRepository
	pod      domain.DeliveryPODRepository
	audit    domain.AuditRepository
	clock    clock.Clock
	storage  storage.Uploader
}

func NewTrackingUsecase(
	tx domain.TxManager,
	tracking domain.TrackingRepository,
	lpb domain.LPBRepository,
	stock domain.StockRepository,
	pod domain.DeliveryPODRepository,
	audit domain.AuditRepository,
	clk clock.Clock,
) *TrackingUsecase {
	return &TrackingUsecase{
		tx:       tx,
		tracking: tracking,
		lpb:      lpb,
		stock:    stock,
		pod:      pod,
		audit:    audit,
		clock:    clk,
	}
}

func (u *TrackingUsecase) WithStorage(st storage.Uploader) *TrackingUsecase {
	u.storage = st
	return u
}

type CheckpointInput struct {
	Status        string
	Latitude      float64
	Longitude     float64
	AccuracyM     *int
	ClientTime    *time.Time
	ClientEventID *string
	Notes         *string
}

type DriverDeliverInput struct {
	ReceiverName  string
	PODPhotoPath  string
	Latitude      float64
	Longitude     float64
	AccuracyM     *int
	ClientTime    *time.Time
	ClientEventID *string
	Notes         *string
}

type DriverDeliverResult struct {
	Log             *domain.ShipmentTrackingLog
	Idempotent      bool
	DistanceMeters  *float64
	DistanceWarning *string
}

type DriverFailInput struct {
	Reason        string
	Latitude      float64
	Longitude     float64
	AccuracyM     *int
	ClientTime    *time.Time
	ClientEventID *string
	Notes         *string
}

type DriverPickupInput struct {
	Latitude      float64
	Longitude     float64
	AccuracyM     *int
	ClientTime    *time.Time
	ClientEventID *string
	Notes         *string
}

type TrackingRouteResult struct {
	LPBID          int64
	LPBNumber      string
	DeliveryStatus *string
	ReceiverName   *string
	DeliveredAt    *time.Time
	PODPhotoURL    *string
	Logs           []domain.ShipmentTrackingLog
}

// MyDeliveries mengembalikan daftar tugas pengiriman & penjemputan aktif milik driver.
func (u *TrackingUsecase) MyDeliveries(ctx context.Context, actor domain.Principal) ([]domain.DriverDeliveryTask, error) {
	return u.tracking.ListMyDeliveries(ctx, actor.UserID)
}

// RecordCheckpoint mencatat checkpoint GPS (PICKED_UP / IN_TRANSIT / ARRIVED) secara idempoten.
func (u *TrackingUsecase) RecordCheckpoint(ctx context.Context, actor domain.Principal, lpbID int64, in CheckpointInput) (*domain.ShipmentTrackingLog, bool, error) {
	st := domain.TrackingStatus(strings.ToUpper(strings.TrimSpace(in.Status)))
	var errs []domain.FieldError
	if !st.ValidCheckpoint() {
		errs = append(errs, domain.FieldError{Field: "status", Message: "status checkpoint harus PICKED_UP, IN_TRANSIT, atau ARRIVED"})
	}
	errs = append(errs, validateCoords(in.Latitude, in.Longitude)...)
	eventID, evErr := validateClientEventID(in.ClientEventID)
	if evErr != nil {
		errs = append(errs, *evErr)
	}
	if len(errs) > 0 {
		return nil, false, &domain.ValidationError{Message: "Data checkpoint tidak valid", Errors: errs}
	}

	if eventID != nil {
		if existing, err := u.tracking.FindByClientEventID(ctx, *eventID); err != nil {
			return nil, false, err
		} else if existing != nil {
			if existing.LPBID != lpbID || (actor.Role != domain.RoleAdmin && existing.DriverID != actor.UserID) {
				return nil, false, domain.ErrForbidden
			}
			return existing, true, nil
		}
	}

	now := u.clock.Now()
	logEntry := &domain.ShipmentTrackingLog{
		LPBID:         lpbID,
		DriverID:      actor.UserID,
		Status:        st,
		Latitude:      in.Latitude,
		Longitude:     in.Longitude,
		AccuracyM:     in.AccuracyM,
		RecordedAt:    now,
		ClientTime:    in.ClientTime,
		ClientEventID: eventID,
		Notes:         cleanOpt(in.Notes),
	}

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		l, err := u.lpb.FindByIDForUpdate(ctx, lpbID)
		if err != nil {
			return err
		}
		if err := ensureDriverAssigned(actor, l); err != nil {
			return err
		}
		if l.Status != domain.LPBApproved || !l.RequiresDelivery {
			return fmt.Errorf("%w: checkpoint hanya untuk LPB APPROVED yang memerlukan pengiriman", domain.ErrInvalidStateTransition)
		}
		if isTerminalDeliveryStatus(l.DeliveryStatus) {
			return fmt.Errorf("%w: pengiriman LPB ini sudah selesai (%s)", domain.ErrInvalidStateTransition, *l.DeliveryStatus)
		}
		if err := u.tracking.Insert(ctx, logEntry); err != nil {
			return err
		}
		return u.tracking.UpdateLPBDeliveryStatus(ctx, l.ID, st)
	})
	if err != nil {
		return nil, false, err
	}
	return logEntry, false, nil
}

// Deliver menandai pengiriman DELIVERED oleh driver (foto POD + nama penerima + GPS, idempoten client_event_id).
func (u *TrackingUsecase) Deliver(ctx context.Context, actor domain.Principal, lpbID int64, in DriverDeliverInput) (*DriverDeliverResult, error) {
	receiver := strings.TrimSpace(in.ReceiverName)
	photoPath := strings.TrimSpace(in.PODPhotoPath)
	var errs []domain.FieldError
	if n := utf8.RuneCountInString(receiver); n < 2 || n > 100 {
		errs = append(errs, domain.FieldError{Field: "receiver_name", Message: "nama penerima wajib diisi (2–100 karakter)"})
	}
	if photoPath == "" {
		errs = append(errs, domain.FieldError{Field: "pod_photo", Message: "foto bukti serah terima (POD) wajib diunggah"})
	}
	errs = append(errs, validateCoords(in.Latitude, in.Longitude)...)
	eventID, evErr := validateClientEventID(in.ClientEventID)
	if evErr != nil {
		errs = append(errs, *evErr)
	}
	if len(errs) > 0 {
		return nil, &domain.ValidationError{Message: "Data bukti pengiriman (POD) tidak valid", Errors: errs}
	}

	if eventID != nil {
		if existing, err := u.tracking.FindByClientEventID(ctx, *eventID); err != nil {
			return nil, err
		} else if existing != nil {
			if existing.LPBID != lpbID || (actor.Role != domain.RoleAdmin && existing.DriverID != actor.UserID) {
				return nil, domain.ErrForbidden
			}
			return &DriverDeliverResult{Log: existing, Idempotent: true}, nil
		}
	}

	now := u.clock.Now()
	deliveredDate := clock.DateWIB(now)
	logEntry := &domain.ShipmentTrackingLog{
		LPBID:         lpbID,
		DriverID:      actor.UserID,
		Status:        domain.TrackingDelivered,
		Latitude:      in.Latitude,
		Longitude:     in.Longitude,
		AccuracyM:     in.AccuracyM,
		RecordedAt:    now,
		ClientTime:    in.ClientTime,
		ClientEventID: eventID,
		ReceiverName:  &receiver,
		PhotoPath:     &photoPath,
		Notes:         cleanOpt(in.Notes),
	}

	var distMeters *float64
	var distWarn *string

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		l, err := u.lpb.FindByIDForUpdate(ctx, lpbID)
		if err != nil {
			return err
		}
		if err := ensureDriverAssigned(actor, l); err != nil {
			return err
		}
		if l.Status != domain.LPBApproved {
			return fmt.Errorf("%w: hanya LPB APPROVED yang dapat ditandai terkirim", domain.ErrInvalidStateTransition)
		}
		if l.SubType != domain.LPBRentalDelivery && l.SubType != domain.LPBConsumableDelivery {
			return fmt.Errorf("%w: POD hanya berlaku untuk LPB pengiriman", domain.ErrInvalidStateTransition)
		}
		if isTerminalDeliveryStatus(l.DeliveryStatus) {
			return fmt.Errorf("%w: pengiriman LPB ini sudah selesai (%s)", domain.ErrInvalidStateTransition, *l.DeliveryStatus)
		}

		// Peringatan jarak GPS > 500m terhadap koordinat site (tidak memblokir, plan Bagian 5.8).
		if l.CustomerSiteID != nil {
			if coords, cerr := u.tracking.SiteCoordinates(ctx, *l.CustomerSiteID); cerr == nil &&
				coords != nil && coords.Latitude != nil && coords.Longitude != nil {
				d := haversineMeters(in.Latitude, in.Longitude, *coords.Latitude, *coords.Longitude)
				distMeters = &d
				if d > podDistanceWarningMeters {
					msg := fmt.Sprintf("Lokasi GPS berjarak %.0f m dari titik cabang pelanggan (> 500 m)", d)
					distWarn = &msg
				}
			}
		}

		var doCtx *domain.DOContext
		if l.ReferenceType != nil && *l.ReferenceType == "DELIVERY_ORDER" && l.ReferenceID != nil {
			doCtx, err = u.pod.LookupDOContext(ctx, *l.ReferenceID)
			if err != nil {
				return err
			}
		}

		if l.SubType == domain.LPBRentalDelivery {
			if doCtx == nil || doCtx.ContractID == nil {
				return fmt.Errorf("%w: LPB pengiriman sewa harus terhubung ke DO & kontrak", domain.ErrInvalidStateTransition)
			}
			items, err := u.lpb.Items(ctx, l.ID)
			if err != nil {
				return err
			}
			serialIDs := make([]int64, 0)
			serialProd := make(map[int64]int64)
			for _, it := range items {
				if !it.HasSerialNumber {
					continue
				}
				ids, err := unitIDs(it)
				if err != nil {
					return err
				}
				for _, sid := range ids {
					serialIDs = append(serialIDs, sid)
					serialProd[sid] = it.ProductID
				}
			}
			sort.Slice(serialIDs, func(i, j int) bool { return serialIDs[i] < serialIDs[j] })
			locked, err := u.stock.LockSerials(ctx, serialIDs)
			if err != nil {
				return err
			}
			if len(locked) != len(serialIDs) {
				return domain.ErrSerialNotValid
			}
			refType := "LPB"
			for _, s := range locked {
				if s.Status != domain.SerialInTransit {
					return fmt.Errorf("%w: unit %s harus berstatus IN_TRANSIT (saat ini %s)", domain.ErrSerialNotValid, s.Number, s.Status)
				}
				if err := u.pod.MarkSerialRented(ctx, s.ID, doCtx.CustomerID, doCtx.SiteID, doCtx.ContractID); err != nil {
					return err
				}
				cItemID, rate, err := u.pod.FindContractItemRate(ctx, *doCtx.ContractID, serialProd[s.ID], doCtx.SiteID)
				if err != nil {
					return err
				}
				if err := u.pod.InsertRentalPlacement(ctx, *doCtx.ContractID, cItemID, s.ID, doCtx.SiteID, rate, deliveredDate, l.ID); err != nil {
					return err
				}
				fromSt := string(s.Status)
				if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
					SerialID:      s.ID,
					FromStatus:    &fromSt,
					ToStatus:      string(domain.SerialRented),
					EventType:     "DELIVERED",
					ReferenceType: &refType,
					ReferenceID:   &l.ID,
					UserID:        &actor.UserID,
					Notes:         cleanOpt(in.Notes),
				}); err != nil {
					return err
				}
			}
		}

		if err := u.tracking.Insert(ctx, logEntry); err != nil {
			return err
		}
		if err := u.pod.MarkDelivered(ctx, l.ID, now, receiver, &photoPath); err != nil {
			return err
		}
		if doCtx != nil {
			if err := u.pod.CompleteDOAndMaybeSO(ctx, doCtx.DOID); err != nil {
				return err
			}
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "LPB_DELIVERED",
			EntityType: domain.EntityLPB,
			EntityID:   &l.ID,
			After: map[string]any{
				"delivery_status": "DELIVERED",
				"receiver_name":   receiver,
				"delivered_date":  deliveredDate,
				"driver_id":       actor.UserID,
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return &DriverDeliverResult{
		Log:             logEntry,
		Idempotent:      false,
		DistanceMeters:  distMeters,
		DistanceWarning: distWarn,
	}, nil
}

// Fail menandai pengiriman gagal (FAILED) beserta alasan wajib & checkpoint GPS.
func (u *TrackingUsecase) Fail(ctx context.Context, actor domain.Principal, lpbID int64, in DriverFailInput) (*domain.ShipmentTrackingLog, bool, error) {
	reason := strings.TrimSpace(in.Reason)
	var errs []domain.FieldError
	if n := utf8.RuneCountInString(reason); n < 3 || n > 500 {
		errs = append(errs, domain.FieldError{Field: "reason", Message: "alasan gagal kirim wajib diisi (3–500 karakter)"})
	}
	errs = append(errs, validateCoords(in.Latitude, in.Longitude)...)
	eventID, evErr := validateClientEventID(in.ClientEventID)
	if evErr != nil {
		errs = append(errs, *evErr)
	}
	if len(errs) > 0 {
		return nil, false, &domain.ValidationError{Message: "Data gagal kirim tidak valid", Errors: errs}
	}

	if eventID != nil {
		if existing, err := u.tracking.FindByClientEventID(ctx, *eventID); err != nil {
			return nil, false, err
		} else if existing != nil {
			if existing.LPBID != lpbID || (actor.Role != domain.RoleAdmin && existing.DriverID != actor.UserID) {
				return nil, false, domain.ErrForbidden
			}
			return existing, true, nil
		}
	}

	now := u.clock.Now()
	logEntry := &domain.ShipmentTrackingLog{
		LPBID:         lpbID,
		DriverID:      actor.UserID,
		Status:        domain.TrackingFailed,
		Latitude:      in.Latitude,
		Longitude:     in.Longitude,
		AccuracyM:     in.AccuracyM,
		RecordedAt:    now,
		ClientTime:    in.ClientTime,
		ClientEventID: eventID,
		FailureReason: &reason,
		Notes:         cleanOpt(in.Notes),
	}

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		l, err := u.lpb.FindByIDForUpdate(ctx, lpbID)
		if err != nil {
			return err
		}
		if err := ensureDriverAssigned(actor, l); err != nil {
			return err
		}
		if l.Status != domain.LPBApproved || (l.SubType != domain.LPBRentalDelivery && l.SubType != domain.LPBConsumableDelivery) {
			return fmt.Errorf("%w: gagal kirim hanya berlaku untuk LPB pengiriman APPROVED", domain.ErrInvalidStateTransition)
		}
		if isTerminalDeliveryStatus(l.DeliveryStatus) {
			return fmt.Errorf("%w: pengiriman LPB ini sudah selesai (%s)", domain.ErrInvalidStateTransition, *l.DeliveryStatus)
		}
		if err := u.tracking.Insert(ctx, logEntry); err != nil {
			return err
		}
		if err := u.tracking.UpdateLPBDeliveryStatus(ctx, l.ID, domain.TrackingFailed); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "LPB_DELIVERY_FAILED",
			EntityType: domain.EntityLPB,
			EntityID:   &l.ID,
			After: map[string]any{
				"delivery_status": "FAILED",
				"failure_reason":  reason,
				"driver_id":       actor.UserID,
			},
		})
	})
	if err != nil {
		return nil, false, err
	}
	return logEntry, false, nil
}

// Pickup menandai unit retur (RENTAL_RETURN) sudah dijemput driver di lokasi pelanggan:
// SN RENTED → IN_TRANSIT, menutup rental_placements aktif (tagihan berhenti), dan mencatat log PICKED_UP.
func (u *TrackingUsecase) Pickup(ctx context.Context, actor domain.Principal, lpbID int64, in DriverPickupInput) (*domain.ShipmentTrackingLog, bool, error) {
	errs := validateCoords(in.Latitude, in.Longitude)
	eventID, evErr := validateClientEventID(in.ClientEventID)
	if evErr != nil {
		errs = append(errs, *evErr)
	}
	if len(errs) > 0 {
		return nil, false, &domain.ValidationError{Message: "Data penjemputan tidak valid", Errors: errs}
	}

	if eventID != nil {
		if existing, err := u.tracking.FindByClientEventID(ctx, *eventID); err != nil {
			return nil, false, err
		} else if existing != nil {
			if existing.LPBID != lpbID || (actor.Role != domain.RoleAdmin && existing.DriverID != actor.UserID) {
				return nil, false, domain.ErrForbidden
			}
			return existing, true, nil
		}
	}

	now := u.clock.Now()
	pickupDate := clock.DateWIB(now)
	logEntry := &domain.ShipmentTrackingLog{
		LPBID:         lpbID,
		DriverID:      actor.UserID,
		Status:        domain.TrackingPickedUp,
		Latitude:      in.Latitude,
		Longitude:     in.Longitude,
		AccuracyM:     in.AccuracyM,
		RecordedAt:    now,
		ClientTime:    in.ClientTime,
		ClientEventID: eventID,
		Notes:         cleanOpt(in.Notes),
	}

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		l, err := u.lpb.FindByIDForUpdate(ctx, lpbID)
		if err != nil {
			return err
		}
		if err := ensureDriverAssigned(actor, l); err != nil {
			return err
		}
		if l.SubType != domain.LPBRentalReturn {
			return fmt.Errorf("%w: pickup penarikan hanya berlaku untuk LPB RENTAL_RETURN", domain.ErrInvalidStateTransition)
		}
		if l.Status != domain.LPBDraft && l.Status != domain.LPBPendingApproval {
			return fmt.Errorf("%w: LPB RENTAL_RETURN berstatus %s tidak dapat dijemput", domain.ErrInvalidStateTransition, l.Status)
		}
		if l.DeliveryStatus != nil && *l.DeliveryStatus == string(domain.TrackingPickedUp) {
			return fmt.Errorf("%w: unit pada LPB ini sudah ditandai PICKED_UP", domain.ErrInvalidStateTransition)
		}

		endReason := "WITHDRAWAL"
		if l.ReferenceType != nil && *l.ReferenceType == "DELIVERY_ORDER" && l.ReferenceID != nil {
			if doCtx, derr := u.pod.LookupDOContext(ctx, *l.ReferenceID); derr == nil && doCtx.OrderType == domain.OrderTypeUnitSwap {
				endReason = "SWAP"
			}
		}

		items, err := u.lpb.Items(ctx, l.ID)
		if err != nil {
			return err
		}
		serialIDs := make([]int64, 0)
		for _, it := range items {
			if !it.HasSerialNumber {
				continue
			}
			ids, err := unitIDs(it)
			if err != nil {
				return err
			}
			serialIDs = append(serialIDs, ids...)
		}
		if len(serialIDs) == 0 {
			return &domain.ValidationError{
				Message: "Data penjemputan tidak valid",
				Errors:  []domain.FieldError{{Field: "serials", Message: "scan/tautkan minimal 1 unit SN sebelum menandai pickup"}},
			}
		}

		sort.Slice(serialIDs, func(i, j int) bool { return serialIDs[i] < serialIDs[j] })
		locked, err := u.stock.LockSerials(ctx, serialIDs)
		if err != nil {
			return err
		}
		if len(locked) != len(serialIDs) {
			return domain.ErrSerialNotValid
		}
		refType := "LPB"
		for _, s := range locked {
			if s.Status != domain.SerialRented {
				return fmt.Errorf("%w: unit %s harus berstatus RENTED untuk dijemput (saat ini %s)", domain.ErrSerialNotValid, s.Number, s.Status)
			}
			if err := u.pod.MarkSerialPickupInTransit(ctx, s.ID); err != nil {
				return err
			}
			if err := u.pod.CloseActivePlacement(ctx, s.ID, pickupDate, endReason, l.ID); err != nil {
				return err
			}
			fromSt := string(s.Status)
			if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
				SerialID:      s.ID,
				FromStatus:    &fromSt,
				ToStatus:      string(domain.SerialInTransit),
				EventType:     "PICKED_UP",
				ReferenceType: &refType,
				ReferenceID:   &l.ID,
				UserID:        &actor.UserID,
				Notes:         cleanOpt(in.Notes),
			}); err != nil {
				return err
			}
		}

		if err := u.pod.MarkPickedUp(ctx, l.ID); err != nil {
			return err
		}
		if err := u.tracking.Insert(ctx, logEntry); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "LPB_PICKED_UP",
			EntityType: domain.EntityLPB,
			EntityID:   &l.ID,
			After: map[string]any{
				"delivery_status": "PICKED_UP",
				"pickup_date":     pickupDate,
				"end_reason":      endReason,
				"serials":         len(serialIDs),
				"driver_id":       actor.UserID,
			},
		})
	})
	if err != nil {
		return nil, false, err
	}
	return logEntry, false, nil
}

// Route mengembalikan polyline checkpoint & signed URL foto POD untuk satu LPB.
func (u *TrackingUsecase) Route(ctx context.Context, actor domain.Principal, lpbID int64) (*TrackingRouteResult, error) {
	l, err := u.lpb.FindByID(ctx, lpbID)
	if err != nil {
		return nil, err
	}
	if !actor.CanAccessWarehouse(l.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	logs, err := u.tracking.ListByLPB(ctx, l.ID)
	if err != nil {
		return nil, err
	}
	var podURL *string
	if l.PODPhotoPath != nil && strings.TrimSpace(*l.PODPhotoPath) != "" && u.storage != nil {
		if signed, serr := u.storage.SignedURL(ctx, *l.PODPhotoPath, storage.DefaultSignedURLTTL); serr == nil {
			podURL = &signed
		}
	}
	return &TrackingRouteResult{
		LPBID:          l.ID,
		LPBNumber:      l.Number,
		DeliveryStatus: l.DeliveryStatus,
		ReceiverName:   l.ReceiverName,
		DeliveredAt:    l.DeliveredAt,
		PODPhotoURL:    podURL,
		Logs:           logs,
	}, nil
}

func ensureDriverAssigned(actor domain.Principal, l *domain.LPB) error {
	if actor.Role == domain.RoleAdmin {
		return nil
	}
	if l.DriverID == nil || *l.DriverID != actor.UserID {
		return domain.ErrForbidden
	}
	return nil
}

func isTerminalDeliveryStatus(st *string) bool {
	if st == nil {
		return false
	}
	return *st == "DELIVERED" || *st == "FAILED" || *st == "RETURNED"
}

func validateCoords(lat, lng float64) []domain.FieldError {
	var errs []domain.FieldError
	if math.IsNaN(lat) || lat < -90 || lat > 90 {
		errs = append(errs, domain.FieldError{Field: "latitude", Message: "latitude harus antara -90 dan 90"})
	}
	if math.IsNaN(lng) || lng < -180 || lng > 180 {
		errs = append(errs, domain.FieldError{Field: "longitude", Message: "longitude harus antara -180 dan 180"})
	}
	return errs
}

func validateClientEventID(raw *string) (*string, *domain.FieldError) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	v := strings.TrimSpace(*raw)
	if !uuidPattern.MatchString(v) {
		return nil, &domain.FieldError{Field: "client_event_id", Message: "client_event_id harus berformat UUID yang valid"}
	}
	return &v, nil
}

// haversineMeters menghitung jarak garis lurus (meter) antara dua titik koordinat GPS.
func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	rLat1 := lat1 * math.Pi / 180
	rLat2 := lat2 * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rLat1)*math.Cos(rLat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}
