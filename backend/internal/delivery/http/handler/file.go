package handler

import (
	"errors"
	"io"
	nethttp "net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/storage"
)

// FileHandler melayani berkas privat lokal melalui signed URL HMAC berumur pendek
// (`GET /api/v1/files?key=...&exp=...&sig=...`) saat STORAGE_DRIVER=local.
type FileHandler struct {
	local *storage.LocalStorage
}

func NewFileHandler(local *storage.LocalStorage) *FileHandler {
	return &FileHandler{local: local}
}

// Serve — GET /api/v1/files
func (h *FileHandler) Serve(c *gin.Context) {
	key := c.Query("key")
	sig := c.Query("sig")
	exp, err := strconv.ParseInt(c.Query("exp"), 10, 64)
	if err != nil || key == "" || sig == "" {
		httperr.Write(c, domain.ErrForbidden)
		return
	}

	verifiedKey, err := h.local.VerifySignedURL(key, exp, sig)
	if err != nil {
		httperr.Write(c, domain.ErrForbidden)
		return
	}

	rc, contentType, err := h.local.Open(c.Request.Context(), verifiedKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httperr.Write(c, domain.ErrNotFound)
			return
		}
		httperr.Write(c, err)
		return
	}
	defer rc.Close()

	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Type", contentType)
	c.Status(nethttp.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}
