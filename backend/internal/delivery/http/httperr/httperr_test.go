package httperr_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/pagination"
	"rental-inventory-api/pkg/response"
)

type itemReq struct {
	ProductID int `json:"product_id" binding:"required,gte=1"`
	Qty       int `json:"qty" binding:"gte=1,lte=100"`
}

type createReq struct {
	Name  string    `json:"name" binding:"required,min=3"`
	Email string    `json:"email" binding:"required,email"`
	Kind  string    `json:"kind" binding:"oneof=A B"`
	Items []itemReq `json:"items" binding:"required,min=1,dive"`
}

type listQuery struct {
	Status string `form:"status" binding:"omitempty,oneof=DRAFT APPROVED"`
	Limit  int    `form:"limit"`
}

// newEngine memasang handler contoh yang mewakili pola handler tipis di plan Bagian 7.1.
func newEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	httperr.Setup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.NoRoute(httperr.NotFound)
	r.NoMethod(httperr.MethodNotAllowed)
	r.Use(middleware.RequestID(), middleware.Recover(logger))

	r.GET("/ok", func(c *gin.Context) { response.OK(c, gin.H{"id": 1}) })
	r.POST("/created", func(c *gin.Context) { response.Created(c, gin.H{"id": 2}) })
	r.GET("/list", func(c *gin.Context) {
		var nilSlice []string
		p, err := pagination.FromQuery(c.Request.URL.Query(), pagination.SortSpec{Allowed: map[string]string{"id": "id"}, Default: "id"})
		if err != nil {
			httperr.Write(c, err)
			return
		}
		response.Paginated(c, nilSlice, pagination.NewMeta(p, 45))
	})
	r.POST("/items", func(c *gin.Context) {
		var req createReq
		if !httperr.BindJSON(c, &req) {
			return
		}
		response.Created(c, req)
	})
	r.GET("/query", func(c *gin.Context) {
		var q listQuery
		if !httperr.BindQuery(c, &q) {
			return
		}
		response.OK(c, q)
	})
	r.GET("/err", func(c *gin.Context) {
		httperr.Write(c, errors.New(c.Query("e")))
	})
	r.GET("/domain", func(c *gin.Context) {
		switch c.Query("e") {
		case "notfound":
			httperr.Write(c, domain.ErrNotFound)
		case "stock":
			httperr.Write(c, fmt.Errorf("%w: rak A-01 tersisa 3", domain.ErrInsufficientStock))
		case "count":
			httperr.Write(c, domain.ErrSerialCountMismatch)
		case "forbidden":
			httperr.Write(c, domain.ErrForbiddenWarehouse)
		case "unauth":
			httperr.Write(c, domain.ErrUnauthorized)
		case "validation":
			httperr.Write(c, &domain.ValidationError{
				Message: "Item LPB tidak valid",
				Errors:  []domain.FieldError{{Field: "items[0].qty", Message: "melebihi stok tersedia (10)"}},
			})
		case "wrapped-validation":
			httperr.Write(c, fmt.Errorf("approve gagal: %w", &domain.ValidationError{Message: "Tidak valid"}))
		}
	})
	r.GET("/boom", func(c *gin.Context) {
		httperr.Write(c, errors.New(`pq: connection to "db.internal:5432" refused (password=hunter2)`))
	})
	r.GET("/panic", func(c *gin.Context) { panic("sesuatu meledak: rahasia-internal") })
	return r
}

type envelope struct {
	Success   bool                  `json:"success"`
	Message   string                `json:"message"`
	Code      string                `json:"code"`
	Data      json.RawMessage       `json:"data"`
	Meta      map[string]any        `json:"meta"`
	Errors    []response.FieldError `json:"errors"`
	RequestID string                `json:"request_id"`
}

func do(t *testing.T, r *gin.Engine, method, path, body string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var env envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), "body: %s", rec.Body.String())
	return rec, env
}

// --- DoD T0.5: 200 / 400 / 404 / 500 sesuai format Bagian 7.4 ---

func TestFormat_200(t *testing.T) {
	rec, env := do(t, newEngine(t), "GET", "/ok", "")

	assert.Equal(t, 200, rec.Code)
	assert.True(t, env.Success)
	assert.Equal(t, "Berhasil", env.Message)
	assert.JSONEq(t, `{"id":1}`, string(env.Data))
	assert.Empty(t, env.Code)
	assert.Empty(t, env.Errors)
}

func TestFormat_201(t *testing.T) {
	rec, env := do(t, newEngine(t), "POST", "/created", "")
	assert.Equal(t, 201, rec.Code)
	assert.True(t, env.Success)
}

