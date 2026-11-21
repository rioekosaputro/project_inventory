package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/config"
)

func newTestRouter(checks ...ReadinessCheck) nethttp.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(&config.Config{AppEnv: config.EnvLocal}, logger, checks...)
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

func TestUnknownRoute_Returns404(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, httptest.NewRequest(nethttp.MethodGet, "/tidak-ada", nil))

	assert.Equal(t, nethttp.StatusNotFound, rec.Code)
}
