package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

const (
	contractExpiryAlertDays = 30
	staleDOReservationHours = 48
	expiredIdempotencyHours = 24
)

// JobExecResult adalah ringkasan hasil eksekusi satu job terjadwal.
type JobExecResult struct {
	JobName string         `json:"job_name"`
	RunKey  string         `json:"run_key"`
	Status  string         `json:"status"`
	Skipped bool           `json:"skipped"`
	Summary map[string]any `json:"summary,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// DailyJobsResult adalah gabungan hasil semua sub-job harian.
type DailyJobsResult struct {
	RunDate string          `json:"run_date"`
	Jobs    []JobExecResult `json:"jobs"`
}

// JobUsecase mengorkestrasi eksekusi job terjadwal dengan pelacakan idempoten di tabel job_runs.
// Dipakai bersama oleh endpoint HTTP /internal/jobs/* (Vercel Cron) dan binary cmd/worker (production).
type JobUsecase struct {
	jobs      domain.JobRepository
	billing   *BillingUsecase
	audit     domain.AuditRepository
	reconcile domain.ReconcileRepository
	clk       clock.Clock
	logger    *slog.Logger
}

// WithReconcile memasang repository pemeriksaan invarian stok untuk job harian & halaman admin (T4.2).
func (u *JobUsecase) WithReconcile(repo domain.ReconcileRepository) *JobUsecase {
	u.reconcile = repo
	return u
}

// CheckReconcile menjalankan pemeriksaan invarian stok (I1–I6) secara on-demand untuk Admin (T4.2).
func (u *JobUsecase) CheckReconcile(ctx context.Context) (*domain.ReconcileReport, error) {
	if u.reconcile == nil {
		return nil, domain.ErrInvalidStateTransition
	}
	return u.reconcile.CheckStockInvariants(ctx, u.clk.Now())
}

func NewJobUsecase(
	jobs domain.JobRepository,
	billing *BillingUsecase,
	audit domain.AuditRepository,
	clk clock.Clock,
	logger *slog.Logger,
) *JobUsecase {
	if logger == nil {
		logger = slog.Default()
	}
	return &JobUsecase{
		jobs:    jobs,
		billing: billing,
		audit:   audit,
		clk:     clk,
		logger:  logger,
	}
}

func (u *JobUsecase) resolveRunDate(dateStr *string) (time.Time, string, error) {
	if dateStr == nil || strings.TrimSpace(*dateStr) == "" {
		d := todayDateWIB(u.clk.Now())
		return d, d.Format("2006-01-02"), nil
	}
	s := strings.TrimSpace(*dateStr)
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, "", masterInvalid("Parameter tanggal job", []domain.FieldError{
			{Field: "date", Message: "format tanggal harus YYYY-MM-DD"},
		})
	}
	return d, d.Format("2006-01-02"), nil
}

// RunBilling menjalankan BILLING_MONTHLY untuk tanggal target (default hari ini WIB).
// Idempoten per tanggal melalui job_runs(job_name, run_key).
func (u *JobUsecase) RunBilling(ctx context.Context, dateStr *string) (*JobExecResult, error) {
	_, runKey, err := u.resolveRunDate(dateStr)
	if err != nil {
		return nil, err
	}

	run, acquired, err := u.jobs.TryAcquire(ctx, domain.JobBillingMonthly, runKey)
	if err != nil {
		return nil, fmt.Errorf("acquire job %s: %w", domain.JobBillingMonthly, err)
	}
	if !acquired {
		u.logger.Info("job billing dilewati (sudah jalan/selesai)", "job", domain.JobBillingMonthly, "run_key", runKey, "status", run.Status)
		return &JobExecResult{
			JobName: domain.JobBillingMonthly,
			RunKey:  runKey,
			Status:  run.Status,
			Skipped: true,
			Summary: run.Summary,
		}, nil
	}

	res, err := u.billing.GenerateMonthlyInvoices(ctx, nil, GenerateBillingInput{
		RunDate:        &runKey,
		GeneratedByJob: true,
	})
	if err != nil {
		_ = u.jobs.MarkFailed(ctx, run.ID, nil, err.Error())
		u.logger.Error("job billing gagal", "job", domain.JobBillingMonthly, "run_key", runKey, "error", err)
		return &JobExecResult{
			JobName: domain.JobBillingMonthly,
			RunKey:  runKey,
			Status:  domain.JobStatusFailed,
			Error:   err.Error(),
		}, err
	}

	summary := map[string]any{
		"run_date":          res.RunDate,
		"contracts_checked": res.ContractsChecked,
		"invoices_created":  res.InvoicesCreated,
		"skipped":           res.Skipped,
		"errors":            res.Errors,
	}

	if res.Errors > 0 {
		errMsg := fmt.Sprintf("%d kontrak gagal dibuatkan invoice", res.Errors)
		_ = u.jobs.MarkFailed(ctx, run.ID, summary, errMsg)
		u.logger.Warn("job billing selesai dengan error parsial", "run_key", runKey, "errors", res.Errors, "created", res.InvoicesCreated)
		return &JobExecResult{
			JobName: domain.JobBillingMonthly,
			RunKey:  runKey,
			Status:  domain.JobStatusFailed,
			Summary: summary,
			Error:   errMsg,
		}, nil
	}

	if err := u.jobs.MarkSuccess(ctx, run.ID, summary); err != nil {
		return nil, fmt.Errorf("mark job success: %w", err)
	}
	_ = u.audit.Insert(ctx, domain.AuditEntry{
		Action:     "JOB_BILLING_MONTHLY",
		EntityType: "job_run",
		EntityID:   &run.ID,
		After:      summary,
	})
	u.logger.Info("job billing selesai", "run_key", runKey, "checked", res.ContractsChecked, "created", res.InvoicesCreated)

	return &JobExecResult{
		JobName: domain.JobBillingMonthly,
		RunKey:  runKey,
		Status:  domain.JobStatusSuccess,
		Summary: summary,
	}, nil
}

// RunDaily menjalankan rangkaian job pemeliharaan harian:
// 1. CONTRACT_EXPIRY (tandai kontrak berakhir -> EXPIRED + alert kontrak berakhir <= 30 hari)
// 2. INVOICE_OVERDUE (tandai invoice ISSUED/PARTIALLY_PAID lewat jatuh tempo -> OVERDUE)
// 3. LOW_STOCK_ALERT (deteksi produk aktif di bawah min_stock_alert)
// 4. STALE_DO_RESERVATION (deteksi reservasi DO menggantung > 48 jam + bersihkan idempotency_keys > 24 jam)
func (u *JobUsecase) RunDaily(ctx context.Context, dateStr *string) (*DailyJobsResult, error) {
	runDate, runKey, err := u.resolveRunDate(dateStr)
	if err != nil {
		return nil, err
	}

	out := &DailyJobsResult{
		RunDate: runKey,
		Jobs:    make([]JobExecResult, 0, 4),
	}

	out.Jobs = append(out.Jobs, u.runTracked(ctx, domain.JobContractExpiry, runKey, func(ctx context.Context) (map[string]any, error) {
		expiredCount, err := u.jobs.ExpireElapsedContracts(ctx, runDate)
		if err != nil {
			return nil, err
		}
		expiring, err := u.jobs.FindExpiringContracts(ctx, runDate, contractExpiryAlertDays)
		if err != nil {
			return nil, err
		}
		var h7, h14, h30 int
		for _, a := range expiring {
			switch {
			case a.DaysRemaining <= 7:
				h7++
			case a.DaysRemaining <= 14:
				h14++
			default:
				h30++
			}
		}
		return map[string]any{
			"expired_marked":    expiredCount,
			"expiring_within30": len(expiring),
			"expiring_h7":       h7,
			"expiring_h14":      h14,
			"expiring_h30":      h30,
			"alerts":            expiring,
		}, nil
	}))

	out.Jobs = append(out.Jobs, u.runTracked(ctx, domain.JobInvoiceOverdue, runKey, func(ctx context.Context) (map[string]any, error) {
		overdue, err := u.jobs.FindOverdueInvoices(ctx, runDate)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"overdue_count": len(overdue),
			"alerts":        overdue,
		}, nil
	}))

	out.Jobs = append(out.Jobs, u.runTracked(ctx, domain.JobLowStockAlert, runKey, func(ctx context.Context) (map[string]any, error) {
		items, err := u.jobs.FindLowStockProducts(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"low_stock_count": len(items),
			"items":           items,
		}, nil
	}))

	out.Jobs = append(out.Jobs, u.runTracked(ctx, domain.JobStaleDOReservation, runKey, func(ctx context.Context) (map[string]any, error) {
		now := u.clk.Now()
		stale, err := u.jobs.FindStaleDOReservations(ctx, now.Add(-staleDOReservationHours*time.Hour))
		if err != nil {
			return nil, err
		}
		cleanedKeys, err := u.jobs.CleanupExpiredIdempotencyKeys(ctx, now.Add(-expiredIdempotencyHours*time.Hour))
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"stale_do_count":          len(stale),
			"stale_dos":               stale,
			"idempotency_keys_pruned": cleanedKeys,
		}, nil
	}))

	if u.reconcile != nil {
		out.Jobs = append(out.Jobs, u.runTracked(ctx, domain.JobStockReconcile, runKey, func(ctx context.Context) (map[string]any, error) {
			rep, err := u.reconcile.CheckStockInvariants(ctx, u.clk.Now())
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"healthy":         rep.Healthy,
				"total_checks":    rep.TotalChecks,
				"violation_count": rep.ViolationCount,
				"violations":      rep.Violations,
			}, nil
		}))
	}

	return out, nil
}

func (u *JobUsecase) runTracked(ctx context.Context, jobName, runKey string, fn func(context.Context) (map[string]any, error)) JobExecResult {
	run, acquired, err := u.jobs.TryAcquire(ctx, jobName, runKey)
	if err != nil {
		u.logger.Error("gagal acquire job", "job", jobName, "run_key", runKey, "error", err)
		return JobExecResult{
			JobName: jobName,
			RunKey:  runKey,
			Status:  domain.JobStatusFailed,
			Error:   err.Error(),
		}
	}
	if !acquired {
		return JobExecResult{
			JobName: jobName,
			RunKey:  runKey,
			Status:  run.Status,
			Skipped: true,
			Summary: run.Summary,
		}
	}

	summary, err := fn(ctx)
	if err != nil {
		_ = u.jobs.MarkFailed(ctx, run.ID, summary, err.Error())
		u.logger.Error("job harian gagal", "job", jobName, "run_key", runKey, "error", err)
		return JobExecResult{
			JobName: jobName,
			RunKey:  runKey,
			Status:  domain.JobStatusFailed,
			Summary: summary,
			Error:   err.Error(),
		}
	}

	if err := u.jobs.MarkSuccess(ctx, run.ID, summary); err != nil {
		u.logger.Error("gagal menandai job sukses", "job", jobName, "run_key", runKey, "error", err)
		return JobExecResult{
			JobName: jobName,
			RunKey:  runKey,
			Status:  domain.JobStatusFailed,
			Summary: summary,
			Error:   err.Error(),
		}
	}
	_ = u.audit.Insert(ctx, domain.AuditEntry{
		Action:     "JOB_" + jobName,
		EntityType: "job_run",
		EntityID:   &run.ID,
		After:      summary,
	})
	u.logger.Info("job harian selesai", "job", jobName, "run_key", runKey)
	return JobExecResult{
		JobName: jobName,
		RunKey:  runKey,
		Status:  domain.JobStatusSuccess,
		Summary: summary,
	}
}
