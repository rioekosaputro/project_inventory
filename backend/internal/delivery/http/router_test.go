package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/config"
)

func testConfig() *config.Config {
	return &config.Config{AppEnv: config.EnvLocal, CORSAllowedOrigins: []string{"http://localhost:5173"}}
}

func newTestRouter(checks ...ReadinessCheck) nethttp.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(testConfig(), logger, checks...)
}

func doGet(t *testing.T, h nethttp.Handler, path string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(nethttp.MethodGet, path, nil))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return rec.Code, body
}

func TestHealthz(t *testing.T) {
	code, body := doGet(t, newTestRouter(), "/healthz")

	assert.Equal(t, nethttp.StatusOK, code)
	assert.Equal(t, "ok", body["status"])
}

func TestReadyz_NoChecks_IsReady(t *testing.T) {
	code, body := doGet(t, newTestRouter(), "/readyz")

	assert.Equal(t, nethttp.StatusOK, code)
	assert.Equal(t, "ready", body["status"])
}

func TestReadyz_ReportsFailingCheck(t *testing.T) {
	up := ReadinessCheck{Name: "cache", Check: func(context.Context) error { return nil }}
	down := ReadinessCheck{Name: "database", Check: func(context.Context) error { return errors.New("koneksi ditolak") }}

	code, body := doGet(t, newTestRouter(up, down), "/readyz")

	assert.Equal(t, nethttp.StatusServiceUnavailable, code)
	assert.Equal(t, "not_ready", body["status"])
	checks := body["checks"].(map[string]any)
	assert.Equal(t, "up", checks["cache"])
	assert.Equal(t, "down", checks["database"])
}

func TestUnknownRoute_Returns404InStandardFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, httptest.NewRequest(nethttp.MethodGet, "/tidak-ada", nil))

	assert.Equal(t, nethttp.StatusNotFound, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, false, body["success"])
	assert.Equal(t, "NOT_FOUND", body["code"])
	assert.Equal(t, rec.Header().Get("X-Request-ID"), body["request_id"])
}

func TestMethodNotAllowed_Returns405InStandardFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, httptest.NewRequest(nethttp.MethodPost, "/healthz", nil))

	assert.Equal(t, nethttp.StatusMethodNotAllowed, rec.Code)
	assert.Contains(t, rec.Body.String(), "METHOD_NOT_ALLOWED")
}

func TestSecurityHeaders_PresentEvenOnErrors(t *testing.T) {
	for _, path := range []string{"/healthz", "/tidak-ada"} {
		rec := httptest.NewRecorder()
		newTestRouter().ServeHTTP(rec, httptest.NewRequest(nethttp.MethodGet, path, nil))

		h := rec.Header()
		assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"), path)
		assert.Equal(t, "DENY", h.Get("X-Frame-Options"), path)
		assert.Equal(t, "no-store", h.Get("Cache-Control"), path)
		assert.Contains(t, h.Get("Content-Security-Policy"), "default-src 'none'", path)
		assert.Empty(t, h.Get("Strict-Transport-Security"), "HSTS tidak dipasang di lokal")
	}
}

func TestHSTS_EnabledOutsideLocal(t *testing.T) {
	cfg := testConfig()
	cfg.AppEnv = config.EnvStaging
	r := NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(nethttp.MethodGet, "/healthz", nil))

	assert.Contains(t, rec.Header().Get("Strict-Transport-Security"), "max-age=")
}

func TestCORS_AllowedOriginGetsHeadersAndPreflightWorks(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest(nethttp.MethodOptions, "/healthz", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "authorization,idempotency-key")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, nethttp.StatusNoContent, rec.Code)
	assert.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
	assert.Contains(t, strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers")), "idempotency-key")
}

func TestCORS_UnknownOriginGetsNoAllowHeader(t *testing.T) {
	req := httptest.NewRequest(nethttp.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://jahat.example")
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
}

func TestRequestID_GeneratedOrReusedWhenValidOrReplacedWhenMalicious(t *testing.T) {
	r := newTestRouter()
	get := func(incoming string) string {
		req := httptest.NewRequest(nethttp.MethodGet, "/healthz", nil)
		if incoming != "" {
			req.Header.Set("X-Request-ID", incoming)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Header().Get("X-Request-ID")
	}

	generated := get("")
	assert.Len(t, generated, 32)
	assert.NotEqual(t, generated, get(""), "id harus unik per request")

	assert.Equal(t, "trace-abc_12345", get("trace-abc_12345"), "id valid dari proxy dipakai ulang")

	for _, bad := range []string{"pendek", "ada spasi di sini ya", "x<script>alert(1)</script>", strings.Repeat("a", 65)} {
		got := get(bad)
		assert.NotEqual(t, bad, got, "id tidak valid harus diganti: %q", bad)
		assert.Len(t, got, 32)
	}
}

func TestAccessLog_RecordsRequestIDStatusAndOmitsQueryString(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := NewRouter(testConfig(), logger)

	req := httptest.NewRequest(nethttp.MethodGet, "/tidak-ada?token=super-rahasia", nil)
	req.Header.Set("X-Request-ID", "req-logtest-0001")
	r.ServeHTTP(httptest.NewRecorder(), req)

	line := buf.String()
	assert.Contains(t, line, `"request_id":"req-logtest-0001"`)
	assert.Contains(t, line, `"status":404`)
	assert.Contains(t, line, `"level":"WARN"`)
	assert.Contains(t, line, `"path":"/tidak-ada"`)
	assert.NotContains(t, line, "super-rahasia", "query string tidak boleh masuk log")
}

func TestAccessLog_HealthChecksAreDebugLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := NewRouter(testConfig(), logger)

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(nethttp.MethodGet, "/healthz", nil))

	assert.Contains(t, buf.String(), `"level":"DEBUG"`)
}

func TestClientIP_IgnoresSpoofedForwardedForWhenNoTrustedProxy(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r := NewRouter(testConfig(), logger)

	req := httptest.NewRequest(nethttp.MethodGet, "/tidak-ada", nil)
	req.RemoteAddr = "203.0.113.9:4321"
	req.Header.Set("X-Forwarded-For", "6.6.6.6")
	r.ServeHTTP(httptest.NewRecorder(), req)

	assert.Contains(t, buf.String(), `"ip":"203.0.113.9"`)
	assert.NotContains(t, buf.String(), "6.6.6.6")
}

func TestClientIP_HonoursForwardedForFromTrustedProxy(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	cfg := testConfig()
	cfg.TrustedProxies = []string{"10.0.0.0/8"}
	r := NewRouter(cfg, logger)

	req := httptest.NewRequest(nethttp.MethodGet, "/tidak-ada", nil)
	req.RemoteAddr = "10.1.2.3:4321"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	r.ServeHTTP(httptest.NewRecorder(), req)

	assert.Contains(t, buf.String(), `"ip":"198.51.100.7"`)
}
