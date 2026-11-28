package handler

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/response"
	"rental-inventory-api/pkg/storage"
)

// TrackingHandler melayani endpoint lapangan driver & rute pengiriman (/api/v1/tracking/*).
type TrackingHandler struct {
	uc      *usecase.TrackingUsecase
	storage storage.Uploader
}

func NewTrackingHandler(uc *usecase.TrackingUsecase, st storage.Uploader) *TrackingHandler {
	return &TrackingHandler{uc: uc, storage: st}
}

type checkpointRequest struct {
	Status        string  `json:"status" binding:"required"`
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	AccuracyM     *int    `json:"accuracy_m"`
	Accuracy      *int    `json:"accuracy"` // alias dari plan Bagian 8.5
	ClientTime    *string `json:"client_time"`
	ClientEventID *string `json:"client_event_id"`
	Notes         *string `json:"notes"`
}

type driverDeliverJSONRequest struct {
	ReceiverName  string  `json:"receiver_name" binding:"required"`
	PODPhotoPath  string  `json:"pod_photo_path"`
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	AccuracyM     *int    `json:"accuracy_m"`
	ClientTime    *string `json:"client_time"`
	ClientEventID *string `json:"client_event_id"`
	Notes         *string `json:"notes"`
}

type driverFailRequest struct {
	Reason        string   `json:"reason" binding:"required"`
	Latitude      *float64 `json:"latitude"`
	Longitude     *float64 `json:"longitude"`
	Lat           *float64 `json:"lat"` // alias dari plan Bagian 8.5
	Lng           *float64 `json:"lng"` // alias dari plan Bagian 8.5
	AccuracyM     *int     `json:"accuracy_m"`
	ClientTime    *string  `json:"client_time"`
	ClientEventID *string  `json:"client_event_id"`
	Notes         *string  `json:"notes"`
}

type driverPickupRequest struct {
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	AccuracyM     *int    `json:"accuracy_m"`
	ClientTime    *string `json:"client_time"`
	ClientEventID *string `json:"client_event_id"`
	Notes         *string `json:"notes"`
}

// MyDeliveries — GET /api/v1/tracking/my-deliveries
func (h *TrackingHandler) MyDeliveries(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	tasks, err := h.uc.MyDeliveries(c.Request.Context(), actor)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]gin.H, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, gin.H{
			"lpb_id":           t.LPBID,
			"lpb_number":       t.LPBNumber,
			"type":             t.Type,
			"sub_type":         t.SubType,
			"status":           t.Status,
			"delivery_status":  t.DeliveryStatus,
			"doc_date":         t.DocDate,
			"warehouse_id":     t.WarehouseID,
			"warehouse_code":   t.WarehouseCode,
			"warehouse_name":   t.WarehouseName,
			"customer_id":      t.CustomerID,
			"customer_name":    t.CustomerName,
			"customer_site_id": t.CustomerSiteID,
			"site_name":        t.SiteName,
			"site_address":     t.SiteAddress,
			"site_city":        t.SiteCity,
			"site_pic_name":    t.SitePICName,
			"site_pic_phone":   t.SitePICPhone,
			"site_latitude":    t.SiteLatitude,
			"site_longitude":   t.SiteLongitude,
			"item_count":       t.ItemCount,
			"total_qty":        t.TotalQty,
			"notes":            t.Notes,
		})
	}
	response.OK(c, out)
}

