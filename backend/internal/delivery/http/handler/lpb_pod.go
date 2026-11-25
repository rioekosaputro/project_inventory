package handler

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/dto"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/response"
)

type deliverLPBRequest struct {
	ReceiverName  string  `json:"receiver_name" binding:"required,min=2,max=100"`
	PODPhotoPath  *string `json:"pod_photo_path" binding:"omitempty,max=500"`
	DeliveredDate *string `json:"delivered_date"`
	Notes         *string `json:"notes" binding:"omitempty,max=500"`
}

type pickupLPBRequest struct {
	PickupDate *string `json:"pickup_date"`
	Notes      *string `json:"notes" binding:"omitempty,max=500"`
}

// Deliver — POST /api/v1/lpb/:id/deliver (POD manual oleh Kepala WH / Admin; mendukung JSON atau multipart foto).
func (h *LPBHandler) Deliver(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}

	var in usecase.DeliverLPBInput
	ct := c.GetHeader("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		in.ReceiverName = c.PostForm("receiver_name")
		if v := strings.TrimSpace(c.PostForm("delivered_date")); v != "" {
			in.DeliveredDate = &v
		}
		if v := strings.TrimSpace(c.PostForm("notes")); v != "" {
			in.Notes = &v
		}
		if fh, err := c.FormFile("photo"); err == nil && fh != nil {
			ext := strings.ToLower(filepath.Ext(fh.Filename))
			if ext == "" {
				ext = ".jpg"
			}
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			relDir := filepath.Join("uploads", "pod")
			_ = os.MkdirAll(relDir, 0o755)
			relPath := filepath.Join(relDir, hex.EncodeToString(buf)+ext)
			if saveErr := c.SaveUploadedFile(fh, relPath); saveErr != nil {
				httperr.Write(c, saveErr)
				return
			}
			norm := filepath.ToSlash(relPath)
			in.PODPhotoPath = &norm
		}
	} else {
		var req deliverLPBRequest
		if !httperr.BindJSON(c, &req) {
			return
		}
		in = usecase.DeliverLPBInput{
			ReceiverName:  req.ReceiverName,
			PODPhotoPath:  req.PODPhotoPath,
			DeliveredDate: req.DeliveredDate,
			Notes:         req.Notes,
		}
	}

	det, err := h.uc.Deliver(c.Request.Context(), actor, id, in)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewLPBDetailResponse(det))
}

// Pickup — POST /api/v1/lpb/:id/pickup (penjemputan unit retur oleh Kepala WH / Admin).
func (h *LPBHandler) Pickup(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req pickupLPBRequest
	if c.Request.ContentLength > 0 {
		if !httperr.BindJSON(c, &req) {
			return
		}
	}
	det, err := h.uc.Pickup(c.Request.Context(), actor, id, usecase.PickupLPBInput{
		PickupDate: req.PickupDate,
		Notes:      req.Notes,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewLPBDetailResponse(det))
}
