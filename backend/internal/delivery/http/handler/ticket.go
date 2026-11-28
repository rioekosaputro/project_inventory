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
	"rental-inventory-api/pkg/pagination"
	"rental-inventory-api/pkg/response"
	"rental-inventory-api/pkg/storage"
)

var ticketSort = pagination.SortSpec{
	Allowed: map[string]string{
		"ticket_number": "t.ticket_number",
		"priority":      "t.priority",
		"status":        "t.status",
		"sla_due_at":    "t.sla_due_at",
		"created_at":    "t.created_at",
	},
	Default: "-created_at",
}

// TicketHandler melayani /api/v1/tickets/* (T3.8).
type TicketHandler struct {
	uc      *usecase.TicketUsecase
	storage storage.Uploader
}

func NewTicketHandler(uc *usecase.TicketUsecase, st storage.Uploader) *TicketHandler {
	return &TicketHandler{uc: uc, storage: st}
}

type createTicketRequest struct {
	CustomerID      int64   `json:"customer_id" binding:"required"`
	CustomerSiteID  int64   `json:"customer_site_id" binding:"required"`
	ProductSerialID int64   `json:"product_serial_id" binding:"required"`
	ReportedByName  *string `json:"reported_by_name"`
	ReportedVia     string  `json:"reported_via"`
	Category        string  `json:"category"`
	Priority        string  `json:"priority"`
	Title           string  `json:"title" binding:"required"`
	Description     *string `json:"description"`
	AssignedTo      *int64  `json:"assigned_to"`
	SLADueAt        *string `json:"sla_due_at"`
}

type updateTicketRequest struct {
	ReportedByName *string `json:"reported_by_name"`
	ReportedVia    string  `json:"reported_via" binding:"required"`
	Category       string  `json:"category" binding:"required"`
	Priority       string  `json:"priority" binding:"required"`
	Title          string  `json:"title" binding:"required"`
	Description    *string `json:"description"`
	SLADueAt       *string `json:"sla_due_at"`
}

type assignTicketRequest struct {
	AssignedTo int64   `json:"assigned_to" binding:"required"`
	Notes      *string `json:"notes"`
}

type ticketStatusRequest struct {
	Status          string  `json:"status" binding:"required"`
	ResolutionNotes *string `json:"resolution_notes"`
	Notes           *string `json:"notes"`
}

type ticketActivityJSONRequest struct {
	ActivityType string   `json:"activity_type" binding:"required"`
	Message      *string  `json:"message"`
	Latitude     *float64 `json:"latitude"`
	Longitude    *float64 `json:"longitude"`
	PhotoPath    *string  `json:"photo_path"`
}

