package domain

import (
	"context"
	"time"
)

const JobStockReconcile = "STOCK_RECONCILE"

// ReconcileViolation adalah satu baris anomali yang melanggar invarian konsistensi stok (plan Bagian 5.9).
type ReconcileViolation struct {
	Code   string `json:"code"`
	Check  string `json:"check"`
	Detail string `json:"detail"`
}

// ReconcileReport adalah hasil pemeriksaan seluruh invarian stok (I1–I6).
type ReconcileReport struct {
	CheckedAt      time.Time            `json:"checked_at"`
	Healthy        bool                 `json:"healthy"`
	TotalChecks    int                  `json:"total_checks"`
	ViolationCount int                  `json:"violation_count"`
	Violations     []ReconcileViolation `json:"violations"`
}

// ReconcileRepository menjalankan query pemeriksaan invarian stok terhadap PostgreSQL (T4.2).
type ReconcileRepository interface {
	CheckStockInvariants(ctx context.Context, now time.Time) (*ReconcileReport, error)
}
