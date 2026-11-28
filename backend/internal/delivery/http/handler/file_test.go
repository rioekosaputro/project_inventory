package handler_test

import (
	"bytes"
	"context"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/delivery/http/handler"
	"rental-inventory-api/pkg/storage"
)

func TestFileHandler_Serve(t *testing.T) {
	gin.SetMode(gin.TestMode)

	loc, err := storage.NewLocal(storage.LocalConfig{
		BaseDir:       t.TempDir(),
		SigningSecret: "test-signing-secret-minimum-32-chars",
	})
	require.NoError(t, err)

	jpegBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	res, err := loc.Upload(context.Background(), "pod", bytes.NewReader(jpegBytes))
	require.NoError(t, err)

	r := gin.New()
	r.GET("/api/v1/files", handler.NewFileHandler(loc).Serve)

	signedURL, err := loc.SignedURL(context.Background(), res.Key, 5*time.Minute)
	require.NoError(t, err)

	t.Run("signed URL valid mengembalikan file dengan header nosniff", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(nethttp.MethodGet, signedURL, nil)
		r.ServeHTTP(rec, req)

		assert.Equal(t, nethttp.StatusOK, rec.Code)
		assert.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, jpegBytes, rec.Body.Bytes())
	})

	t.Run("tanpa signature ditolak 403", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(nethttp.MethodGet, "/api/v1/files?key="+res.Key, nil)
		r.ServeHTTP(rec, req)
		assert.Equal(t, nethttp.StatusForbidden, rec.Code)
	})

	t.Run("signature salah ditolak 403", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(nethttp.MethodGet, signedURL+"tampered", nil)
		r.ServeHTTP(rec, req)
		assert.Equal(t, nethttp.StatusForbidden, rec.Code)
	})

	t.Run("file sudah dihapus mengembalikan 404 walau signature valid", func(t *testing.T) {
		require.NoError(t, loc.Delete(context.Background(), res.Key))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(nethttp.MethodGet, signedURL, nil)
		r.ServeHTTP(rec, req)
		assert.Equal(t, nethttp.StatusNotFound, rec.Code)
	})
}
