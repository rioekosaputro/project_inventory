//go:build integration

package http_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/testutil/pgtest"
)

func issueDOForSO(t *testing.T, e *env, biz string, soBody map[string]any, warehouseID int64) (int64, int64) {
	t.Helper()
	soRes := e.post(biz, "/api/v1/sales-orders", soBody)
	require.Equal(t, 201, soRes.status(), soRes.rec.Body.String())
	soID := int64(soRes.data()["id"].(float64))
	require.Equal(t, 200, e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/confirm", soID), nil).status())
	doRes := e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/delivery-orders", soID), map[string]any{
		"warehouse_id": warehouseID,
	})
	require.Equal(t, 201, doRes.status(), doRes.rec.Body.String())
	dos := doRes.list()
	require.Len(t, dos, 1)
	doID := int64(dos[0]["id"].(float64))

	det := e.get(biz, fmt.Sprintf("/api/v1/delivery-orders/%d", doID))
	require.Equal(t, 200, det.status(), det.rec.Body.String())
	items := det.data()["items"].([]any)
	require.Len(t, items, 1)
	doItemID := int64(items[0].(map[string]any)["id"].(float64))
	return doID, doItemID
}

func TestDeliveryOrder_ReserveCreateLPBAndApprove_WithInvariants(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-DO-01")
	ctrID := seedActiveContract(t, e, biz, cid, sid, f.p1)

	// Terima stok awal: 2 unit SN (p1) dan 10 unit non-SN (p2) di WH-A / a1.
	e.receive(f, f.whA, f.a1, f.p1, 2, "SN-DO-001", "SN-DO-002")
	e.receive(f, f.whA, f.a1, f.p2, 10)

	var serialID int64
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT id FROM product_serials WHERE serial_number = 'SN-DO-001'`).Scan(&serialID))

	// 1. DO OUTBOUND untuk RENTAL_DELIVERY (SN).
	doID, doItemID := issueDOForSO(t, e, biz, map[string]any{
		"order_type":       "NEW_DELIVERY",
		"customer_id":      cid,
		"customer_site_id": sid,
		"contract_id":      ctrID,
		"items":            []map[string]any{{"product_id": f.p1, "qty": 1, "unit_price": "500000.00"}},
	}, f.whA)

	// Warehouse scoping: staffB (WH-B) tidak boleh mengakses DO milik WH-A.
	assert.Equal(t, 403, e.get(f.staffB, fmt.Sprintf("/api/v1/delivery-orders/%d", doID)).status())

	// Reserve SN pada DO oleh staffA.
	resReply := e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/reserve", doID), map[string]any{
		"items": []map[string]any{
			{"do_item_id": doItemID, "rack_id": f.a1, "qty": 1, "serial_ids": []int64{serialID}},
		},
	})
	require.Equal(t, 200, resReply.status(), resReply.rec.Body.String())
	assert.Equal(t, "RESERVED", resReply.data()["status"])
	pgtest.AssertInvariants(t, e.pool)

	// Buat draft LPB dari DO.
	lpbReply := e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/lpb", doID), nil)
	require.Equal(t, 201, lpbReply.status(), lpbReply.rec.Body.String())
	lpbID := int64(lpbReply.data()["id"].(float64))
	assert.Equal(t, "RENTAL_DELIVERY", lpbReply.data()["sub_type"])

	// Submit oleh staffA → Approve oleh headA (memakai jalur ConsumeReservedStock + DispatchReservedSerial).
	require.Equal(t, 200, e.post(f.staffA, fmt.Sprintf("/api/v1/lpb/%d/submit", lpbID), nil).status())
	appReply := e.approve(f.headA, lpbID, "do-lpb-sn-1")
	require.Equal(t, 200, appReply.status(), appReply.rec.Body.String())

	// Verifikasi saldo, status unit IN_TRANSIT, delivery_status=READY, dan invariant = 0 pelanggaran.
	var onHand, reserved int
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT qty_on_hand, qty_reserved FROM stock_balances WHERE product_id = $1 AND rack_id = $2`, f.p1, f.a1).Scan(&onHand, &reserved))
	assert.Equal(t, 1, onHand)
	assert.Equal(t, 0, reserved)

	var snStatus string
	var snCust *int64
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT status, current_customer_id FROM product_serials WHERE id = $1`, serialID).Scan(&snStatus, &snCust))
	assert.Equal(t, "IN_TRANSIT", snStatus)
	require.NotNil(t, snCust)
	assert.Equal(t, cid, *snCust)

	var delStatus *string
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT delivery_status FROM lpb_documents WHERE id = $1`, lpbID).Scan(&delStatus))
	require.NotNil(t, delStatus)
	assert.Equal(t, "READY", *delStatus)
	pgtest.AssertInvariants(t, e.pool)

	// 2. DO OUTBOUND untuk CONSUMABLE_DELIVERY (non-SN, qty 4 dari 10).
	do2ID, do2ItemID := issueDOForSO(t, e, biz, map[string]any{
		"order_type":       "CONSUMABLE",
		"customer_id":      cid,
		"customer_site_id": sid,
		"items":            []map[string]any{{"product_id": f.p2, "qty": 4, "unit_price": "150000.00"}},
	}, f.whA)
	require.Equal(t, 200, e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/reserve", do2ID), map[string]any{
		"items": []map[string]any{{"do_item_id": do2ItemID, "rack_id": f.a1, "qty": 4}},
	}).status())
	lpb2Reply := e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/lpb", do2ID), nil)
	require.Equal(t, 201, lpb2Reply.status(), lpb2Reply.rec.Body.String())
	lpb2ID := int64(lpb2Reply.data()["id"].(float64))
	require.Equal(t, 200, e.post(f.staffA, fmt.Sprintf("/api/v1/lpb/%d/submit", lpb2ID), nil).status())
	app2 := e.approve(f.headA, lpb2ID, "do-lpb-cons-1")
	require.Equal(t, 200, app2.status(), app2.rec.Body.String())

	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT qty_on_hand, qty_reserved FROM stock_balances WHERE product_id = $1 AND rack_id = $2`, f.p2, f.a1).Scan(&onHand, &reserved))
	assert.Equal(t, 6, onHand)
	assert.Equal(t, 0, reserved)
	pgtest.AssertInvariants(t, e.pool)
}

func TestDeliveryOrder_CancelReleasesReservedStock(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-DO-REL")
	ctrID := seedActiveContract(t, e, biz, cid, sid, f.p1)

	e.receive(f, f.whA, f.a1, f.p1, 1, "SN-DO-REL-1")
	var serialID int64
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT id FROM product_serials WHERE serial_number = 'SN-DO-REL-1'`).Scan(&serialID))

	doID, doItemID := issueDOForSO(t, e, biz, map[string]any{
		"order_type":       "NEW_DELIVERY",
		"customer_id":      cid,
		"customer_site_id": sid,
		"contract_id":      ctrID,
		"items":            []map[string]any{{"product_id": f.p1, "qty": 1}},
	}, f.whA)

	require.Equal(t, 200, e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/reserve", doID), map[string]any{
		"items": []map[string]any{{"do_item_id": doItemID, "rack_id": f.a1, "qty": 1, "serial_ids": []int64{serialID}}},
	}).status())

	// Cancel DO → qty_reserved kembali 0 dan serial kembali AVAILABLE.
	cn := e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/cancel", doID), nil)
	require.Equal(t, 200, cn.status(), cn.rec.Body.String())
	assert.Equal(t, "CANCELLED", cn.data()["status"])

	var reserved int
	var snStatus string
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT qty_reserved FROM stock_balances WHERE product_id = $1 AND rack_id = $2`, f.p1, f.a1).Scan(&reserved))
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT status FROM product_serials WHERE id = $1`, serialID).Scan(&snStatus))
	assert.Equal(t, 0, reserved)
	assert.Equal(t, "AVAILABLE", snStatus)
	pgtest.AssertInvariants(t, e.pool)
}

