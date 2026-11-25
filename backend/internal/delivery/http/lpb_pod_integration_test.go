//go:build integration

package http_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/testutil/pgtest"
)

func TestLPB_ManualPODAndWithdrawalPickup_FullCommercialCycle(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-POD-01")
	ctrID := seedActiveContract(t, e, biz, cid, sid, f.p1)

	// 1. Terima 1 unit SN di gudang A.
	e.receive(f, f.whA, f.a1, f.p1, 1, "SN-POD-001")
	var serialID int64
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT id FROM product_serials WHERE serial_number = 'SN-POD-001'`).Scan(&serialID))

	// 2. SO NEW_DELIVERY -> Confirm -> Issue DO OUTBOUND -> Reserve -> LPB RENTAL_DELIVERY -> Submit -> Approve.
	soRes := e.post(biz, "/api/v1/sales-orders", map[string]any{
		"order_type":       "NEW_DELIVERY",
		"customer_id":      cid,
		"customer_site_id": sid,
		"contract_id":      ctrID,
		"items":            []map[string]any{{"product_id": f.p1, "qty": 1}},
	})
	require.Equal(t, 201, soRes.status())
	soID := int64(soRes.data()["id"].(float64))
	require.Equal(t, 200, e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/confirm", soID), nil).status())

	doRes := e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/delivery-orders", soID), map[string]any{"warehouse_id": f.whA})
	require.Equal(t, 201, doRes.status())
	doID := int64(doRes.list()[0]["id"].(float64))
	doItemID := int64(e.get(biz, fmt.Sprintf("/api/v1/delivery-orders/%d", doID)).data()["items"].([]any)[0].(map[string]any)["id"].(float64))

	require.Equal(t, 200, e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/reserve", doID), map[string]any{
		"items": []map[string]any{{"do_item_id": doItemID, "rack_id": f.a1, "qty": 1, "serial_ids": []int64{serialID}}},
	}).status())

	lpbID := int64(e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/lpb", doID), nil).data()["id"].(float64))
	require.Equal(t, 200, e.post(f.staffA, fmt.Sprintf("/api/v1/lpb/%d/submit", lpbID), nil).status())
	require.Equal(t, 200, e.approve(f.headA, lpbID, "pod-lpb-1").status())

	// 3. Tandai DELIVERED (POD manual oleh Kepala WH).
	podRes := e.post(f.headA, fmt.Sprintf("/api/v1/lpb/%d/deliver", lpbID), map[string]any{
		"receiver_name":  "Bapak Hendro (GA)",
		"pod_photo_path": "uploads/pod/sj-signed.jpg",
		"delivered_date": "2026-11-10",
	})
	require.Equal(t, 200, podRes.status(), podRes.rec.Body.String())
	assert.Equal(t, "DELIVERED", podRes.data()["delivery_status"])
	assert.Equal(t, "Bapak Hendro (GA)", podRes.data()["receiver_name"])

	// Verifikasi: SN menjadi RENTED, rental_placements aktif terbentuk dengan tarif 500000.00, DO & SO COMPLETED.
	var snStatus string
	var snContract *int64
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT status, current_contract_id FROM product_serials WHERE id = $1`, serialID).Scan(&snStatus, &snContract))
	assert.Equal(t, "RENTED", snStatus)
	require.NotNil(t, snContract)
	assert.Equal(t, ctrID, *snContract)

	var placementRate string
	var startDate string
	var endDate *string
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT monthly_rate::text, start_date::text, end_date::text FROM rental_placements WHERE product_serial_id = $1`, serialID).Scan(&placementRate, &startDate, &endDate))
	assert.Equal(t, "500000.00", placementRate)
	assert.Equal(t, "2026-11-10", startDate)
	assert.Nil(t, endDate)

	var doStatus, soStatus string
	require.NoError(t, e.pool.QueryRow(t.Context(), `SELECT status FROM delivery_orders WHERE id = $1`, doID).Scan(&doStatus))
	require.NoError(t, e.pool.QueryRow(t.Context(), `SELECT status FROM sales_orders WHERE id = $1`, soID).Scan(&soStatus))
	assert.Equal(t, "COMPLETED", doStatus)
	assert.Equal(t, "COMPLETED", soStatus)
	pgtest.AssertInvariants(t, e.pool)

	// 4. Penarikan (WITHDRAWAL): SO -> Confirm -> DO INBOUND -> LPB RENTAL_RETURN -> Pickup -> Submit -> Approve.
	soWd := e.post(biz, "/api/v1/sales-orders", map[string]any{
		"order_type":       "WITHDRAWAL",
		"customer_id":      cid,
		"customer_site_id": sid,
		"contract_id":      ctrID,
		"items":            []map[string]any{{"product_id": f.p1, "qty": 1, "product_serial_id": serialID}},
	})
	require.Equal(t, 201, soWd.status(), soWd.rec.Body.String())
	soWdID := int64(soWd.data()["id"].(float64))
	require.Equal(t, 200, e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/confirm", soWdID), nil).status())
	doInRes := e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/delivery-orders", soWdID), map[string]any{"warehouse_id": f.whA})
	require.Equal(t, 201, doInRes.status())
	doInID := int64(doInRes.list()[0]["id"].(float64))

	lpbRetRes := e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/lpb", doInID), map[string]any{"rack_id": f.a1})
	require.Equal(t, 201, lpbRetRes.status(), lpbRetRes.rec.Body.String())
	lpbRetID := int64(lpbRetRes.data()["id"].(float64))

	// Tandai PICKED_UP manual → SN RENTED → IN_TRANSIT, rental_placements ditutup pada 2026-11-25.
	pkRes := e.post(f.headA, fmt.Sprintf("/api/v1/lpb/%d/pickup", lpbRetID), map[string]any{
		"pickup_date": "2026-11-25",
	})
	require.Equal(t, 200, pkRes.status(), pkRes.rec.Body.String())
	assert.Equal(t, "PICKED_UP", pkRes.data()["delivery_status"])

	var endReason *string
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT end_date::text, end_reason FROM rental_placements WHERE product_serial_id = $1`, serialID).Scan(&endDate, &endReason))
	require.NotNil(t, endDate)
	assert.Equal(t, "2026-11-25", *endDate)
	require.NotNil(t, endReason)
	assert.Equal(t, "WITHDRAWAL", *endReason)

	// Submit & Approve LPB RENTAL_RETURN → SN kembali AVAILABLE di rak a1, DO & SO WITHDRAWAL COMPLETED.
	require.Equal(t, 200, e.post(f.staffA, fmt.Sprintf("/api/v1/lpb/%d/submit", lpbRetID), nil).status())
	require.Equal(t, 200, e.approve(f.headA, lpbRetID, "pod-ret-1").status())

	require.NoError(t, e.pool.QueryRow(t.Context(), `SELECT status FROM product_serials WHERE id = $1`, serialID).Scan(&snStatus))
	assert.Equal(t, "AVAILABLE", snStatus)
	require.NoError(t, e.pool.QueryRow(t.Context(), `SELECT status FROM delivery_orders WHERE id = $1`, doInID).Scan(&doStatus))
	require.NoError(t, e.pool.QueryRow(t.Context(), `SELECT status FROM sales_orders WHERE id = $1`, soWdID).Scan(&soStatus))
	assert.Equal(t, "COMPLETED", doStatus)
	assert.Equal(t, "COMPLETED", soStatus)
	pgtest.AssertInvariants(t, e.pool)
}
