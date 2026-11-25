package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/response"
)

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