// List — GET /api/v1/tickets
func (h *TicketHandler) List(c *gin.Context) {
	page, pp, ok := pageOf(c, ticketSort)
	if !ok {
		return
	}
	f := domain.TicketFilter{Query: strings.TrimSpace(c.Query("q"))}
	if st := strings.ToUpper(strings.TrimSpace(c.Query("status"))); st != "" {
		s := domain.TicketStatus(st)
		f.Status = &s
	}
	if pr := strings.ToUpper(strings.TrimSpace(c.Query("priority"))); pr != "" {
		p := domain.TicketPriority(pr)
		f.Priority = &p
	}
	if v := strings.TrimSpace(c.Query("assigned_to")); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			f.AssignedTo = &id
		}
	}
	if v := strings.TrimSpace(c.Query("customer_id")); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			f.CustomerID = &id
		}
	}

	items, total, err := h.uc.List(c.Request.Context(), f, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]gin.H, 0, len(items))
	for i := range items {
		out = append(out, ticketDTO(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /api/v1/tickets/:id
func (h *TicketHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	det, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, ticketDetailDTO(det))
}

// Create — POST /api/v1/tickets
func (h *TicketHandler) Create(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	var req createTicketRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	slaDue, err := parseOptionalTime(req.SLADueAt)
	if err != nil {
		httperr.Write(c, &domain.ValidationError{
			Message: "Format sla_due_at tidak valid",
			Errors:  []domain.FieldError{{Field: "sla_due_at", Message: "gunakan format RFC3339"}},
		})
		return
	}
	det, err := h.uc.Create(c.Request.Context(), actor, usecase.CreateTicketInput{
		CustomerID:      req.CustomerID,
		CustomerSiteID:  req.CustomerSiteID,
		ProductSerialID: req.ProductSerialID,
		ReportedByName:  req.ReportedByName,
		ReportedVia:     req.ReportedVia,
		Category:        req.Category,
		Priority:        req.Priority,
		Title:           req.Title,
		Description:     req.Description,
		AssignedTo:      req.AssignedTo,
		SLADueAt:        slaDue,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, ticketDetailDTO(det))
}

// Update — PATCH /api/v1/tickets/:id
func (h *TicketHandler) Update(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req updateTicketRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	slaDue, err := parseOptionalTime(req.SLADueAt)
	if err != nil {
		httperr.Write(c, &domain.ValidationError{
			Message: "Format sla_due_at tidak valid",
			Errors:  []domain.FieldError{{Field: "sla_due_at", Message: "gunakan format RFC3339"}},
		})
		return
	}
	det, err := h.uc.Update(c.Request.Context(), actor, id, usecase.UpdateTicketInput{
		ReportedByName: req.ReportedByName,
		ReportedVia:    req.ReportedVia,
		Category:       req.Category,
		Priority:       req.Priority,
		Title:          req.Title,
		Description:    req.Description,
		SLADueAt:       slaDue,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, ticketDetailDTO(det))
}

// Assign — POST /api/v1/tickets/:id/assign
func (h *TicketHandler) Assign(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req assignTicketRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	det, err := h.uc.Assign(c.Request.Context(), actor, id, req.AssignedTo, req.Notes)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, ticketDetailDTO(det))
}

// ChangeStatus — POST /api/v1/tickets/:id/status
func (h *TicketHandler) ChangeStatus(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req ticketStatusRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	det, err := h.uc.ChangeStatus(c.Request.Context(), actor, id, req.Status, req.ResolutionNotes, req.Notes)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, ticketDetailDTO(det))
}

// AddActivity — POST /api/v1/tickets/:id/activities (JSON atau multipart foto).
func (h *TicketHandler) AddActivity(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}

	var in usecase.AddTicketActivityInput
	var uploadedKey string

	ct := c.GetHeader("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		in.ActivityType = c.PostForm("activity_type")
		if in.ActivityType == "" {
			in.ActivityType = string(domain.ActivityPhoto)
		}
		if msg := strings.TrimSpace(c.PostForm("message")); msg != "" {
			in.Message = &msg
		}
		if latStr := strings.TrimSpace(c.PostForm("latitude")); latStr != "" {
			if v, err := strconv.ParseFloat(latStr, 64); err == nil {
				in.Latitude = &v
			}
		}
		if lngStr := strings.TrimSpace(c.PostForm("longitude")); lngStr != "" {
			if v, err := strconv.ParseFloat(lngStr, 64); err == nil {
				in.Longitude = &v
			}
		}
		if fh, err := c.FormFile("photo"); err == nil && fh != nil {
			if h.storage == nil {
				httperr.Write(c, errors.New("storage belum dikonfigurasi"))
				return
			}
			f, openErr := fh.Open()
			if openErr != nil {
				httperr.Write(c, openErr)
				return
			}
			res, upErr := h.storage.Upload(c.Request.Context(), "tickets", f)
			_ = f.Close()
			if upErr != nil {
				if errors.Is(upErr, storage.ErrFileTooLarge) ||
					errors.Is(upErr, storage.ErrInvalidMimeType) ||
					errors.Is(upErr, storage.ErrEmptyFile) {
					httperr.Write(c, &domain.ValidationError{
						Message: upErr.Error(),
						Errors:  []domain.FieldError{{Field: "photo", Message: upErr.Error()}},
					})
					return
				}
				httperr.Write(c, upErr)
				return
			}
			uploadedKey = res.Key
			in.PhotoPath = &res.Key
		}
	} else {
		var req ticketActivityJSONRequest
		if !httperr.BindJSON(c, &req) {
			return
		}
		in = usecase.AddTicketActivityInput{
			ActivityType: req.ActivityType,
			Message:      req.Message,
			Latitude:     req.Latitude,
			Longitude:    req.Longitude,
			PhotoPath:    req.PhotoPath,
		}
	}

	act, err := h.uc.AddActivity(c.Request.Context(), actor, id, in)
	if err != nil {
		if uploadedKey != "" && h.storage != nil {
			_ = h.storage.Delete(c.Request.Context(), uploadedKey)
		}
		httperr.Write(c, err)
		return
	}
	response.Created(c, ticketActivityDTO(act))
}

type requestSparepartRequest struct {
	ProductID  int64   `json:"product_id" binding:"required"`
	Qty        int     `json:"qty" binding:"required,min=1"`
	IsBillable bool    `json:"is_billable"`
	Notes      *string `json:"notes"`
}

type createSparepartLPBRequest struct {
	WarehouseID int64   `json:"warehouse_id" binding:"required"`
	RackID      int64   `json:"rack_id" binding:"required"`
	Notes       *string `json:"notes"`
}

// RequestSparepart — POST /api/v1/tickets/:id/spareparts (T3.9).
func (h *TicketHandler) RequestSparepart(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req requestSparepartRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	det, err := h.uc.RequestSparepart(c.Request.Context(), actor, id, usecase.RequestSparepartInput{
		ProductID:  req.ProductID,
		Qty:        req.Qty,
		IsBillable: req.IsBillable,
		Notes:      req.Notes,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, ticketDetailDTO(det))
}

// CreateSparepartLPB — POST /api/v1/ticket-spareparts/:id/lpb (T3.9).
func (h *TicketHandler) CreateSparepartLPB(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	spID, ok := parseID(c)
	if !ok {
		return
	}
	var req createSparepartLPBRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	lpbDoc, err := h.uc.CreateSparepartLPB(c.Request.Context(), actor, spID, usecase.CreateSparepartLPBInput{
		WarehouseID: req.WarehouseID,
		RackID:      req.RackID,
		Notes:       req.Notes,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, gin.H{
		"lpb_id":     lpbDoc.ID,
		"lpb_number": lpbDoc.Number,
		"status":     lpbDoc.Status,
	})
}

func ticketDTO(t *domain.MaintenanceTicket) gin.H {
	slaBreached := t.SLADueAt != nil &&
		t.Status != domain.TicketResolved &&
		t.Status != domain.TicketClosed &&
		t.Status != domain.TicketCancelled &&
		time.Now().After(*t.SLADueAt)
	return gin.H{
		"id":                t.ID,
		"ticket_number":     t.TicketNumber,
		"customer_id":       t.CustomerID,
		"customer_name":     t.CustomerName,
		"customer_site_id":  t.CustomerSiteID,
		"site_name":         t.SiteName,
		"site_address":      t.SiteAddress,
		"site_city":         t.SiteCity,
		"product_serial_id": t.ProductSerialID,
		"asset_tag":         t.AssetTag,
		"serial_number":     t.SerialNumber,
		"product_name":      t.ProductName,
		"reported_by_name":  t.ReportedByName,
		"reported_via":      t.ReportedVia,
		"category":          t.Category,
		"priority":          t.Priority,
		"status":            t.Status,
		"title":             t.Title,
		"description":       t.Description,
		"assigned_to":       t.AssignedTo,
		"assigned_to_name":  t.AssignedToName,
		"sla_due_at":        t.SLADueAt,
		"sla_breached":      slaBreached,
		"resolution_notes":  t.ResolutionNotes,
		"resolved_at":       t.ResolvedAt,
		"closed_at":         t.ClosedAt,
		"swap_so_id":        t.SwapSOID,
		"swap_so_number":    t.SwapSONumber,
		"created_by":        t.CreatedBy,
		"created_by_name":   t.CreatedByName,
		"created_at":        t.CreatedAt,
		"updated_at":        t.UpdatedAt,
	}
}

func ticketActivityDTO(a *domain.TicketActivity) gin.H {
	return gin.H{
		"id":            a.ID,
		"ticket_id":     a.TicketID,
		"user_id":       a.UserID,
		"user_name":     a.UserName,
		"activity_type": a.ActivityType,
		"message":       a.Message,
		"from_status":   a.FromStatus,
		"to_status":     a.ToStatus,
		"latitude":      a.Latitude,
		"longitude":     a.Longitude,
		"photo_path":    a.PhotoPath,
		"photo_url":     a.PhotoURL,
		"created_at":    a.CreatedAt,
	}
}

func ticketDetailDTO(d *domain.TicketDetail) gin.H {
	base := ticketDTO(&d.MaintenanceTicket)
	acts := make([]gin.H, 0, len(d.Activities))
	for i := range d.Activities {
		acts = append(acts, ticketActivityDTO(&d.Activities[i]))
	}
	sps := make([]gin.H, 0, len(d.Spareparts))
	for _, sp := range d.Spareparts {
		sps = append(sps, gin.H{
			"id":                sp.ID,
			"ticket_id":         sp.TicketID,
			"product_id":        sp.ProductID,
			"sku":               sp.SKU,
			"product_name":      sp.ProductName,
			"qty":               sp.Qty,
			"status":            sp.Status,
			"lpb_id":            sp.LPBID,
			"lpb_number":        sp.LPBNumber,
			"is_billable":       sp.IsBillable,
			"requested_by":      sp.RequestedBy,
			"requested_by_name": sp.RequestedByName,
			"requested_at":      sp.RequestedAt,
			"notes":             sp.Notes,
		})
	}
	base["activities"] = acts
	base["spareparts"] = sps
	return base
}