// Checkpoint — POST /api/v1/tracking/lpb/:id/checkpoint
func (h *TrackingHandler) Checkpoint(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	lpbID, ok := parseID(c)
	if !ok {
		return
	}
	var req checkpointRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	acc := req.AccuracyM
	if acc == nil {
		acc = req.Accuracy
	}
	clientTime, err := parseOptionalTime(req.ClientTime)
	if err != nil {
		httperr.Write(c, &domain.ValidationError{
			Message: "Format client_time tidak valid",
			Errors:  []domain.FieldError{{Field: "client_time", Message: "gunakan format RFC3339"}},
		})
		return
	}

	logEntry, idempotent, err := h.uc.RecordCheckpoint(c.Request.Context(), actor, lpbID, usecase.CheckpointInput{
		Status:        req.Status,
		Latitude:      req.Latitude,
		Longitude:     req.Longitude,
		AccuracyM:     acc,
		ClientTime:    clientTime,
		ClientEventID: req.ClientEventID,
		Notes:         req.Notes,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, gin.H{
		"checkpoint": trackingLogDTO(logEntry),
		"idempotent": idempotent,
	})
}

// Deliver — POST /api/v1/tracking/lpb/:id/deliver (multipart: pod_photo/photo, receiver_name, latitude, longitude, client_event_id)
func (h *TrackingHandler) Deliver(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	lpbID, ok := parseID(c)
	if !ok {
		return
	}

	var in usecase.DriverDeliverInput
	var uploadedKey string

	ct := c.GetHeader("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		in.ReceiverName = c.PostForm("receiver_name")
		lat, latErr := strconv.ParseFloat(strings.TrimSpace(c.PostForm("latitude")), 64)
		lng, lngErr := strconv.ParseFloat(strings.TrimSpace(c.PostForm("longitude")), 64)
		if latErr != nil || lngErr != nil {
			httperr.Write(c, &domain.ValidationError{
				Message: "Koordinat GPS wajib diisi",
				Errors: []domain.FieldError{
					{Field: "latitude", Message: "latitude wajib berupa angka"},
					{Field: "longitude", Message: "longitude wajib berupa angka"},
				},
			})
			return
		}
		in.Latitude = lat
		in.Longitude = lng
		if accStr := strings.TrimSpace(c.PostForm("accuracy_m")); accStr != "" {
			if acc, err := strconv.Atoi(accStr); err == nil {
				in.AccuracyM = &acc
			}
		}
		if ctStr := strings.TrimSpace(c.PostForm("client_time")); ctStr != "" {
			parsed, err := parseOptionalTime(&ctStr)
			if err != nil {
				httperr.Write(c, &domain.ValidationError{
					Message: "Format client_time tidak valid",
					Errors:  []domain.FieldError{{Field: "client_time", Message: "gunakan format RFC3339"}},
				})
				return
			}
			in.ClientTime = parsed
		}
		if ev := strings.TrimSpace(c.PostForm("client_event_id")); ev != "" {
			in.ClientEventID = &ev
		}
		if notes := strings.TrimSpace(c.PostForm("notes")); notes != "" {
			in.Notes = &notes
		}

		// Terima field "pod_photo" (plan Bagian 8.5) atau "photo" sebagai alias.
		fh, err := c.FormFile("pod_photo")
		if err != nil {
			fh, err = c.FormFile("photo")
		}
		if err == nil && fh != nil {
			if h.storage == nil {
				httperr.Write(c, errors.New("storage belum dikonfigurasi"))
				return
			}
			f, openErr := fh.Open()
			if openErr != nil {
				httperr.Write(c, openErr)
				return
			}
			res, upErr := h.storage.Upload(c.Request.Context(), "pod", f)
			_ = f.Close()
			if upErr != nil {
				if errors.Is(upErr, storage.ErrFileTooLarge) ||
					errors.Is(upErr, storage.ErrInvalidMimeType) ||
					errors.Is(upErr, storage.ErrEmptyFile) {
					httperr.Write(c, &domain.ValidationError{
						Message: upErr.Error(),
						Errors:  []domain.FieldError{{Field: "pod_photo", Message: upErr.Error()}},
					})
					return
				}
				httperr.Write(c, upErr)
				return
			}
			uploadedKey = res.Key
			in.PODPhotoPath = res.Key
		}
	} else {
		var req driverDeliverJSONRequest
		if !httperr.BindJSON(c, &req) {
			return
		}
		parsedTime, err := parseOptionalTime(req.ClientTime)
		if err != nil {
			httperr.Write(c, &domain.ValidationError{
				Message: "Format client_time tidak valid",
				Errors:  []domain.FieldError{{Field: "client_time", Message: "gunakan format RFC3339"}},
			})
			return
		}
		in = usecase.DriverDeliverInput{
			ReceiverName:  req.ReceiverName,
			PODPhotoPath:  req.PODPhotoPath,
			Latitude:      req.Latitude,
			Longitude:     req.Longitude,
			AccuracyM:     req.AccuracyM,
			ClientTime:    parsedTime,
			ClientEventID: req.ClientEventID,
			Notes:         req.Notes,
		}
	}

	res, err := h.uc.Deliver(c.Request.Context(), actor, lpbID, in)
	if err != nil {
		if uploadedKey != "" && h.storage != nil {
			_ = h.storage.Delete(c.Request.Context(), uploadedKey)
		}
		httperr.Write(c, err)
		return
	}
	// Jika replay idempoten mengembalikan log lama, hapus file duplikat yang baru saja diunggah.
	if res.Idempotent && uploadedKey != "" && h.storage != nil {
		_ = h.storage.Delete(c.Request.Context(), uploadedKey)
	}
	response.OK(c, gin.H{
		"checkpoint":       trackingLogDTO(res.Log),
		"idempotent":       res.Idempotent,
		"distance_meters":  res.DistanceMeters,
		"distance_warning": res.DistanceWarning,
	})
}

// Fail — POST /api/v1/tracking/lpb/:id/fail
func (h *TrackingHandler) Fail(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	lpbID, ok := parseID(c)
	if !ok {
		return
	}
	var req driverFailRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	lat := req.Latitude
	if lat == nil {
		lat = req.Lat
	}
	lng := req.Longitude
	if lng == nil {
		lng = req.Lng
	}
	if lat == nil || lng == nil {
		httperr.Write(c, &domain.ValidationError{
			Message: "Koordinat GPS wajib diisi",
			Errors: []domain.FieldError{
				{Field: "latitude", Message: "latitude wajib diisi"},
				{Field: "longitude", Message: "longitude wajib diisi"},
			},
		})
		return
	}
	clientTime, err := parseOptionalTime(req.ClientTime)
	if err != nil {
		httperr.Write(c, &domain.ValidationError{
			Message: "Format client_time tidak valid",
			Errors:  []domain.FieldError{{Field: "client_time", Message: "gunakan format RFC3339"}},
		})
		return
	}

	res, err := h.uc.Fail(c.Request.Context(), actor, lpbID, usecase.DriverFailInput{
		Reason:        req.Reason,
		Latitude:      *lat,
		Longitude:     *lng,
		AccuracyM:     req.AccuracyM,
		ClientTime:    clientTime,
		ClientEventID: req.ClientEventID,
		Notes:         req.Notes,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, gin.H{
		"checkpoint":        trackingLogDTO(res.Log),
		"idempotent":        res.Idempotent,
		"return_lpb_id":     res.ReturnLPBID,
		"return_lpb_number": res.ReturnLPBNumber,
	})
}

// Pickup — POST /api/v1/tracking/lpb/:id/pickup
func (h *TrackingHandler) Pickup(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	lpbID, ok := parseID(c)
	if !ok {
		return
	}
	var req driverPickupRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	clientTime, err := parseOptionalTime(req.ClientTime)
	if err != nil {
		httperr.Write(c, &domain.ValidationError{
			Message: "Format client_time tidak valid",
			Errors:  []domain.FieldError{{Field: "client_time", Message: "gunakan format RFC3339"}},
		})
		return
	}

	logEntry, idempotent, err := h.uc.Pickup(c.Request.Context(), actor, lpbID, usecase.DriverPickupInput{
		Latitude:      req.Latitude,
		Longitude:     req.Longitude,
		AccuracyM:     req.AccuracyM,
		ClientTime:    clientTime,
		ClientEventID: req.ClientEventID,
		Notes:         req.Notes,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, gin.H{
		"checkpoint": trackingLogDTO(logEntry),
		"idempotent": idempotent,
	})
}

// Route — GET /api/v1/tracking/lpb/:id/route
func (h *TrackingHandler) Route(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	lpbID, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.uc.Route(c.Request.Context(), actor, lpbID)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	logs := make([]gin.H, 0, len(res.Logs))
	for i := range res.Logs {
		logs = append(logs, trackingLogDTO(&res.Logs[i]))
	}
	response.OK(c, gin.H{
		"lpb_id":          res.LPBID,
		"lpb_number":      res.LPBNumber,
		"delivery_status": res.DeliveryStatus,
		"receiver_name":   res.ReceiverName,
		"delivered_at":    res.DeliveredAt,
		"pod_photo_url":   res.PODPhotoURL,
		"logs":            logs,
	})
}

// MapAssets — GET /api/v1/tracking/map/assets (pin per site + daftar unit RENTED, T3.7).
func (h *TrackingHandler) MapAssets(c *gin.Context) {
	f := domain.MapAssetsFilter{
		Query: strings.TrimSpace(c.Query("q")),
	}
	if cidStr := strings.TrimSpace(c.Query("customer_id")); cidStr != "" {
		cid, err := strconv.ParseInt(cidStr, 10, 64)
		if err != nil || cid <= 0 {
			httperr.Write(c, &domain.ValidationError{
				Message: "Filter customer_id tidak valid",
				Errors:  []domain.FieldError{{Field: "customer_id", Message: "harus berupa bilangan bulat positif"}},
			})
			return
		}
		f.CustomerID = &cid
	}

	pins, err := h.uc.MapAssets(c.Request.Context(), f)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]gin.H, 0, len(pins))
	for _, p := range pins {
		units := make([]gin.H, 0, len(p.Units))
		for _, u := range p.Units {
			units = append(units, gin.H{
				"serial_id":     u.SerialID,
				"asset_tag":     u.AssetTag,
				"serial_number": u.SerialNumber,
				"sku":           u.SKU,
				"product_name":  u.ProductName,
				"contract_id":   u.ContractID,
			})
		}
		out = append(out, gin.H{
			"site_id":       p.SiteID,
			"site_name":     p.SiteName,
			"customer_id":   p.CustomerID,
			"customer_name": p.CustomerName,
			"address_line":  p.AddressLine,
			"city":          p.City,
			"province":      p.Province,
			"pic_name":      p.PICName,
			"pic_phone":     p.PICPhone,
			"latitude":      p.Latitude,
			"longitude":     p.Longitude,
			"unit_count":    len(p.Units),
			"units":         units,
		})
	}
	response.OK(c, out)
}

func trackingLogDTO(l *domain.ShipmentTrackingLog) gin.H {
	return gin.H{
		"id":              l.ID,
		"lpb_id":          l.LPBID,
		"driver_id":       l.DriverID,
		"driver_name":     l.DriverName,
		"status":          l.Status,
		"latitude":        l.Latitude,
		"longitude":       l.Longitude,
		"accuracy_m":      l.AccuracyM,
		"recorded_at":     l.RecordedAt,
		"client_time":     l.ClientTime,
		"client_event_id": l.ClientEventID,
		"receiver_name":   l.ReceiverName,
		"photo_path":      l.PhotoPath,
		"failure_reason":  l.FailureReason,
		"notes":           l.Notes,
	}
}

func parseOptionalTime(raw *string) (*time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(*raw))
	if err != nil {
		return nil, err
	}
	return &t, nil
}
