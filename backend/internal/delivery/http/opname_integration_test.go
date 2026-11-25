//go:build integration

package http_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/testutil/pgtest"
)

func opnamePath(id int64, suffix string) string {
	return fmt.Sprintf("/api/v1/inventory/opnames/%d%s", id, suffix)
}

func TestOpname_FullLifecycle_LockAdjustInvariantsAndRediscoverLost(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	a3 := f.newRackIn(e, f.whA, "A-03", map[string]any{"zone": "PRINTER"})
	catCons := e.newCategory(f.admin, "Kertas", "CONSUMABLE")
	p4 := idOf(e.newProduct(f.admin, catCons, "KRT-A4", map[string]any{"name": "Kertas A4"}))

	// Stok awal di A-01: 2 printer SN (SN-OPN-1, SN-OPN-2) + 10 toner non-SN; dan 1 PC di A-03.
	e.receive(f, f.whA, f.a1, f.p1, 2, "SN-OPN-1", "SN-OPN-2")
	e.receive(f, f.whA, f.a1, f.p2, 10)
	e.receive(f, f.whA, a3, f.p3, 1, "SN-IN-A3")

	// RBAC: driver tidak boleh membaca/membuat opname.
	assert.Equal(t, 403, e.get(f.driver, "/api/v1/inventory/opnames").status())
	assert.Equal(t, 403, e.post(f.mgmt, "/api/v1/inventory/opnames", map[string]any{"warehouse_id": f.whA, "rack_ids": []int64{f.a1}}).status())

	// 1. Mulai opname (blind count = true) pada rak A-01.
	start := e.post(f.staffA, "/api/v1/inventory/opnames", map[string]any{
		"warehouse_id":   f.whA,
		"rack_ids":       []int64{f.a1},
		"is_blind_count": true,
		"notes":          "Opname rutin bulanan",
	})
	require.Equal(t, 201, start.status(), start.rec.Body.String())
	opID := idOf(start)
	assert.Equal(t, "IN_PROGRESS", start.str("status"))
	assert.Contains(t, start.str("opname_number"), "OPN/WH-A/")

	// 2. DoD: LPB lain & opname kedua pada rak terkunci ditolak 409 RACK_LOCKED_BY_OPNAME.
	blockedLPB := e.newLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1)))
	assert.Equal(t, 409, blockedLPB.status())
	assert.Equal(t, "RACK_LOCKED_BY_OPNAME", blockedLPB.code())

	blockedOp := e.post(f.staffA, "/api/v1/inventory/opnames", map[string]any{"warehouse_id": f.whA, "rack_ids": []int64{f.a1}})
	assert.Equal(t, 409, blockedOp.status())
	assert.Equal(t, "RACK_LOCKED_BY_OPNAME", blockedOp.code())

	// 3. Blind count: staf tidak melihat system_qty; kepala gudang melihatnya.
	staffView := e.get(f.staffA, opnamePath(opID, ""))
	require.Equal(t, 200, staffView.status())
	staffItems := itemsOf(staffView)
	require.Len(t, staffItems, 1)
	assert.Nil(t, staffItems[0]["system_qty"], "blind count menyembunyikan qty sistem dari staf")

	headView := e.get(f.headA, opnamePath(opID, ""))
	require.Equal(t, 200, headView.status())
	assert.EqualValues(t, 10, itemsOf(headView)[0]["system_qty"])

	// Submit sebelum semua item dihitung → 422.
	assert.Equal(t, 422, e.post(f.staffA, opnamePath(opID, "/submit"), nil).status())

	// 4. Input hitungan non-SN: toner 10→7 (kurang 3), kertas 0→4 (surplus 4).
	cnt := e.call("PUT", opnamePath(opID, "/counts"), map[string]any{
		"items": []map[string]any{
			{"product_id": f.p2, "rack_id": f.a1, "counted_qty": 7},
			{"product_id": p4, "rack_id": f.a1, "counted_qty": 4},
		},
	}, withBearer(f.staffA))
	require.Equal(t, 200, cnt.status(), cnt.rec.Body.String())

	// 5. Scan SN: SN-OPN-1 (FOUND), SN-OPN-2 tidak discan (tetap MISSING), SN-IN-A3 (WRONG_RACK), SN-GHOST (UNEXPECTED).
	sc1 := e.post(f.staffA, opnamePath(opID, "/scans"), map[string]any{"rack_id": f.a1, "code": e.tag("SN-OPN-1")})
	require.Equal(t, 200, sc1.status(), sc1.rec.Body.String())
	assert.Equal(t, "FOUND", sc1.str("result"))

	scWrong := e.post(f.staffA, opnamePath(opID, "/scans"), map[string]any{"rack_id": f.a1, "code": "SN-IN-A3"})
	require.Equal(t, 200, scWrong.status(), scWrong.rec.Body.String())
	assert.Equal(t, "WRONG_RACK", scWrong.str("result"))

	scUnexp := e.post(f.staffA, opnamePath(opID, "/scans"), map[string]any{"rack_id": f.a1, "code": "SN-GHOST-999"})
	require.Equal(t, 200, scUnexp.status(), scUnexp.rec.Body.String())
	assert.Equal(t, "UNEXPECTED", scUnexp.str("result"))

	// 6. Submit oleh staf → SUBMITTED.
	sub := e.post(f.staffA, opnamePath(opID, "/submit"), nil)
	require.Equal(t, 200, sub.status(), sub.rec.Body.String())
	assert.Equal(t, "SUBMITTED", sub.str("status"))

	// Staf tidak boleh finalize (403).
	assert.Equal(t, 403, e.post(f.staffA, opnamePath(opID, "/finalize"), nil).status())

	// 7. Finalize oleh Kepala Gudang → membuat & auto-approve LPB adjustment IN & OUT + membuka kunci rak.
	fin := e.post(f.headA, opnamePath(opID, "/finalize"), nil)
	require.Equal(t, 200, fin.status(), fin.rec.Body.String())
	assert.Equal(t, "FINALIZED", fin.str("status"))
	assert.NotNil(t, fin.data()["adjustment_in_lpb_id"])
	assert.NotNil(t, fin.data()["adjustment_out_lpb_id"])

	// Saldo & unit terbarui akurat.
	assert.Equal(t, 7, e.bal(f.p2, f.a1), "toner disesuaikan 10 → 7")
	assert.Equal(t, 4, e.bal(p4, f.a1), "kertas surplus 0 → 4")
	assert.Equal(t, 1, e.bal(f.p1, f.a1), "printer 2 → 1 (SN-OPN-2 hilang)")
	assert.Equal(t, "AVAILABLE", e.unit("SN-OPN-1").status)
	lostUnit := e.unit("SN-OPN-2")
	assert.Equal(t, "LOST", lostUnit.status)
	assert.Nil(t, lostUnit.rack)
	assert.Nil(t, lostUnit.wh)

	// Kunci rak terbuka → LPB baru pada A-01 berhasil.
	assert.Equal(t, 201, e.newLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1))).status())
	pgtest.AssertInvariants(t, e.pool)

	// 8. Opname kedua: unit LOST (SN-OPN-2) ditemukan kembali di A-01 → kembali AVAILABLE saat finalize!
	op2 := e.post(f.staffA, "/api/v1/inventory/opnames", map[string]any{"warehouse_id": f.whA, "rack_ids": []int64{f.a1}, "is_blind_count": false})
	require.Equal(t, 201, op2.status())
	op2ID := idOf(op2)
	// Hitung ulang non-SN sesuai saldo sekarang (7 dan 4), scan SN-OPN-1 + SN-OPN-2 (yang LOST).
	require.Equal(t, 200, e.call("PUT", opnamePath(op2ID, "/counts"), map[string]any{
		"items": []map[string]any{
			{"product_id": f.p2, "rack_id": f.a1, "counted_qty": 7},
			{"product_id": p4, "rack_id": f.a1, "counted_qty": 4},
		},
	}, withBearer(f.staffA)).status())
	require.Equal(t, 200, e.post(f.staffA, opnamePath(op2ID, "/scans"), map[string]any{"rack_id": f.a1, "code": "SN-OPN-1"}).status())
	redisc := e.post(f.staffA, opnamePath(op2ID, "/scans"), map[string]any{"rack_id": f.a1, "code": "SN-OPN-2"})
	require.Equal(t, 200, redisc.status())
	assert.Equal(t, "UNEXPECTED", redisc.str("result"))
	require.Equal(t, 200, e.post(f.staffA, opnamePath(op2ID, "/submit"), nil).status())
	fin2 := e.post(f.headA, opnamePath(op2ID, "/finalize"), nil)
	require.Equal(t, 200, fin2.status(), fin2.rec.Body.String())

	assert.Equal(t, 2, e.bal(f.p1, f.a1), "SN-OPN-2 kembali menambah saldo")
	restored := e.unit("SN-OPN-2")
	assert.Equal(t, "AVAILABLE", restored.status)
	require.NotNil(t, restored.rack)
	assert.Equal(t, f.a1, *restored.rack)
	pgtest.AssertInvariants(t, e.pool)
}

func TestOpname_Cancel_UnlocksRacksWithoutChangingStock(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	e.receive(f, f.whA, f.a1, f.p2, 5)

	start := e.post(f.staffA, "/api/v1/inventory/opnames", map[string]any{"warehouse_id": f.whA, "rack_ids": []int64{f.a1}})
	require.Equal(t, 201, start.status())
	opID := idOf(start)

	assert.Equal(t, 403, e.post(f.staffA, opnamePath(opID, "/cancel"), nil).status(), "hanya HEAD_WAREHOUSE/ADMIN boleh batal")
	cancel := e.post(f.headA, opnamePath(opID, "/cancel"), nil)
	require.Equal(t, 200, cancel.status(), cancel.rec.Body.String())
	assert.Equal(t, "CANCELLED", cancel.str("status"))

	assert.Equal(t, 5, e.bal(f.p2, f.a1))
	assert.Equal(t, 201, e.newLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1))).status(), "rak kembali terbuka")
	pgtest.AssertInvariants(t, e.pool)
}
