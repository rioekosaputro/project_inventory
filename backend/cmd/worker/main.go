// Binary cmd/worker menjalankan penjadwalan job background untuk production (VPS/Docker)
// menggunakan robfig/cron/v3 pada zona waktu WIB (plan Bagian 8.6 & T2.10).
// Untuk staging Vercel, penjadwalan memanggil GET /api/v1/internal/jobs/* (vercel.json).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"

	"rental-inventory-api/config"
	"rental-inventory-api/internal/app"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/database"
)

const jobRunTimeout = 10 * time.Minute

func main() {
	runOnce := flag.String("run-once", "", "jalankan satu job langsung lalu keluar: billing | daily")
	runDate := flag.String("date", "", "override tanggal target (YYYY-MM-DD, opsional untuk -run-once)")
	flag.Parse()

	if err := run(*runOnce, *runDate); err != nil {
		slog.Error("worker berhenti dengan error", "error", err)
		os.Exit(1)
	}
}

func run(runOnce, runDate string) error {
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

	jobUC := app.NewJobUsecase(pool, clock.System{}, logger)

	if runOnce != "" {
		var datePtr *string
		if runDate != "" {
			datePtr = &runDate
		}
		execCtx, cancel := context.WithTimeout(ctx, jobRunTimeout)
		defer cancel()

		switch runOnce {
		case "billing":
			res, err := jobUC.RunBilling(execCtx, datePtr)
			if err != nil {
				return err
			}
			logger.Info("run-once billing selesai", "run_key", res.RunKey, "status", res.Status, "skipped", res.Skipped)
			return nil
		case "daily":
			res, err := jobUC.RunDaily(execCtx, datePtr)
			if err != nil {
				return err
			}
			logger.Info("run-once daily selesai", "run_date", res.RunDate, "jobs", len(res.Jobs))
			return nil
		default:
			return fmt.Errorf("nilai -run-once tidak dikenal: %q (gunakan billing atau daily)", runOnce)
		}
	}

	loc := cfg.Location
	if loc == nil {
		loc = clock.WIB
	}
	c := cron.New(cron.WithLocation(loc))

	// 01:00 WIB: penagihan bulanan (BILLING_MONTHLY).
	if _, err := c.AddFunc("0 1 * * *", func() {
		execCtx, cancel := context.WithTimeout(ctx, jobRunTimeout)
		defer cancel()
		if _, err := jobUC.RunBilling(execCtx, nil); err != nil {
			logger.Error("cron billing gagal", "error", err)
		}
	}); err != nil {
		return fmt.Errorf("daftarkan cron billing: %w", err)
	}

	// 01:30 WIB: pemeliharaan harian (kontrak berakhir, invoice overdue, low-stock, stale DO).
	if _, err := c.AddFunc("30 1 * * *", func() {
		execCtx, cancel := context.WithTimeout(ctx, jobRunTimeout)
		defer cancel()
		if _, err := jobUC.RunDaily(execCtx, nil); err != nil {
			logger.Error("cron daily gagal", "error", err)
		}
	}); err != nil {
		return fmt.Errorf("daftarkan cron daily: %w", err)
	}

	c.Start()
	logger.Info("worker cron berjalan", "timezone", loc.String(), "env", cfg.AppEnv)

	<-ctx.Done()
	logger.Info("sinyal berhenti diterima, menunggu job aktif selesai...")
	<-c.Stop().Done()
	logger.Info("worker berhenti dengan bersih")
	return nil
}

func newLogger(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if cfg.IsLocal() {
		opts.Level = slog.LevelDebug
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
