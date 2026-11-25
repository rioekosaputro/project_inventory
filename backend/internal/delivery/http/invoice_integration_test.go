//go:build integration

package http_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"rental-inventory-api/internal/domain"
)

func TestInvoice_IssuePayPPh23IdempotencyVoidAndExport(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	mgmt := e.tokenFor(domain.RoleManagement, nil)

	e.exec(`INSERT INTO tax_settings (tax_code, rate, dpp_numerator, dpp_denominator, faktur_code, effective_from, notes)
	        VALUES ('PPN', 12.00, 11, 12, '04', '2025-01-01', 'PMK 131/2024')
	        ON CONFLICT (tax_code, effective_from) DO NOTHING`)

	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-INV-01")
	ctrID := seedActiveContract(t, e, biz, cid, sid, f.p1)
	snID := seedSerialForBilling(t, e, f.p1, cid, sid, ctrID, "SN-INV-01")
	e.exec(`INSERT INTO rental_placements (contract_id, product_serial_id, customer_site_id, monthly_rate, start_date)
	        VALUES ($1, $2, $3, 5000000.00, '2026-10-01')`, ctrID, snID, sid)

	// Generate 1 invoice untuk Nov 2026: subtotal 5.000.000, DPP Nilai Lain 4.583.333, PPN 550.000, Grand Total 5.550.000.
	gen := e.post(biz, "/api/v1/invoices/generate", map[string]any{"run_date": "2026-12-01"})
	require.Equal(t, 200, gen.status())
	results := gen.data()["results"].([]any)
	require.Len(t, results, 1)
	invID := int64(results[0].(map[string]any)["invoice_id"].(float64))

	// List & Get (MANAGEMENT boleh baca).
	assert.Equal(t, 200, e.get(mgmt, "/api/v1/invoices").status())
	assert.Equal(t, 403, e.post(mgmt, fmt.Sprintf("/api/v1/invoices/%d/issue", invID), nil).status())

	// Issue DRAFT → ISSUED dengan nomor faktur pajak.
	iss := e.post(biz, fmt.Sprintf("/api/v1/invoices/%d/issue", invID), map[string]any{
		"faktur_pajak_number": "040.000-26.00000001",
	})
	require.Equal(t, 200, iss.status(), iss.rec.Body.String())
	assert.Equal(t, "ISSUED", iss.data()["status"])
	assert.Equal(t, "040.000-26.00000001", iss.data()["faktur_pajak_number"])
	assert.Equal(t, "5550000.00", iss.data()["grand_total"])

	// Pembayaran #1 (parsial + PPh 23 2% = Rp 100.000, transfer Rp 2.000.000 → total 2.100.000).
	// Wajib Idempotency-Key.
	pay1 := e.call("POST", fmt.Sprintf("/api/v1/invoices/%d/payments", invID), map[string]any{
		"payment_date":      "2026-12-10",
		"amount_received":   "2000000.00",
		"pph23_amount":      "100000.00",
		"pph23_slip_number": "BP-PPH23-001",
		"method":            "TRANSFER",
		"reference_number":  "TRF-001",
	}, withBearer(biz), withHeader("Idempotency-Key", "pay-key-1"))
	require.Equal(t, 201, pay1.status(), pay1.rec.Body.String())
	assert.Equal(t, "PARTIALLY_PAID", pay1.data()["status"])
	assert.Equal(t, "2100000.00", pay1.data()["paid_amount"])
	assert.Equal(t, "3450000.00", pay1.data()["remaining_amount"])

	// Replay Idempotency-Key yang sama → tidak mencatat pembayaran ganda!
	pay1Replay := e.call("POST", fmt.Sprintf("/api/v1/invoices/%d/payments", invID), map[string]any{
		"payment_date":      "2026-12-10",
		"amount_received":   "2000000.00",
		"pph23_amount":      "100000.00",
		"pph23_slip_number": "BP-PPH23-001",
		"method":            "TRANSFER",
		"reference_number":  "TRF-001",
	}, withBearer(biz), withHeader("Idempotency-Key", "pay-key-1"))
	require.Equal(t, 201, pay1Replay.status())
	assert.Equal(t, "2100000.00", pay1Replay.data()["paid_amount"])

	// Overpayment ditolak (422).
	over := e.call("POST", fmt.Sprintf("/api/v1/invoices/%d/payments", invID), map[string]any{
		"amount_received": "4000000.00",
		"method":          "TRANSFER",
	}, withBearer(biz), withHeader("Idempotency-Key", "pay-key-over"))
	assert.Equal(t, 422, over.status())

	// Invoice yang sudah dibayar sebagian tidak bisa di-VOID (409).
	assert.Equal(t, 409, e.post(biz, fmt.Sprintf("/api/v1/invoices/%d/void", invID), map[string]any{
		"void_reason": "Coba void setelah dibayar",
	}).status())

	// Pembayaran #2 melunasi sisa Rp 3.450.000 → otomatis menjadi PAID.
	pay2 := e.call("POST", fmt.Sprintf("/api/v1/invoices/%d/payments", invID), map[string]any{
		"payment_date":    "2026-12-15",
		"amount_received": "3450000.00",
		"method":          "TRANSFER",
	}, withBearer(biz), withHeader("Idempotency-Key", "pay-key-2"))
	require.Equal(t, 201, pay2.status(), pay2.rec.Body.String())
	assert.Equal(t, "PAID", pay2.data()["status"])
	assert.Equal(t, "5550000.00", pay2.data()["paid_amount"])
	assert.Equal(t, "0.00", pay2.data()["remaining_amount"])
	assert.Len(t, pay2.data()["payments"].([]any), 2)

	// Export Excel (.xlsx) valid dan punya 2 sheet: Daftar Tagihan & Rekap PPN.
	exp := e.get(biz, "/api/v1/invoices/export")
	require.Equal(t, 200, exp.status())
	wb, err := excelize.OpenReader(bytes.NewReader(exp.rec.Body.Bytes()))
	require.NoError(t, err)
	defer func() { _ = wb.Close() }()
	assert.Equal(t, []string{"Daftar Tagihan", "Rekap PPN"}, wb.GetSheetList())
	rows, err := wb.GetRows("Daftar Tagihan")
	require.NoError(t, err)
	require.Len(t, rows, 2) // header + 1 invoice
	assert.Equal(t, "PAID", rows[1][8])
}
