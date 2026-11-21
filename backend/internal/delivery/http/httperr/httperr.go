// Package httperr memetakan error (domain, validasi, panic tak terduga) ke respons HTTP standar.
// Pemetaan HTTP HANYA boleh ada di sini; usecase tidak tahu kode status.
package httperr

import (
	"errors"
	"log/slog"
	nethttp "net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/pagination"
	"rental-inventory-api/pkg/reqctx"
	"rental-inventory-api/pkg/response"
)

const (
	CodeInternal         = "INTERNAL_ERROR"
	CodeBadRequest       = "BAD_REQUEST"
	CodeValidation       = "VALIDATION_ERROR"
	CodeInvalidData      = "INVALID_DATA"
	CodePayloadTooLarge  = "PAYLOAD_TOO_LARGE"
	CodeNotFound         = "NOT_FOUND"
	CodeDuplicate        = "DUPLICATE_VALUE"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"

	messageInternal = "Terjadi kesalahan pada server"
)

type mapped struct {
	err    error
	status int
	code   string
}

// mapping diperiksa berurutan dengan errors.Is (plan Bagian 7.4).
var mapping = []mapped{
	{domain.ErrNotFound, nethttp.StatusNotFound, "NOT_FOUND"},
	{domain.ErrLPBNotFound, nethttp.StatusNotFound, "LPB_NOT_FOUND"},
	{domain.ErrUnauthorized, nethttp.StatusUnauthorized, "UNAUTHORIZED"},
	{domain.ErrInvalidCredentials, nethttp.StatusUnauthorized, "INVALID_CREDENTIALS"},
	{domain.ErrInvalidToken, nethttp.StatusUnauthorized, "INVALID_TOKEN"},
	{domain.ErrAccountDisabled, nethttp.StatusForbidden, "ACCOUNT_DISABLED"},
	{domain.ErrPasswordChangeRequired, nethttp.StatusForbidden, "PASSWORD_CHANGE_REQUIRED"},
	{domain.ErrTooManyRequests, nethttp.StatusTooManyRequests, "TOO_MANY_REQUESTS"},
	{domain.ErrForbidden, nethttp.StatusForbidden, "FORBIDDEN"},
	{domain.ErrSelfApproveForbidden, nethttp.StatusForbidden, "SELF_APPROVE_FORBIDDEN"},
	{domain.ErrForbiddenWarehouse, nethttp.StatusForbidden, "FORBIDDEN_WAREHOUSE"},
	{domain.ErrConflict, nethttp.StatusConflict, "CONFLICT"},
	{domain.ErrEmailTaken, nethttp.StatusConflict, "EMAIL_TAKEN"},
	{domain.ErrLastAdmin, nethttp.StatusConflict, "LAST_ADMIN"},
	{domain.ErrLPBNotPending, nethttp.StatusConflict, "LPB_NOT_PENDING"},
	{domain.ErrInsufficientStock, nethttp.StatusConflict, "INSUFFICIENT_STOCK"},
	{domain.ErrSerialAlreadyExists, nethttp.StatusConflict, "SERIAL_EXISTS"},
	{domain.ErrRackLocked, nethttp.StatusConflict, "RACK_LOCKED_BY_OPNAME"},
	{domain.ErrReservationMismatch, nethttp.StatusConflict, "RESERVATION_MISMATCH"},
	{domain.ErrSerialInOtherLPB, nethttp.StatusConflict, "SERIAL_IN_OTHER_LPB"},
	{domain.ErrInvalidStateTransition, nethttp.StatusConflict, "INVALID_STATE"},
	{domain.ErrLocationHasStock, nethttp.StatusConflict, "LOCATION_HAS_STOCK"},
	{domain.ErrProductInUse, nethttp.StatusConflict, "PRODUCT_IN_USE"},
	{domain.ErrCategoryInUse, nethttp.StatusConflict, "CATEGORY_IN_USE"},
	{domain.ErrSerialCountMismatch, nethttp.StatusUnprocessableEntity, "SERIAL_COUNT_MISMATCH"},
	{domain.ErrSerialNotValid, nethttp.StatusUnprocessableEntity, "SERIAL_NOT_VALID"},
	{domain.ErrLPBEmptyItems, nethttp.StatusUnprocessableEntity, "LPB_EMPTY"},
}

// Write menerjemahkan err menjadi respons JSON dan menghentikan rantai handler.
// Error yang tidak dikenali → 500 dengan pesan generik; detailnya hanya masuk log
// (bersama request_id) dan tidak pernah dikirim ke klien.
func Write(c *gin.Context, err error) {
	if err == nil {
		return
	}

	// Parameter query (page/limit/sort).
	var pErr *pagination.Error
	if errors.As(err, &pErr) {
		response.Error(c, nethttp.StatusBadRequest, CodeValidation, "Parameter query tidak valid",
			[]response.FieldError{{Field: pErr.Field, Message: pErr.Message}})
		return
	}

	// Validasi DTO (binding) dan kesalahan JSON.
	if status, code, msg, fields, ok := translateBindError(err); ok {
		response.Error(c, status, code, msg, fields)
		return
	}

	// Validasi aturan bisnis dari usecase.
	var vErr *domain.ValidationError
	if errors.As(err, &vErr) {
		fields := make([]response.FieldError, 0, len(vErr.Errors))
		for _, f := range vErr.Errors {
			fields = append(fields, response.FieldError{Field: f.Field, Message: f.Message})
		}
		response.Error(c, nethttp.StatusUnprocessableEntity, CodeInvalidData, vErr.Message, fields)
		return
	}

	// Nilai unik bentrok: 409 dengan nama field agar form bisa menandai kolomnya.
	var dErr *domain.DuplicateError
	if errors.As(err, &dErr) {
		response.Error(c, nethttp.StatusConflict, CodeDuplicate, dErr.Message,
			[]response.FieldError{{Field: dErr.Field, Message: dErr.Message}})
		return
	}

	// Rate limit: beri tahu klien kapan boleh mencoba lagi.
	var rlErr *domain.RateLimitError
	if errors.As(err, &rlErr) {
		secs := int((rlErr.RetryAfter + time.Second - 1) / time.Second)
		c.Header("Retry-After", strconv.Itoa(max(secs, 1)))
	}

	for _, m := range mapping {
		if errors.Is(err, m.err) {
			response.Error(c, m.status, m.code, err.Error(), nil)
			return
		}
	}

	slog.ErrorContext(c.Request.Context(), "error tak terduga",
		"error", err,
		"method", c.Request.Method,
		"path", c.FullPath(),
		"request_id", reqctx.RequestID(c.Request.Context()),
	)
	response.Error(c, nethttp.StatusInternalServerError, CodeInternal, messageInternal, nil)
}

// NotFound handler untuk route yang tidak ada.
func NotFound(c *gin.Context) {
	response.Error(c, nethttp.StatusNotFound, CodeNotFound, "Endpoint tidak ditemukan", nil)
}

// MethodNotAllowed handler untuk method yang tidak didukung route.
func MethodNotAllowed(c *gin.Context) {
	response.Error(c, nethttp.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method tidak diizinkan untuk endpoint ini", nil)
}
