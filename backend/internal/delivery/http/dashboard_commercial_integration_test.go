//go:build integration

package http_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

func TestDashboard_CommercialMetricsAndRoleVisibility(t *testing.T) {
	e := newEnv(t)
	// Tetapkan jam palsu di 2026-12-15 10:00 WIB agar bulan berjalan = Des 2026.
	e.clk.Set(time.Date(2026, 12, 15, 3, 0, 0, 0, time.UTC))
	f := newLPBFixture(t, e)

	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	mgmt := e.tokenFor(domain.RoleManagement, nil)
	staff := f.staffA

	e.exec(`INSERT INTO tax_settings (tax_code, rate, dpp_numerator, dpp_denominator, faktur_code, effective_from, notes)
	        VALUES ('PPN', 12.00, 11, 12, '04', '2025-01-01', 'PMK 131/2024')
	        ON CONFLICT (tax_code, effective_from) DO NOTHING`)

	// Seed 1 pelanggan + 1 kontrak aktif yang berakhir 10 hari lagi (2026-12-25).
	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-DASH-01")
	ctrID := seedActiveContract(t, e, biz, cid, sid, f.p1)
	e.exec(`UPDATE rental_contracts SET end_date = '2026-12-25' WHERE id = $1`, ctrID)

	// Seed 2 unit SN: 1 RENTED + 1 AVAILABLE → utilisasi aset = 50.00%.
	sn1 := seedSerialForBilling(t, e, f.p1, cid, sid, ctrID, "SN-DASH-RENTED")
	e.exec(`INSERT INTO product_serials (product_id, serial_number, asset_tag, status, current_warehouse_id, current_rack_id)
	        VALUES ($1, 'SN-DASH-AVAIL', 'AST-DASH-AVAIL', 'AVAILABLE', $2, $3)`, f.p1, f.whA, f.a1)
	e.exec(`INSERT INTO rental_placements (contract_id, product_serial_id, customer_site_id, monthly_rate, start_date)
	        VALUES ($1, $2, $3, 4000000.00, '2026-10-01')`, ctrID, sn1, sid)

	// Generate invoice Des 2026 (invoice_date = 2026-12-01, grand_total = 4.440.000), issue, lalu mundurkan due_date ke 2026-12-10 (overdue 5 hari).
	gen := e.post(biz, "/api/v1/invoices/generate", map[string]any{"run_date": "2026-12-01"})
	require.Equal(t, 200, gen.status())
	var invID int64
	require.NoError(t, e.pool.QueryRow(context.Background(), `SELECT id FROM invoices ORDER BY id DESC LIMIT 1`).Scan(&invID))
	require.Equal(t, 200, e.post(biz, fmt.Sprintf("/api/v1/invoices/%d/issue", invID), map[string]any{}).status())
	e.exec(`UPDATE invoices SET due_date = '2026-12-10' WHERE id = $1`, invID)

	// Catat pembayaran parsial Rp 1.000.000 pada 2026-12-12 → sisa piutang & overdue = Rp 3.440.000.
	pay := e.call("POST", fmt.Sprintf("/api/v1/invoices/%d/payments", invID), map[string]any{
		"payment_date":    "2026-12-12",
		"amount_received": "1000000.00",
		"method":          "TRANSFER",
	}, withBearer(biz), withHeader("Idempotency-Key", "dash-pay-1"))
	require.Equal(t, 201, pay.status(), pay.rec.Body.String())

	// 1. MANAGEMENT mendapat metrik komersial lengkap.
	resp := e.get(mgmt, "/api/v1/dashboard/summary")
	require.Equal(t, 200, resp.status(), resp.rec.Body.String())
	comm, ok := resp.data()["commercial"].(map[string]any)
	require.True(t, ok, "MANAGEMENT seharusnya menerima field commercial")

	assert.Equal(t, float64(1), comm["active_customers"])
	assert.Equal(t, float64(1), comm["active_contracts"])
	assert.Equal(t, float64(1), comm["rented_units"])
	assert.Equal(t, float64(2), comm["total_active_assets"])
	assert.Equal(t, "50.00", comm["asset_utilization_pct"])
	assert.Equal(t, "4440000.00", comm["monthly_revenue"])
	assert.Equal(t, "1000000.00", comm["monthly_collected"])
	assert.Equal(t, "3440000.00", comm["outstanding_receivables"])
	assert.Equal(t, float64(1), comm["overdue_invoice_count"])
	assert.Equal(t, "3440000.00", comm["overdue_invoice_amount"])
	assert.Equal(t, float64(1), comm["expiring_contracts_count"])
	assert.Len(t, comm["expiring_contracts"].([]any), 1)
	assert.Len(t, comm["overdue_invoices"].([]any), 1)

	// 2. STAFF_WAREHOUSE tidak menerima metrik komersial (tidak membocorkan angka omzet).
	whResp := e.get(staff, "/api/v1/dashboard/summary")
	require.Equal(t, 200, whResp.status())
	_, hasComm := whResp.data()["commercial"]
	assert.False(t, hasComm, "STAFF_WAREHOUSE tidak boleh melihat metrik komersial")

	_ = clock.WIB
}