func TestFormat_400_ValidationWithIndonesianFieldMessages(t *testing.T) {
	body := `{"name":"ab","email":"bukan-email","kind":"Z","items":[{"product_id":1,"qty":0},{"product_id":0,"qty":101}]}`
	rec, env := do(t, newEngine(t), "POST", "/items", body)

	assert.Equal(t, 400, rec.Code)
	assert.False(t, env.Success)
	assert.Equal(t, "VALIDATION_ERROR", env.Code)
	assert.Equal(t, "Data yang dikirim tidak valid", env.Message)
	assert.NotEmpty(t, env.RequestID)
	assert.Equal(t, rec.Header().Get("X-Request-ID"), env.RequestID)

	got := map[string]string{}
	for _, e := range env.Errors {
		got[e.Field] = e.Message
	}
	assert.Equal(t, map[string]string{
		"name":                "minimal 3 karakter",
		"email":               "format email tidak valid",
		"kind":                "harus salah satu dari: A, B",
		"items[0].qty":        "minimal 1",
		"items[1].product_id": "wajib diisi",
		"items[1].qty":        "maksimal 100",
	}, got)
}

func TestFormat_400_RequiredFieldsMissing(t *testing.T) {
	_, env := do(t, newEngine(t), "POST", "/items", `{}`)

	got := map[string]string{}
	for _, e := range env.Errors {
		got[e.Field] = e.Message
	}
	assert.Equal(t, "wajib diisi", got["name"])
	assert.Equal(t, "wajib diisi", got["email"])
	assert.Equal(t, "wajib diisi", got["items"])
}

func TestFormat_400_MalformedJSONAndEmptyBodyAndWrongType(t *testing.T) {
	r := newEngine(t)

	rec, env := do(t, r, "POST", "/items", `{"name":`)
	assert.Equal(t, 400, rec.Code)
	assert.Equal(t, "BAD_REQUEST", env.Code)
	assert.Equal(t, "Format JSON tidak valid", env.Message)

	rec, env = do(t, r, "POST", "/items", ``)
	assert.Equal(t, 400, rec.Code)
	assert.Equal(t, "Body request kosong", env.Message)

	rec, env = do(t, r, "POST", "/items", `{"name":123}`)
	assert.Equal(t, 400, rec.Code)
	assert.Equal(t, "VALIDATION_ERROR", env.Code)
	require.Len(t, env.Errors, 1)
	assert.Equal(t, "name", env.Errors[0].Field)
	assert.Equal(t, "harus berupa teks", env.Errors[0].Message)
}

func TestFormat_413_BodyTooLarge(t *testing.T) {
	big := `{"name":"` + strings.Repeat("a", 2<<20) + `"}`
	rec, env := do(t, newEngine(t), "POST", "/items", big)

	assert.Equal(t, 413, rec.Code)
	assert.Equal(t, "PAYLOAD_TOO_LARGE", env.Code)
}

func TestFormat_400_QueryBinding(t *testing.T) {
	r := newEngine(t)

	rec, env := do(t, r, "GET", "/query?status=BUKAN", "")
	assert.Equal(t, 400, rec.Code)
	assert.Equal(t, "status", env.Errors[0].Field)
	assert.Equal(t, "harus salah satu dari: DRAFT, APPROVED", env.Errors[0].Message)

	// Gagal konversi tipe: pesan generik, detail strconv tidak bocor.
	rec, env = do(t, r, "GET", "/query?limit=abc", "")
	assert.Equal(t, 400, rec.Code)
	assert.Equal(t, "Parameter query tidak valid", env.Message)
	assert.NotContains(t, rec.Body.String(), "strconv")
}

func TestFormat_400_PaginationQuery(t *testing.T) {
	r := newEngine(t)

	rec, env := do(t, r, "GET", "/list?page=0", "")
	assert.Equal(t, 400, rec.Code)
	assert.Equal(t, "VALIDATION_ERROR", env.Code)
	assert.Equal(t, "page", env.Errors[0].Field)

	rec, env = do(t, r, "GET", "/list?sort=password_hash", "")
	assert.Equal(t, 400, rec.Code)
	assert.Equal(t, "sort", env.Errors[0].Field)
}

func TestFormat_404_UnknownRouteAndDomainNotFound(t *testing.T) {
	r := newEngine(t)

	rec, env := do(t, r, "GET", "/tidak-ada", "")
	assert.Equal(t, 404, rec.Code)
	assert.False(t, env.Success)
	assert.Equal(t, "NOT_FOUND", env.Code)
	assert.NotEmpty(t, env.RequestID)

	rec, env = do(t, r, "GET", "/domain?e=notfound", "")
	assert.Equal(t, 404, rec.Code)
	assert.Equal(t, "NOT_FOUND", env.Code)
	assert.Equal(t, "Data tidak ditemukan", env.Message)
}

