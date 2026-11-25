//go:build integration

package http_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
)

func seedSerialForBilling(t *testing.T, e *env, prodID, cid, sid, ctrID int64, sn string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, e.pool.QueryRow(t.Context(), `
		INSERT INTO product_serials (product_id, serial_number, asset_tag, status, condition, current_customer_id, current_site_id, current_contract_id)
		VALUES ($1, $2, $3, 'RENTED', 'GOOD', $4, $5, $6)
		RETURNING id`, prodID, sn, "AST-"+sn, cid, sid, ctrID).Scan(&id))
	return id
}

func TestBilling_ProrataFullMonthSwapPreviewAndIdempotency(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	mgmt := e.tokenFor(domain.RoleManagement, nil)

	// RBAC: MANAGEMENT tidak punya invoice.create.
	assert.Equal(t, 403, e.post(mgmt, "/api/v1/invoices/generate", nil).status())

	// Seed PMK 131/2024.
	e.exec(`INSERT INTO tax_settings (tax_code, rate, dpp_numerator, dpp_denominator, faktur_code, effective_from, notes)
	        VALUES ('PPN', 12.00, 11, 12, '04', '2025-01-01', 'PMK 131/2024')
	        ON CONFLICT (tax_code, effective_from) DO NOTHING`)

	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-BILL-01")
	ctrProrata := seedActiveContract(t, e, biz, cid, sid, f.p1)

	// Kontrak kedua: FULL_MONTH & non-PPN (is_ppn=false).
	c2Res := e.post(biz, "/api/v1/contracts", map[string]any{
		"customer_id":          cid,
		"start_date":           "2026-01-01",
		"duration_months":      12,
		"billing_day_of_month": 1,
		"billing_mode":         "FULL_MONTH",
		"is_ppn":               false,
		"items": []map[string]any{
			{"product_id": f.p1, "customer_site_id": sid, "qty": 1, "monthly_rate": "500000.00"},
		},
	})
	require.Equal(t, 201, c2Res.status(), c2Res.rec.Body.String())
	ctrFull := int64(c2Res.data()["id"].(float64))
	require.Equal(t, 200, e.post(biz, "/api/v1/contracts/"+formatID(ctrFull)+"/activate", nil).status())

	// Periode uji: 1–30 November 2026 (30 hari), run_date = 2026-12-01.
	// 1) Unit penuh 1–30 Nov (30/30) -> Rp 500.000
	snFull := seedSerialForBilling(t, e, f.p1, cid, sid, ctrProrata, "SN-B-FULL")
	e.exec(`INSERT INTO rental_placements (contract_id, product_serial_id, customer_site_id, monthly_rate, start_date)
	        VALUES ($1, $2, $3, 500000.00, '2026-10-01')`, ctrProrata, snFull, sid)

	// 2) Unit dikirim 16 Nov (15/30 hari) -> Rp 250.000
	snMidIn := seedSerialForBilling(t, e, f.p1, cid, sid, ctrProrata, "SN-B-MIDIN")
	e.exec(`INSERT INTO rental_placements (contract_id, product_serial_id, customer_site_id, monthly_rate, start_date)
	        VALUES ($1, $2, $3, 500000.00, '2026-11-16')`, ctrProrata, snMidIn, sid)

	// 3) Unit ditarik 10 Nov (end_date eksklusif -> 1–9 Nov = 9/30 hari) -> Rp 150.000
	snMidOut := seedSerialForBilling(t, e, f.p1, cid, sid, ctrProrata, "SN-B-MIDOUT")
	e.exec(`INSERT INTO rental_placements (contract_id, product_serial_id, customer_site_id, monthly_rate, start_date, end_date, end_reason)
	        VALUES ($1, $2, $3, 500000.00, '2026-10-01', '2026-11-10', 'WITHDRAWAL')`, ctrProrata, snMidOut, sid)

	// 4) Unit swap tgl 15 Nov: lama 1–14 Nov (14 hari) + baru 15–30 Nov (16 hari) = 30 hari -> Rp 500.000
	snSwapOld := seedSerialForBilling(t, e, f.p1, cid, sid, ctrProrata, "SN-B-SWOLD")
	snSwapNew := seedSerialForBilling(t, e, f.p1, cid, sid, ctrProrata, "SN-B-SWNEW")
	e.exec(`INSERT INTO rental_placements (contract_id, product_serial_id, customer_site_id, monthly_rate, start_date, end_date, end_reason)
	        VALUES ($1, $2, $3, 500000.00, '2026-10-01', '2026-11-15', 'SWAP')`, ctrProrata, snSwapOld, sid)
	e.exec(`INSERT INTO rental_placements (contract_id, product_serial_id, customer_site_id, monthly_rate, start_date)
	        VALUES ($1, $2, $3, 500000.00, '2026-11-15')`, ctrProrata, snSwapNew, sid)

	// 5) Kontrak FULL_MONTH non-PPN: unit masuk 16 Nov -> tetap ditagih penuh Rp 500.000, PPN 0.
	snFM := seedSerialForBilling(t, e, f.p1, cid, sid, ctrFull, "SN-B-FM")
	e.exec(`INSERT INTO rental_placements (contract_id, product_serial_id, customer_site_id, monthly_rate, start_date)
	        VALUES ($1, $2, $3, 500000.00, '2026-11-16')`, ctrFull, snFM, sid)

	// --- DRY-RUN (?preview=1) ---
	prevRes := e.post(biz, "/api/v1/invoices/generate?preview=1", map[string]any{"run_date": "2026-12-01"})
	require.Equal(t, 200, prevRes.status(), prevRes.rec.Body.String())
	pd := prevRes.data()
	assert.Equal(t, true, pd["preview"])
	assert.EqualValues(t, 2, pd["contracts_checked"])
	assert.EqualValues(t, 2, pd["invoices_created"])
	assert.EqualValues(t, 0, pd["errors"])

	// Pastikan preview TIDAK menulis satu baris pun ke tabel invoices.
	var invCount int
	require.NoError(t, e.pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM invoices`).Scan(&invCount))
	assert.Equal(t, 0, invCount)

	// Verifikasi angka kontrak PRORATA (subtotal = 500.000 + 250.000 + 150.000 + 233.333,33 + 266.666,67 = 1.400.000,00)
	// PMK 131/2024: DPP Nilai Lain = round(1.400.000 * 11/12) = 1.283.333; PPN 12% = round(1.283.333 * 12%) = 154.000; Grand Total = 1.554.000.
	results := pd["results"].([]any)
	r0 := results[0].(map[string]any)
	assert.Equal(t, "WOULD_CREATE", r0["status"])
	assert.Equal(t, "2026-11-01", r0["period_start"])
	assert.Equal(t, "2026-11-30", r0["period_end"])
	assert.Equal(t, "1400000.00", r0["subtotal"])
	assert.Equal(t, "1283333.00", r0["dpp_other_value"])
	assert.Equal(t, "154000.00", r0["tax_amount"])
	assert.Equal(t, "1554000.00", r0["grand_total"])

	// Verifikasi kontrak FULL_MONTH non-PPN: subtotal 500.000, tax 0, grand total 500.000.
	r1 := results[1].(map[string]any)
	assert.Equal(t, "WOULD_CREATE", r1["status"])
	assert.Equal(t, "500000.00", r1["subtotal"])
	assert.Equal(t, "0.00", r1["tax_amount"])
	assert.Equal(t, "500000.00", r1["grand_total"])

	// --- JALANKAN NYATA #1 ---
	run1 := e.post(biz, "/api/v1/invoices/generate", map[string]any{"run_date": "2026-12-01"})
	require.Equal(t, 200, run1.status(), run1.rec.Body.String())
	assert.EqualValues(t, 2, run1.data()["invoices_created"])
	assert.EqualValues(t, 0, run1.data()["skipped"])

	require.NoError(t, e.pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM invoices`).Scan(&invCount))
	assert.Equal(t, 2, invCount)

	// --- IDEMPOTENSI: JALANKAN NYATA #2 pada tanggal sama -> 0 invoice ganda ---
	run2 := e.post(biz, "/api/v1/invoices/generate", map[string]any{"run_date": "2026-12-01"})
	require.Equal(t, 200, run2.status(), run2.rec.Body.String())
	assert.EqualValues(t, 0, run2.data()["invoices_created"])
	assert.EqualValues(t, 2, run2.data()["skipped"])

	require.NoError(t, e.pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM invoices`).Scan(&invCount))
	assert.Equal(t, 2, invCount)

	// --- VOID salah satu invoice -> generate ulang membuat tepat 1 invoice baru ---
	e.exec(`UPDATE invoices SET status = 'VOID' WHERE contract_id = $1`, ctrFull)
	run3 := e.post(biz, "/api/v1/invoices/generate", map[string]any{"run_date": "2026-12-01"})
	require.Equal(t, 200, run3.status())
	assert.EqualValues(t, 1, run3.data()["invoices_created"])
	assert.EqualValues(t, 1, run3.data()["skipped"])
}

func formatID(id int64) string {
	return string(append([]byte{}, []byte(fmtInt(id))...))
}

func fmtInt(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
