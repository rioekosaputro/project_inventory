package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/storage"
)

// TicketUsecase mengelola siklus hidup tiket maintenance teknisi (T3.8).
type TicketUsecase struct {
	tx      domain.TxManager
	repo    domain.TicketRepository
	audit   domain.AuditRepository
	docs    domain.DocNumberGenerator
	clock   clock.Clock
	storage storage.Uploader
}

func NewTicketUsecase(
	tx domain.TxManager,
	repo domain.TicketRepository,
	audit domain.AuditRepository,
	docs domain.DocNumberGenerator,
	clk clock.Clock,
) *TicketUsecase {
	return &TicketUsecase{tx: tx, repo: repo, audit: audit, docs: docs, clock: clk}
}

func (u *TicketUsecase) WithStorage(st storage.Uploader) *TicketUsecase {
	u.storage = st
	return u
}

type CreateTicketInput struct {
	CustomerID      int64
	CustomerSiteID  int64
	ProductSerialID int64
	ReportedByName  *string
	ReportedVia     string
	Category        string
	Priority        string
	Title           string
	Description     *string
	AssignedTo      *int64
	SLADueAt        *time.Time
}

type UpdateTicketInput struct {
	ReportedByName *string
	ReportedVia    string
	Category       string
	Priority       string
	Title          string
	Description    *string
	SLADueAt       *time.Time
}

type AddTicketActivityInput struct {
	ActivityType string
	Message      *string
	Latitude     *float64
	Longitude    *float64
	PhotoPath    *string
}

func (u *TicketUsecase) List(ctx context.Context, f domain.TicketFilter, p domain.PageParams) ([]domain.MaintenanceTicket, int64, error) {
	f.Query = strings.TrimSpace(f.Query)
	return u.repo.List(ctx, f, p)
}

func (u *TicketUsecase) Get(ctx context.Context, id int64) (*domain.TicketDetail, error) {
	t, err := u.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	acts, err := u.repo.ListActivities(ctx, id)
	if err != nil {
		return nil, err
	}
	if u.storage != nil {
		for i := range acts {
			if acts[i].PhotoPath != nil && strings.TrimSpace(*acts[i].PhotoPath) != "" {
				if signed, serr := u.storage.SignedURL(ctx, *acts[i].PhotoPath, storage.DefaultSignedURLTTL); serr == nil {
					acts[i].PhotoURL = &signed
				}
			}
		}
	}
	sps, err := u.repo.ListSpareparts(ctx, id)
	if err != nil {
		return nil, err
	}
	return &domain.TicketDetail{
		MaintenanceTicket: *t,
		Activities:        acts,
		Spareparts:        sps,
	}, nil
}