func TestFormat_405(t *testing.T) {
	rec, env := do(t, newEngine(t), "DELETE", "/ok", "")
	assert.Equal(t, 405, rec.Code)
	assert.Equal(t, "METHOD_NOT_ALLOWED", env.Code)
}

func TestFormat_500_UnknownErrorDoesNotLeakDetails(t *testing.T) {
	rec, env := do(t, newEngine(t), "GET", "/boom", "")

	assert.Equal(t, 500, rec.Code)
	assert.False(t, env.Success)
	assert.Equal(t, "INTERNAL_ERROR", env.Code)
	assert.Equal(t, "Terjadi kesalahan pada server", env.Message)
	assert.NotEmpty(t, env.RequestID)
	assert.NotContains(t, rec.Body.String(), "hunter2")
	assert.NotContains(t, rec.Body.String(), "db.internal")
}

func TestFormat_500_PanicIsRecoveredWithoutLeakingDetails(t *testing.T) {
	rec, env := do(t, newEngine(t), "GET", "/panic", "")

	assert.Equal(t, 500, rec.Code)
	assert.Equal(t, "INTERNAL_ERROR", env.Code)
	assert.NotEmpty(t, env.RequestID)
	assert.NotContains(t, rec.Body.String(), "rahasia-internal")
	assert.NotContains(t, rec.Body.String(), "goroutine")
}

// --- pemetaan error domain ---

func TestDomainErrorMapping(t *testing.T) {
	r := newEngine(t)
	tests := []struct {
		query  string
		status int
		code   string
		msg    string
	}{
		{"stock", 409, "INSUFFICIENT_STOCK", "Stok di rak tidak mencukupi: rak A-01 tersisa 3"},
		{"count", 422, "SERIAL_COUNT_MISMATCH", "Jumlah serial number tidak sama dengan qty"},
		{"forbidden", 403, "FORBIDDEN_WAREHOUSE", "Anda tidak memiliki akses ke gudang ini"},
		{"unauth", 401, "UNAUTHORIZED", "Autentikasi diperlukan"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			rec, env := do(t, r, "GET", "/domain?e="+tt.query, "")
			assert.Equal(t, tt.status, rec.Code)
			assert.Equal(t, tt.code, env.Code)
			assert.Equal(t, tt.msg, env.Message)
		})
	}
}

func TestDomainValidationError_Is422WithFieldDetails(t *testing.T) {
	r := newEngine(t)

	rec, env := do(t, r, "GET", "/domain?e=validation", "")
	assert.Equal(t, 422, rec.Code)
	assert.Equal(t, "INVALID_DATA", env.Code)
	assert.Equal(t, "Item LPB tidak valid", env.Message)
	assert.Equal(t, []response.FieldError{{Field: "items[0].qty", Message: "melebihi stok tersedia (10)"}}, env.Errors)

	rec, env = do(t, r, "GET", "/domain?e=wrapped-validation", "")
	assert.Equal(t, 422, rec.Code, "ValidationError yang dibungkus harus tetap dikenali")
	assert.Equal(t, "INVALID_DATA", env.Code)
}

func TestMapping_CoversEveryDomainSentinel(t *testing.T) {
	// Setiap sentinel di domain wajib punya pemetaan; jika tidak, ia akan menjadi 500.
	sentinels := []error{
		domain.ErrNotFound, domain.ErrUnauthorized, domain.ErrForbidden, domain.ErrConflict,
		domain.ErrLPBNotFound, domain.ErrLPBNotPending, domain.ErrInsufficientStock, domain.ErrSerialAlreadyExists,
		domain.ErrRackLocked, domain.ErrReservationMismatch, domain.ErrSerialInOtherLPB, domain.ErrSerialCountMismatch,
		domain.ErrSerialNotValid, domain.ErrLPBEmptyItems, domain.ErrSelfApproveForbidden,
		domain.ErrForbiddenWarehouse, domain.ErrInvalidStateTransition,
	}
	gin.SetMode(gin.TestMode)
	for _, s := range sentinels {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest("GET", "/", nil)

		httperr.Write(c, s)

		assert.NotEqual(t, nethttp.StatusInternalServerError, rec.Code, "tidak terpetakan: %v", s)
	}
}

// --- pagination / meta ---

func TestPaginated_NilSliceBecomesEmptyArrayAndMetaPresent(t *testing.T) {
	rec, env := do(t, newEngine(t), "GET", "/list?page=2&limit=20", "")

	assert.Equal(t, 200, rec.Code)
	assert.JSONEq(t, `[]`, string(env.Data), "slice nil harus menjadi [], bukan null")
	assert.EqualValues(t, 2, env.Meta["page"])
	assert.EqualValues(t, 20, env.Meta["limit"])
	assert.EqualValues(t, 45, env.Meta["total"])
	assert.EqualValues(t, 3, env.Meta["total_pages"])
}
