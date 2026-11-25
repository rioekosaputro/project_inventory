// Package handler menyediakan entrypoint Go Serverless Function untuk Vercel.
// Lihat backend/vercel.json untuk aturan rewrite seluruh route ke /api/index.go.
package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"rental-inventory-api/config"
	"rental-inventory-api/internal/app"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/database"
)

var (
	initOnce sync.Once
	engine   http.Handler
	initErr  error
)

func initRouter() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		initErr = fmt.Errorf("config load: %w", err)
		logger.Error("gagal memuat konfigurasi Vercel API", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, database.Config{
		URL:        cfg.DatabaseURL,
		PoolerMode: cfg.DBPoolerMode,
		MaxConns:   cfg.DBMaxConns,
	})
	if err != nil {
		initErr = fmt.Errorf("db pool: %w", err)
		logger.Error("gagal membuka pool database", "error", err)
		return
	}

	r, err := app.NewRouter(cfg, logger, pool, clock.System{})
	if err != nil {
		initErr = fmt.Errorf("new router: %w", err)
		logger.Error("gagal merakit router", "error", err)
		return
	}
	engine = r
}

// Handler adalah entrypoint HTTP resmi yang dipanggil runtime @vercel/go.
func Handler(w http.ResponseWriter, r *http.Request) {
	initOnce.Do(initRouter)
	if initErr != nil || engine == nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, `{"success":false,"code":"INIT_ERROR","message":"Inisialisasi API gagal: %v"}`, initErr)
		return
	}

	// Vercel rewrite "/(.*)" -> "/api/index.go?__path=$1" dapat mengirim r.URL.Path sebagai
	// "/api", "/api/index", atau "/api/index.go". Selalu pulihkan path asli dari __path bila ada.
	q := r.URL.Query()
	if q.Has("__path") {
		orig := q.Get("__path")
		q.Del("__path")
		r.URL.RawQuery = q.Encode()
		r.URL.Path = "/" + strings.TrimPrefix(orig, "/")
	}

	engine.ServeHTTP(w, r)
}