func TestDeliveryOrder_ParallelReserveSameSerial_ExactlyOneWins(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-DO-RACE")
	ctrID := seedActiveContract(t, e, biz, cid, sid, f.p1)

	e.receive(f, f.whA, f.a1, f.p1, 1, "SN-DO-RACE-1")
	var serialID int64
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT id FROM product_serials WHERE serial_number = 'SN-DO-RACE-1'`).Scan(&serialID))

	do1ID, do1ItemID := issueDOForSO(t, e, biz, map[string]any{
		"order_type":       "NEW_DELIVERY",
		"customer_id":      cid,
		"customer_site_id": sid,
		"contract_id":      ctrID,
		"items":            []map[string]any{{"product_id": f.p1, "qty": 1}},
	}, f.whA)
	do2ID, do2ItemID := issueDOForSO(t, e, biz, map[string]any{
		"order_type":       "NEW_DELIVERY",
		"customer_id":      cid,
		"customer_site_id": sid,
		"contract_id":      ctrID,
		"items":            []map[string]any{{"product_id": f.p1, "qty": 1}},
	}, f.whA)

	start := make(chan struct{})
	statuses := make([]int, 2)
	var wg sync.WaitGroup
	targets := [][2]int64{{do1ID, do1ItemID}, {do2ID, do2ItemID}}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			r := e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/reserve", targets[idx][0]), map[string]any{
				"items": []map[string]any{
					{"do_item_id": targets[idx][1], "rack_id": f.a1, "qty": 1, "serial_ids": []int64{serialID}},
				},
			})
			statuses[idx] = r.status()
		}(i)
	}
	close(start)
	wg.Wait()

	okCount, failCount := 0, 0
	for _, st := range statuses {
		if st == 200 {
			okCount++
		} else {
			failCount++
		}
	}
	assert.Equal(t, 1, okCount, "tepat 1 reservasi paralel harus menang, got %v", statuses)
	assert.Equal(t, 1, failCount, "tepat 1 reservasi paralel harus gagal, got %v", statuses)
	pgtest.AssertInvariants(t, e.pool)
}
