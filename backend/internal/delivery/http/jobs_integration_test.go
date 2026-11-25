//go:build integration

package http_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/config"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/repository/postgres"
)

func TestInternalJobs_CronSecretProtection(t *testing.T) {
	// 1. Tanpa CRON_SECRET → 503 (tidak pernah terbuka tanpa konfigurasi).
	eUnconf := newEnv(t)
	assert.Equal(t, 503, eUnconf.call("GET", "/api/v1/internal/jobs/daily", nil).status())

	// 2. Dengan CRON_SECRET → tanpa header / token salah = 401; token benar = 200.
	const secret = "cron-secret-test-9876543210"
	e := newEnv(t, func(cfg *config.Config) { cfg.CronSecret = secret })

	assert.Equal(t, 401, e.call("GET", "/api/v1/internal/jobs/billing", nil).status())
	assert.Equal(t, 401, e.call("GET", "/api/v1/internal/jobs/billing", nil, withBearer("salah-token")).status())
	assert.Equal(t, 200, e.call("GET", "/api/v1/internal/jobs/billing?date=2026-12-01", nil, withBearer(secret)).status())
}

func TestInternalJobs_BillingAndDailyMaintenance_IdempotencyAndAlerts(t *testing.T) {
	const secret = "cron-secret-test-1234567890"
	e := newEnv(t, func(cfg *config.Config) { cfg.CronSecret = secret })
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)

	e.exec(`INSERT INTO tax_settings (tax_code, rate, dpp_numerator, dpp_denominator, faktur_code, effective_from, notes)
	        VALUES ('PPN', 12.00, 11, 12, '04', '2025-01-01', 'PMK 131/2024')
	        ON CONFLICT (tax_code, effective_from) DO NOTHING`)

	// Seed pelanggan + site + kontrak aktif yang akan berakhir dalam 5 hari dari 2026-12-01 (2026-12-06).
	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-JOB-01")
	ctrExpiringSoon := seedActiveContract(t, e, biz, cid, sid, f.p1)
	e.exec(`UPDATE rental_contracts SET end_date = '2026-12-06' WHERE id = $1`, ctrExpiringSoon)

	// Seed kontrak kedua yang sudah lewat end_date (2026-11-20 < 2026-12-01) → harus ditandai EXPIRED.
	ctrElapsed := seedActiveContract(t, e, biz, cid, sid, f.p1)
	e.exec(`UPDATE rental_contracts SET start_date = '2025-11-21', end_date = '2026-11-20' WHERE id = $1`, ctrElapsed)

	// Placement aktif pada ctrExpiringSoon agar billing 2026-12-01 menghasilkan 1 invoice.
	snID := seedSerialForBilling(t, e, f.p1, cid, sid, ctrExpiringSoon, "SN-JOB-01")
	e.exec(`INSERT INTO rental_placements (contract_id, product_serial_id, customer_site_id, monthly_rate, start_date)
	        VALUES ($1, $2, $3, 3000000.00, '2026-10-01')`, ctrExpiringSoon, snID, sid)

	// Buat produk f.p1 low-stock (min_stock_alert = 5, saldo 0).
	e.exec(`UPDATE products SET min_stock_alert = 5 WHERE id = $1`, f.p1)

	// Buat 1 idempotency key kedaluwarsa (> 24 jam) untuk dibersihkan oleh daily job.
	var anyUserID int64
	require.NoError(t, e.pool.QueryRow(context.Background(), `SELECT id FROM users LIMIT 1`).Scan(&anyUserID))
	e.exec(`INSERT INTO idempotency_keys (idem_key, user_id, endpoint, status_code, response_body, created_at)
	        VALUES ('old-expired-key-1', $1, 'POST /api/v1/lpb/1/approve', 200, '{}'::jsonb, NOW() - INTERVAL '48 hours')`, anyUserID)

	// 1. Jalankan /internal/jobs/billing pada 2026-12-01.
	b1 := e.call("GET", "/api/v1/internal/jobs/billing?date=2026-12-01", nil, withBearer(secret))
	require.Equal(t, 200, b1.status(), b1.rec.Body.String())
	assert.Equal(t, domain.JobBillingMonthly, b1.data()["job_name"])
	assert.Equal(t, "2026-12-01", b1.data()["run_key"])
	assert.Equal(t, domain.JobStatusSuccess, b1.data()["status"])
	assert.Equal(t, false, b1.data()["skipped"])
	b1Sum := b1.data()["summary"].(map[string]any)
	assert.Equal(t, float64(1), b1Sum["invoices_created"])

	// 2. Panggil ulang billing untuk tanggal yang sama → skipped=true (idempoten via job_runs).
	b2 := e.call("GET", "/api/v1/internal/jobs/billing?date=2026-12-01", nil, withBearer(secret))
	require.Equal(t, 200, b2.status())
	assert.Equal(t, true, b2.data()["skipped"])
	assert.Equal(t, domain.JobStatusSuccess, b2.data()["status"])

	// Terbitkan invoice yang baru dibuat lalu mundurkan due_date ke 2026-11-30 agar di-mark OVERDUE oleh daily job.
	var invID int64
	require.NoError(t, e.pool.QueryRow(context.Background(), `SELECT id FROM invoices ORDER BY id DESC LIMIT 1`).Scan(&invID))
	e.exec(`UPDATE invoices SET status = 'ISSUED', due_date = '2026-11-30' WHERE id = $1`, invID)

	// 3. Jalankan /internal/jobs/daily pada 2026-12-01.
	d1 := e.call("GET", "/api/v1/internal/jobs/daily?date=2026-12-01", nil, withBearer(secret))
	require.Equal(t, 200, d1.status(), d1.rec.Body.String())
	jobs := d1.data()["jobs"].([]any)
	require.Len(t, jobs, 4)

	byName := map[string]map[string]any{}
	for _, raw := range jobs {
		m := raw.(map[string]any)
		byName[m["job_name"].(string)] = m
	}

	// CONTRACT_EXPIRY: 1 kontrak ditandai EXPIRED, 1 kontrak masuk alert H-7.
	ce := byName[domain.JobContractExpiry]
	assert.Equal(t, domain.JobStatusSuccess, ce["status"])
	assert.Equal(t, false, ce["skipped"])
	ceSum := ce["summary"].(map[string]any)
	assert.Equal(t, float64(1), ceSum["expired_marked"])
	assert.Equal(t, float64(1), ceSum["expiring_within30"])
	assert.Equal(t, float64(1), ceSum["expiring_h7"])

	var elapsedStatus string
	require.NoError(t, e.pool.QueryRow(context.Background(), `SELECT status FROM rental_contracts WHERE id = $1`, ctrElapsed).Scan(&elapsedStatus))
	assert.Equal(t, "EXPIRED", elapsedStatus)

	// INVOICE_OVERDUE: mendeteksi 1 invoice jatuh tempo (status tetap ISSUED, overdue adalah kondisi terhitung).
	io := byName[domain.JobInvoiceOverdue]
	assert.Equal(t, domain.JobStatusSuccess, io["status"])
	assert.Equal(t, float64(1), io["summary"].(map[string]any)["overdue_count"])

	// LOW_STOCK_ALERT: minimal 1 produk terdeteksi.
	ls := byName[domain.JobLowStockAlert]
	assert.Equal(t, domain.JobStatusSuccess, ls["status"])
	assert.GreaterOrEqual(t, ls["summary"].(map[string]any)["low_stock_count"].(float64), float64(1))

	// STALE_DO_RESERVATION: 1 idempotency key kedaluwarsa terhapus.
	sd := byName[domain.JobStaleDOReservation]
	assert.Equal(t, domain.JobStatusSuccess, sd["status"])
	assert.Equal(t, float64(1), sd["summary"].(map[string]any)["idempotency_keys_pruned"])

	// 4. Jalankan ulang daily untuk tanggal sama → semua 4 job skipped=true.
	d2 := e.call("GET", "/api/v1/internal/jobs/daily?date=2026-12-01", nil, withBearer(secret))
	require.Equal(t, 200, d2.status())
	for _, raw := range d2.data()["jobs"].([]any) {
		m := raw.(map[string]any)
		assert.Equal(t, true, m["skipped"], "job %s seharusnya skipped", m["job_name"])
	}

	// 5. Uji retry job FAILED: ubah status INVOICE_OVERDUE menjadi FAILED lalu panggil lagi → harus jalan ulang & SUCCESS.
	e.exec(`UPDATE job_runs SET status = 'FAILED', error = 'simulasi gagal' WHERE job_name = $1 AND run_key = '2026-12-01'`, domain.JobInvoiceOverdue)
	d3 := e.call("GET", "/api/v1/internal/jobs/daily?date=2026-12-01", nil, withBearer(secret))
	require.Equal(t, 200, d3.status())
	for _, raw := range d3.data()["jobs"].([]any) {
		m := raw.(map[string]any)
		if m["job_name"] == domain.JobInvoiceOverdue {
			assert.Equal(t, false, m["skipped"])
			assert.Equal(t, domain.JobStatusSuccess, m["status"])
		} else {
			assert.Equal(t, true, m["skipped"])
		}
	}

	// Pastikan ListRecent mengembalikan semua 5 job_runs.
	recent, err := postgres.NewJobRepo(e.pool).ListRecent(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, recent, 5)
}
