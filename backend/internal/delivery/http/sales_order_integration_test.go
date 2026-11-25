//go:build integration

package http_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
)

func seedActiveContract(t *testing.T, e *env, biz string, cid, sid, assetProdID int64) int64 {
	t.Helper()
	cRes := e.post(biz, "/api/v1/contracts", map[string]any{
		"customer_id":     cid,
		"start_date":      "2026-01-01",
		"duration_months": 12,
		"items": []map[string]any{
			{"product_id": assetProdID, "customer_site_id": sid, "qty": 5, "monthly_rate": "500000.00"},
		},
	})
	require.Equal(t, 201, cRes.status(), cRes.rec.Body.String())
	ctrID := int64(cRes.data()["id"].(float64))
	actRes := e.post(biz, fmt.Sprintf("/api/v1/contracts/%d/activate", ctrID), nil)
	require.Equal(t, 200, actRes.status(), actRes.rec.Body.String())
	return ctrID
}

func TestSalesOrder_AllFourTypesTaxAndIssueDO(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-SO-01")
	ctrID := seedActiveContract(t, e, biz, cid, sid, f.p1)

	// Pastikan tax_settings PMK 131/2024 ada di schema test.
	e.exec(`INSERT INTO tax_settings (tax_code, rate, dpp_numerator, dpp_denominator, faktur_code, effective_from, notes)
	        VALUES ('PPN', 12.00, 11, 12, '04', '2025-01-01', 'PMK 131/2024')
	        ON CONFLICT (tax_code, effective_from) DO NOTHING`)

	// 1. CONSUMABLE tanpa kontrak: 10 x 500.000 = 5.000.000 -> DPP Nilai Lain 4.583.333 -> PPN 550.000 -> Total 5.550.000.
	soCons := e.post(biz, "/api/v1/sales-orders", map[string]any{
		"order_type":       "CONSUMABLE",
		"customer_id":      cid,
		"customer_site_id": sid,
		"order_date":       "2026-11-25",
		"is_ppn":           true,
		"items": []map[string]any{
			{"product_id": f.p2, "qty": 10, "unit_price": "500000.00"},
		},
	})
	require.Equal(t, 201, soCons.status(), soCons.rec.Body.String())
	cd := soCons.data()
	assert.Equal(t, "5000000.00", cd["subtotal"])
	assert.Equal(t, "4583333.00", cd["dpp_other_value"])
	assert.Equal(t, "12.00", cd["tax_rate"])
	assert.Equal(t, "550000.00", cd["tax_amount"])
	assert.Equal(t, "5550000.00", cd["grand_total"])

	// 2. NEW_DELIVERY tanpa kontrak ditolak (422).
	assert.Equal(t, 422, e.post(biz, "/api/v1/sales-orders", map[string]any{
		"order_type":       "NEW_DELIVERY",
		"customer_id":      cid,
		"customer_site_id": sid,
		"items":            []map[string]any{{"product_id": f.p1, "qty": 2}},
	}).status())

	// 3. UNIT_SWAP wajib punya OUT dan IN (hanya OUT ditolak 422).
	assert.Equal(t, 422, e.post(biz, "/api/v1/sales-orders", map[string]any{
		"order_type":       "UNIT_SWAP",
		"customer_id":      cid,
		"customer_site_id": sid,
		"contract_id":      ctrID,
		"items":            []map[string]any{{"product_id": f.p1, "direction": "OUT", "qty": 1}},
	}).status())

	// Masukkan 1 unit RENTED milik pelanggan untuk ditarik dalam UNIT_SWAP & WITHDRAWAL.
	var rentedSerialID int64
	err := e.pool.QueryRow(t.Context(), `
		INSERT INTO product_serials (product_id, serial_number, asset_tag, status, condition, current_customer_id, current_site_id, current_contract_id)
		VALUES ($1, 'SN-SWAP-OLD', 'AST-SWAP-OLD', 'RENTED', 'GOOD', $2, $3, $4)
		RETURNING id`, f.p1, cid, sid, ctrID).Scan(&rentedSerialID)
	require.NoError(t, err)

	// UNIT_SWAP valid -> Confirm -> Issue DO menghasilkan 2 DO (OUTBOUND + INBOUND) dan SO menjadi IN_PROGRESS.
	soSwap := e.post(biz, "/api/v1/sales-orders", map[string]any{
		"order_type":       "UNIT_SWAP",
		"customer_id":      cid,
		"customer_site_id": sid,
		"contract_id":      ctrID,
		"items": []map[string]any{
			{"product_id": f.p1, "direction": "OUT", "qty": 1, "unit_price": "0.00"},
			{"product_id": f.p1, "direction": "IN", "qty": 1, "product_serial_id": rentedSerialID},
		},
	})
	require.Equal(t, 201, soSwap.status(), soSwap.rec.Body.String())
	swapID := int64(soSwap.data()["id"].(float64))

	// Sebelum confirm tidak bisa terbitkan DO (409).
	assert.Equal(t, 409, e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/delivery-orders", swapID), map[string]any{
		"warehouse_id": f.whA,
	}).status())

	// Confirm -> Issue DO.
	require.Equal(t, 200, e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/confirm", swapID), nil).status())
	doRes := e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/delivery-orders", swapID), map[string]any{
		"warehouse_id":   f.whA,
		"scheduled_date": "2026-11-26",
	})
	require.Equal(t, 201, doRes.status(), doRes.rec.Body.String())
	dos := doRes.list()
	require.Len(t, dos, 2)
	assert.Equal(t, "OUTBOUND", dos[0]["direction"])
	assert.Equal(t, "INBOUND", dos[1]["direction"])

	// Cek detail SO: status IN_PROGRESS dan delivery_orders berisi 2 DO.
	det := e.get(biz, fmt.Sprintf("/api/v1/sales-orders/%d", swapID))
	require.Equal(t, 200, det.status())
	assert.Equal(t, "IN_PROGRESS", det.data()["status"])
	assert.Len(t, det.data()["delivery_orders"].([]any), 2)

	// SO IN_PROGRESS tidak dapat dibatalkan (409).
	assert.Equal(t, 409, e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/cancel", swapID), map[string]any{
		"cancel_reason": "Coba batalkan setelah DO terbit",
	}).status())

	// 4. WITHDRAWAL valid -> Confirm -> Cancel (sebelum DO terbit) -> CANCELLED.
	soWd := e.post(biz, "/api/v1/sales-orders", map[string]any{
		"order_type":       "WITHDRAWAL",
		"customer_id":      cid,
		"customer_site_id": sid,
		"contract_id":      ctrID,
		"items": []map[string]any{
			{"product_id": f.p1, "qty": 1, "product_serial_id": rentedSerialID},
		},
	})
	require.Equal(t, 201, soWd.status(), soWd.rec.Body.String())
	wdID := int64(soWd.data()["id"].(float64))
	require.Equal(t, 200, e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/confirm", wdID), nil).status())
	cnRes := e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/cancel", wdID), map[string]any{
		"cancel_reason": "Pelanggan menunda penarikan unit",
	})
	require.Equal(t, 200, cnRes.status(), cnRes.rec.Body.String())
	assert.Equal(t, "CANCELLED", cnRes.data()["status"])
}

func TestSalesOrder_RBAC(t *testing.T) {
	e := newEnv(t)
	mgmt := e.tokenFor(domain.RoleManagement, nil)
	staff := e.tokenFor(domain.RoleStaffWarehouse, nil)

	assert.Equal(t, 200, e.get(mgmt, "/api/v1/sales-orders").status())
	assert.Equal(t, 403, e.post(mgmt, "/api/v1/sales-orders", map[string]any{}).status())
	assert.Equal(t, 403, e.get(staff, "/api/v1/sales-orders").status())
}
