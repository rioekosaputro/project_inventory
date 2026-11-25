package domain

import (
	"context"
	"time"
)

// Nama-nama job terjadwal (plan Bagian 6.5, 8.6, T2.10).
const (
	JobBillingMonthly     = "BILLING_MONTHLY"
	JobContractExpiry     = "CONTRACT_EXPIRY"
	JobInvoiceOverdue     = "INVOICE_OVERDUE"
	JobLowStockAlert      = "LOW_STOCK_ALERT"
	JobStaleDOReservation = "STALE_DO_RESERVATION"
)

const (
	JobStatusRunning = "RUNNING"
	JobStatusSuccess = "SUCCESS"
	JobStatusFailed  = "FAILED"
)

// JobRun merepresentasikan satu eksekusi job terjadwal di tabel job_runs.
type JobRun struct {
	ID         int64          `json:"id"`
	JobName    string         `json:"job_name"`
	RunKey     string         `json:"run_key"`
	Status     string         `json:"status"`
	Summary    map[string]any `json:"summary,omitempty"`
	Error      *string        `json:"error,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
}

// ExpiringContractAlert adalah ringkasan kontrak ACTIVE yang akan berakhir dalam N hari.
type ExpiringContractAlert struct {
	ContractID     int64  `json:"contract_id"`
	ContractNumber string `json:"contract_number"`
	CustomerID     int64  `json:"customer_id"`
	CustomerName   string `json:"customer_name"`
	EndDate        string `json:"end_date"`
	DaysRemaining  int    `json:"days_remaining"`
}

// JobLowStockItem adalah ringkasan produk aktif yang stok tersedianya di bawah min_stock_alert.
type JobLowStockItem struct {
	ProductID     int64  `json:"product_id"`
	SKU           string `json:"sku"`
	ProductName   string `json:"product_name"`
	QtyAvailable  int    `json:"qty_available"`
	MinStockAlert int    `json:"min_stock_alert"`
}

// StaleDOAlertItem adalah DO OPEN/RESERVED yang reservasinya menggantung melewati ambang waktu.
type StaleDOAlertItem struct {
	DOID             int64     `json:"do_id"`
	DONumber         string    `json:"do_number"`
	WarehouseID      int64     `json:"warehouse_id"`
	Status           string    `json:"status"`
	OldestReservedAt time.Time `json:"oldest_reserved_at"`
	ReservedCount    int       `json:"reserved_count"`
}

// OverdueInvoiceAlert adalah ringkasan invoice ISSUED/PARTIALLY_PAID yang melewati jatuh tempo.
type OverdueInvoiceAlert struct {
	InvoiceID       int64  `json:"invoice_id"`
	InvoiceNumber   string `json:"invoice_number"`
	CustomerID      int64  `json:"customer_id"`
	CustomerName    string `json:"customer_name"`
	DueDate         string `json:"due_date"`
	DaysOverdue     int    `json:"days_overdue"`
	RemainingAmount string `json:"remaining_amount"`
}

// JobRepository mengelola tabel job_runs dan query pemeliharaan harian.
type JobRepository interface {
	// TryAcquire mencoba mengunci eksekusi (job_name, run_key).
	// Mengembalikan acquired=true bila job boleh dijalankan (baru atau retry dari FAILED),
	// atau acquired=false bila sudah SUCCESS/RUNNING.
	TryAcquire(ctx context.Context, jobName, runKey string) (*JobRun, bool, error)
	MarkSuccess(ctx context.Context, id int64, summary map[string]any) error
	MarkFailed(ctx context.Context, id int64, summary map[string]any, errMsg string) error
	ListRecent(ctx context.Context, limit int) ([]JobRun, error)

	// Pemeliharaan harian (T2.10).
	ExpireElapsedContracts(ctx context.Context, today time.Time) (int64, error)
	FindExpiringContracts(ctx context.Context, today time.Time, withinDays int) ([]ExpiringContractAlert, error)
	FindOverdueInvoices(ctx context.Context, today time.Time) ([]OverdueInvoiceAlert, error)
	FindLowStockProducts(ctx context.Context) ([]JobLowStockItem, error)
	FindStaleDOReservations(ctx context.Context, olderThan time.Time) ([]StaleDOAlertItem, error)
	CleanupExpiredIdempotencyKeys(ctx context.Context, olderThan time.Time) (int64, error)
}
