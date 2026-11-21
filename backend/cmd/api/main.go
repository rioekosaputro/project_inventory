package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rental-inventory-api/config"
	apihttp "rental-inventory-api/internal/delivery/http"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

const shutdownTimeout = 15 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("aplikasi berhenti dengan error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, database.Config{
		URL:        cfg.DatabaseURL,
		PoolerMode: cfg.DBPoolerMode,
		MaxConns:   cfg.DBMaxConns,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	// TxManager dipakai usecase mulai T1.x; assertion memastikan kontraknya terpenuhi.
	var _ domain.TxManager = database.NewTxManager(pool)

	router := apihttp.NewRouter(cfg, logger, apihttp.ReadinessCheck{
		Name:  "database",
		Check: pool.Ping,
	})

	srv := &nethttp.Server{
		Addr:              net.JoinHostPort("", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("API berjalan", "addr", srv.Addr, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("sinyal berhenti diterima, mematikan server...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("server berhenti dengan bersih")
	return nil
}

// newLogger: teks berwarna-ringan di lokal, JSON di staging/production.
func newLogger(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if cfg.IsLocal() {
		opts.Level = slog.LevelDebug
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
