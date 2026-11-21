package httperr

import (
	"errors"
	nethttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"rental-inventory-api/pkg/response"
)

// maxJSONBody membatasi body JSON biasa. Upload file memakai handler multipart tersendiri.
const maxJSONBody = 1 << 20 // 1 MiB

// BindJSON mem-bind + memvalidasi body JSON ke dst. Bila gagal, respons 400/413 sudah
// ditulis dan fungsi mengembalikan false — handler cukup `return`.
func BindJSON(c *gin.Context, dst any) bool {
	c.Request.Body = nethttp.MaxBytesReader(c.Writer, c.Request.Body, maxJSONBody)
	if err := c.ShouldBindJSON(dst); err != nil {
		Write(c, err)
		return false
	}
	return true
}

// BindQuery mem-bind + memvalidasi query string ke dst (tag `form`).
func BindQuery(c *gin.Context, dst any) bool {
	if err := c.ShouldBindQuery(dst); err != nil {
		var vErrs validator.ValidationErrors
		if errors.As(err, &vErrs) {
			Write(c, err)
		} else { // gagal konversi tipe (mis. angka) → pesan generik, detail teknis tidak dibocorkan
			response.Error(c, nethttp.StatusBadRequest, CodeBadRequest, "Parameter query tidak valid", nil)
		}
		return false
	}
	return true
}
