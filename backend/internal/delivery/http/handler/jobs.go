package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/pagination"
	"rental-inventory-api/pkg/response"
)

var auditSort = pagination.SortSpec{
	Allowed: map[string]string{
		"created_at":  "a.created_at",
		"action":      "a.action",
		"entity_type": "a.entity_type",
	},
	Default: "-created_at",
}

type JobHandler struct {
	uc *usecase.JobUsecase
}

func NewJobHandler(uc *usecase.JobUsecase) *JobHandler {
	return &JobHandler{uc: uc}
}

// RunBilling menangani GET /api/v1/internal/jobs/billing (Vercel Cron / Bearer CRON_SECRET).
func (h *JobHandler) RunBilling(c *gin.Context) {
	var datePtr *string
	if d := strings.TrimSpace(c.Query("date")); d != "" {
		datePtr = &d
	}
	res, err := h.uc.RunBilling(c.Request.Context(), datePtr)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, res)
}

// RunDaily menangani GET /api/v1/internal/jobs/daily (Vercel Cron / Bearer CRON_SECRET).
func (h *JobHandler) RunDaily(c *gin.Context) {
	var datePtr *string
	if d := strings.TrimSpace(c.Query("date")); d != "" {
		datePtr = &d
	}
	res, err := h.uc.RunDaily(c.Request.Context(), datePtr)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, res)
}

// Reconcile menangani GET /api/v1/admin/reconcile (pemeriksaan invarian stok I1–I6 oleh Admin, T4.2).
func (h *JobHandler) Reconcile(c *gin.Context) {
	rep, err := h.uc.CheckReconcile(c.Request.Context())
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, rep)
}

// ListJobs menangani GET /api/v1/admin/jobs (riwayat eksekusi job terjadwal untuk Admin, T4.3).
func (h *JobHandler) ListJobs(c *gin.Context) {
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			httperr.BadQueryField(c, "limit", "harus angka 1 sampai 200")
			return
		}
		limit = n
	}
	items, err := h.uc.ListJobRuns(c.Request.Context(), limit)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, items)
}

// ListAuditLogs menangani GET /api/v1/admin/audit-logs (Audit Log Viewer untuk Admin & Manajemen, T4.3).
func (h *JobHandler) ListAuditLogs(c *gin.Context) {
	page, err := pagination.FromQuery(c.Request.URL.Query(), auditSort)
	if err != nil {
		httperr.Write(c, err)
		return
	}

	filter := domain.AuditFilter{
		Query:      strings.TrimSpace(c.Query("q")),
		Action:     strings.TrimSpace(c.Query("action")),
		EntityType: strings.TrimSpace(c.Query("entity_type")),
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			httperr.BadQueryField(c, "user_id", "harus ID pengguna positif")
			return
		}
		filter.UserID = &id
	}
	if raw := strings.TrimSpace(c.Query("date_from")); raw != "" {
		t, err := time.ParseInLocation("2006-01-02", raw, clock.WIB)
		if err != nil {
			httperr.BadQueryField(c, "date_from", "format tanggal harus YYYY-MM-DD")
			return
		}
		filter.DateFrom = &t
	}
	if raw := strings.TrimSpace(c.Query("date_to")); raw != "" {
		t, err := time.ParseInLocation("2006-01-02", raw, clock.WIB)
		if err != nil {
			httperr.BadQueryField(c, "date_to", "format tanggal harus YYYY-MM-DD")
			return
		}
		next := t.AddDate(0, 0, 1)
		filter.DateTo = &next
	}

	items, total, err := h.uc.ListAuditLogs(c.Request.Context(), filter, domain.PageParams{
		Limit:   page.Limit,
		Offset:  page.Offset(),
		OrderBy: page.OrderBy(),
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Paginated(c, items, pagination.NewMeta(page, total))
}