func (u *TicketUsecase) Create(ctx context.Context, actor domain.Principal, in CreateTicketInput) (*domain.TicketDetail, error) {
	title := strings.TrimSpace(in.Title)
	var errs []domain.FieldError
	if in.CustomerID <= 0 {
		errs = append(errs, domain.FieldError{Field: "customer_id", Message: "pelanggan wajib dipilih"})
	}
	if in.CustomerSiteID <= 0 {
		errs = append(errs, domain.FieldError{Field: "customer_site_id", Message: "cabang pelanggan wajib dipilih"})
	}
	if in.ProductSerialID <= 0 {
		errs = append(errs, domain.FieldError{Field: "product_serial_id", Message: "unit serial wajib dipilih"})
	}
	if n := utf8.RuneCountInString(title); n < 3 || n > 200 {
		errs = append(errs, domain.FieldError{Field: "title", Message: "judul tiket wajib diisi (3–200 karakter)"})
	}

	via := domain.TicketReportedVia(strings.ToUpper(strings.TrimSpace(in.ReportedVia)))
	if via == "" {
		via = domain.ViaPhone
	}
	if !via.Valid() {
		errs = append(errs, domain.FieldError{Field: "reported_via", Message: "saluran laporan tidak valid"})
	}
	cat := domain.TicketCategory(strings.ToUpper(strings.TrimSpace(in.Category)))
	if cat == "" {
		cat = domain.CatOther
	}
	if !cat.Valid() {
		errs = append(errs, domain.FieldError{Field: "category", Message: "kategori tiket tidak valid"})
	}
	prio := domain.TicketPriority(strings.ToUpper(strings.TrimSpace(in.Priority)))
	if prio == "" {
		prio = domain.PriorityMedium
	}
	if !prio.Valid() {
		errs = append(errs, domain.FieldError{Field: "priority", Message: "prioritas tiket tidak valid"})
	}
	if len(errs) > 0 {
		return nil, &domain.ValidationError{Message: "Data tiket tidak valid", Errors: errs}
	}

	now := u.clock.Now()
	slaDue := in.SLADueAt
	if slaDue == nil {
		d := defaultSLADue(now, prio)
		slaDue = &d
	}
	initStatus := domain.TicketOpen
	if in.AssignedTo != nil && *in.AssignedTo > 0 {
		initStatus = domain.TicketAssigned
	}

	var createdID int64
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		num, err := u.docs.Next(ctx, "TKT", "GLOBAL", now)
		if err != nil {
			return err
		}
		created, err := u.repo.Create(ctx, &domain.MaintenanceTicket{
			TicketNumber:    num,
			CustomerID:      in.CustomerID,
			CustomerSiteID:  in.CustomerSiteID,
			ProductSerialID: in.ProductSerialID,
			ReportedByName:  cleanOpt(in.ReportedByName),
			ReportedVia:     via,
			Category:        cat,
			Priority:        prio,
			Status:          initStatus,
			Title:           title,
			Description:     cleanOpt(in.Description),
			AssignedTo:      in.AssignedTo,
			SLADueAt:        slaDue,
			CreatedBy:       actor.UserID,
		})
		if err != nil {
			return err
		}
		createdID = created.ID
		msg := "Tiket dibuat"
		if _, err := u.repo.InsertActivity(ctx, &domain.TicketActivity{
			TicketID:     created.ID,
			UserID:       actor.UserID,
			ActivityType: domain.ActivityStatusChange,
			Message:      &msg,
			ToStatus:     strPtr(string(initStatus)),
		}); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "TICKET_CREATE",
			EntityType: "maintenance_ticket",
			EntityID:   &created.ID,
			After: map[string]any{
				"ticket_number": created.TicketNumber,
				"priority":      string(prio),
				"status":        string(initStatus),
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.Get(ctx, createdID)
}

func (u *TicketUsecase) Update(ctx context.Context, actor domain.Principal, id int64, in UpdateTicketInput) (*domain.TicketDetail, error) {
	title := strings.TrimSpace(in.Title)
	var errs []domain.FieldError
	if n := utf8.RuneCountInString(title); n < 3 || n > 200 {
		errs = append(errs, domain.FieldError{Field: "title", Message: "judul tiket wajib diisi (3–200 karakter)"})
	}
	via := domain.TicketReportedVia(strings.ToUpper(strings.TrimSpace(in.ReportedVia)))
	if !via.Valid() {
		errs = append(errs, domain.FieldError{Field: "reported_via", Message: "saluran laporan tidak valid"})
	}
	cat := domain.TicketCategory(strings.ToUpper(strings.TrimSpace(in.Category)))
	if !cat.Valid() {
		errs = append(errs, domain.FieldError{Field: "category", Message: "kategori tiket tidak valid"})
	}
	prio := domain.TicketPriority(strings.ToUpper(strings.TrimSpace(in.Priority)))
	if !prio.Valid() {
		errs = append(errs, domain.FieldError{Field: "priority", Message: "prioritas tiket tidak valid"})
	}
	if len(errs) > 0 {
		return nil, &domain.ValidationError{Message: "Data tiket tidak valid", Errors: errs}
	}

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		t, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if t.Status == domain.TicketClosed || t.Status == domain.TicketCancelled {
			return fmt.Errorf("%w: tiket berstatus %s tidak dapat diubah", domain.ErrInvalidStateTransition, t.Status)
		}
		t.Title = title
		t.Description = cleanOpt(in.Description)
		t.Category = cat
		t.Priority = prio
		t.ReportedByName = cleanOpt(in.ReportedByName)
		t.ReportedVia = via
		if in.SLADueAt != nil {
			t.SLADueAt = in.SLADueAt
		}
		if err := u.repo.Update(ctx, t); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "TICKET_UPDATE",
			EntityType: "maintenance_ticket",
			EntityID:   &t.ID,
			After:      map[string]any{"title": title, "priority": string(prio), "category": string(cat)},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.Get(ctx, id)
}

func (u *TicketUsecase) Assign(ctx context.Context, actor domain.Principal, id int64, technicianID int64, notes *string) (*domain.TicketDetail, error) {
	if technicianID <= 0 {
		return nil, &domain.ValidationError{
			Message: "Teknisi wajib dipilih",
			Errors:  []domain.FieldError{{Field: "assigned_to", Message: "ID teknisi wajib diisi"}},
		}
	}
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		t, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if t.Status == domain.TicketClosed || t.Status == domain.TicketCancelled {
			return fmt.Errorf("%w: tiket berstatus %s tidak dapat ditugaskan", domain.ErrInvalidStateTransition, t.Status)
		}
		newStatus := t.Status
		if t.Status == domain.TicketOpen {
			newStatus = domain.TicketAssigned
		}
		if err := u.repo.Assign(ctx, t.ID, technicianID, newStatus); err != nil {
			return err
		}
		msg := "Teknisi ditugaskan"
		if clean := cleanOpt(notes); clean != nil {
			msg = *clean
		}
		if _, err := u.repo.InsertActivity(ctx, &domain.TicketActivity{
			TicketID:     t.ID,
			UserID:       actor.UserID,
			ActivityType: domain.ActivityAssign,
			Message:      &msg,
			FromStatus:   strPtr(string(t.Status)),
			ToStatus:     strPtr(string(newStatus)),
		}); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "TICKET_ASSIGN",
			EntityType: "maintenance_ticket",
			EntityID:   &t.ID,
			After:      map[string]any{"assigned_to": technicianID, "status": string(newStatus)},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.Get(ctx, id)
}

func (u *TicketUsecase) ChangeStatus(ctx context.Context, actor domain.Principal, id int64, rawToStatus string, resolutionNotes, notes *string) (*domain.TicketDetail, error) {
	toStatus := domain.TicketStatus(strings.ToUpper(strings.TrimSpace(rawToStatus)))
	if !toStatus.Valid() {
		return nil, &domain.ValidationError{
			Message: "Status tiket tidak valid",
			Errors:  []domain.FieldError{{Field: "status", Message: "status tidak dikenal"}},
		}
	}
	now := u.clock.Now()
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		t, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !canTransitionTicket(t.Status, toStatus) {
			return fmt.Errorf("%w: transisi status tiket %s → %s tidak diizinkan", domain.ErrInvalidStateTransition, t.Status, toStatus)
		}
		var resolvedAt, closedAt *time.Time
		if toStatus == domain.TicketResolved {
			resolvedAt = &now
		}
		if toStatus == domain.TicketClosed {
			closedAt = &now
		}
		if err := u.repo.UpdateStatus(ctx, t.ID, toStatus, cleanOpt(resolutionNotes), resolvedAt, closedAt); err != nil {
			return err
		}
		msg := fmt.Sprintf("Status diubah dari %s ke %s", t.Status, toStatus)
		if clean := cleanOpt(notes); clean != nil {
			msg = *clean
		} else if clean := cleanOpt(resolutionNotes); clean != nil && toStatus == domain.TicketResolved {
			msg = *clean
		}
		if _, err := u.repo.InsertActivity(ctx, &domain.TicketActivity{
			TicketID:     t.ID,
			UserID:       actor.UserID,
			ActivityType: domain.ActivityStatusChange,
			Message:      &msg,
			FromStatus:   strPtr(string(t.Status)),
			ToStatus:     strPtr(string(toStatus)),
		}); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "TICKET_STATUS_CHANGE",
			EntityType: "maintenance_ticket",
			EntityID:   &t.ID,
			Before:     map[string]any{"status": string(t.Status)},
			After:      map[string]any{"status": string(toStatus)},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.Get(ctx, id)
}

func (u *TicketUsecase) AddActivity(ctx context.Context, actor domain.Principal, id int64, in AddTicketActivityInput) (*domain.TicketActivity, error) {
	actType := domain.TicketActivityType(strings.ToUpper(strings.TrimSpace(in.ActivityType)))
	var errs []domain.FieldError
	if !actType.ValidManual() {
		errs = append(errs, domain.FieldError{Field: "activity_type", Message: "tipe aktivitas harus COMMENT, VISIT_CHECKIN, VISIT_CHECKOUT, atau PHOTO"})
	}
	msg := cleanOpt(in.Message)
	photo := cleanOpt(in.PhotoPath)
	if actType == domain.ActivityComment && msg == nil {
		errs = append(errs, domain.FieldError{Field: "message", Message: "pesan komentar wajib diisi"})
	}
	if (actType == domain.ActivityVisitCheckin || actType == domain.ActivityVisitCheckout) && (in.Latitude == nil || in.Longitude == nil) {
		errs = append(errs, domain.FieldError{Field: "latitude", Message: "koordinat GPS wajib untuk check-in/check-out kunjungan"})
	} else if in.Latitude != nil && in.Longitude != nil {
		errs = append(errs, validateCoords(*in.Latitude, *in.Longitude)...)
	}
	if actType == domain.ActivityPhoto && photo == nil {
		errs = append(errs, domain.FieldError{Field: "photo", Message: "foto wajib diunggah untuk aktivitas PHOTO"})
	}
	if len(errs) > 0 {
		return nil, &domain.ValidationError{Message: "Data aktivitas tiket tidak valid", Errors: errs}
	}

	var created *domain.TicketActivity
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		t, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if t.Status == domain.TicketClosed || t.Status == domain.TicketCancelled {
			return fmt.Errorf("%w: tidak dapat menambah aktivitas pada tiket berstatus %s", domain.ErrInvalidStateTransition, t.Status)
		}
		created, err = u.repo.InsertActivity(ctx, &domain.TicketActivity{
			TicketID:     t.ID,
			UserID:       actor.UserID,
			ActivityType: actType,
			Message:      msg,
			Latitude:     in.Latitude,
			Longitude:    in.Longitude,
			PhotoPath:    photo,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	if created.PhotoPath != nil && u.storage != nil {
		if signed, serr := u.storage.SignedURL(ctx, *created.PhotoPath, storage.DefaultSignedURLTTL); serr == nil {
			created.PhotoURL = &signed
		}
	}
	return created, nil
}

func canTransitionTicket(from, to domain.TicketStatus) bool {
	if from == to {
		return false
	}
	if to == domain.TicketCancelled {
		return from != domain.TicketClosed && from != domain.TicketCancelled
	}
	switch from {
	case domain.TicketOpen:
		return to == domain.TicketAssigned || to == domain.TicketInProgress
	case domain.TicketAssigned:
		return to == domain.TicketInProgress || to == domain.TicketWaitingPart || to == domain.TicketResolved
	case domain.TicketInProgress:
		return to == domain.TicketWaitingPart || to == domain.TicketResolved
	case domain.TicketWaitingPart:
		return to == domain.TicketInProgress || to == domain.TicketResolved
	case domain.TicketResolved:
		return to == domain.TicketClosed || to == domain.TicketInProgress
	default:
		return false
	}
}

func defaultSLADue(now time.Time, p domain.TicketPriority) time.Time {
	switch p {
	case domain.PriorityCritical:
		return now.Add(4 * time.Hour)
	case domain.PriorityHigh:
		return now.Add(8 * time.Hour)
	case domain.PriorityLow:
		return now.Add(72 * time.Hour)
	default:
		return now.Add(24 * time.Hour)
	}
}

func strPtr(s string) *string { return &s }
