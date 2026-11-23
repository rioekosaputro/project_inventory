//go:build integration

package http_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/testutil/pgtest"
)

// ---- helpers T1.8 ----

// receive menambah stok lewat INITIAL_STOCK yang disetujui (saldo selalu punya ledger). sns bersifat opsional
// (produk SN); condition dipakai untuk semua SN.
func (e *env) receive(f lpbFx, wh, rack, product int64, qty int, sns ...string) {
	e.t.Helper()
	staff, head := f.staffA, f.headG
	if wh == f.whB {
		staff = f.staffB
	}
	body := map[string]any{"sub_type": "INITIAL_STOCK", "warehouse_id": wh, "items": []map[string]any{lpbItem(product, rack, qty)}}
	id, _ := e.pending(staff, body, map[int][]string{0: sns})
	r := e.approve(head, id, nextKey())
	require.Equal(e.t, 200, r.status(), r.rec.Body.String())
}

// receiveBroken menerima unit rusak ke rak non-jual lewat INITIAL_STOCK (kondisi BROKEN).
func (e *env) receiveBroken(f lpbFx, rack, product int64, sns ...string) {
	e.t.Helper()
	body := map[string]any{"sub_type": "INITIAL_STOCK", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(product, rack, len(sns))}}
	r := e.mustLPB(f.staffA, body)
	for _, sn := range sns {
		s := e.scan(f.staffA, idOf(r), itemID(r, 0), sn, map[string]any{"condition": "BROKEN"})
		require.Equal(e.t, 201, s.status(), s.rec.Body.String())
	}
	require.Equal(e.t, 200, e.post(f.staffA, lpbPath(idOf(r), "/submit"), nil).status())
	ap := e.approve(f.headG, idOf(r), nextKey())
	require.Equal(e.t, 200, ap.status(), ap.rec.Body.String())
}

func (e *env) tag(sn string) string {
	e.t.Helper()
	var tag string
	require.NoError(e.t, e.pool.QueryRow(testCtx(), `SELECT asset_tag FROM product_serials WHERE serial_number = $1`, sn).Scan(&tag))
	return tag
}

type unitRow struct {
	status string
	rack   *int64
	wh     *int64
}

func (e *env) unit(sn string) unitRow {
	e.t.Helper()
	var u unitRow
	require.NoError(e.t, e.pool.QueryRow(testCtx(),
		`SELECT status, current_rack_id, current_warehouse_id FROM product_serials WHERE serial_number = $1`, sn).Scan(&u.status, &u.rack, &u.wh))
	return u
}

func (e *env) bal(product, rack int64) int {
	n, _ := e.balance(product, rack)
	return n
}

func transfer(wh int64, items ...map[string]any) map[string]any {
	return map[string]any{"sub_type": "TRANSFER_OUT", "warehouse_id": wh, "items": items}
}

func toRack(product, rack int64, qty int, dest int64) map[string]any {
	return lpbItem(product, rack, qty, map[string]any{"dest_rack_id": dest})
}

func disposal(wh int64, items ...map[string]any) map[string]any {
	return map[string]any{"sub_type": "DISPOSAL", "warehouse_id": wh, "items": items}
}

func (f lpbFx) newRackIn(e *env, wh int64, code string, extra map[string]any) int64 {
	return idOf(e.newRack(f.admin, wh, code, extra))
}

// ================================================================ DISPOSAL

func TestDisposal_RetiresBrokenUnitsAndRemovesStock(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	e.receiveBroken(f, f.a2, f.p1, "BRK-1", "BRK-2", "BRK-3")
	require.Equal(t, 3, e.bal(f.p1, f.a2))

	// Scan asset tag untuk unit pertama, SN untuk unit kedua (keduanya diterima).
	id, _ := e.pending(f.staffA, disposal(f.whA, lpbItem(f.p1, f.a2, 2)), map[int][]string{0: {e.tag("BRK-1"), "brk-2"}})
	r := e.approve(f.headA, id, nextKey())
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	assert.Equal(t, "APPROVED", r.str("status"))

	assert.Equal(t, 1, e.bal(f.p1, f.a2), "3 − 2")
	for _, sn := range []string{"BRK-1", "BRK-2"} {
		u := e.unit(sn)
		assert.Equal(t, "RETIRED", u.status)
		assert.Nil(t, u.rack, "unit yang dihapus tidak lagi berada di rak")
		assert.Nil(t, u.wh)
	}
	assert.Equal(t, "BROKEN", e.unit("BRK-3").status, "unit yang tidak ikut tetap utuh")
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM product_serial_histories WHERE event_type = 'LPB_APPROVED'
	   AND from_status = 'BROKEN' AND to_status = 'RETIRED' AND user_id = $1 AND reference_id = $2`, f.headAID, id))

	var before, after, qty int
	var dir string
	var serials []int64
	require.NoError(t, e.pool.QueryRow(testCtx(),
		`SELECT direction, balance_before, balance_after, qty, serial_ids FROM stock_ledgers WHERE lpb_id = $1`, id).Scan(&dir, &before, &after, &qty, &serials))
	assert.Equal(t, "OUT", dir)
	assert.Equal(t, []int{3, 1, 2}, []int{before, after, qty})
	assert.Len(t, serials, 2)

	d := e.get(f.mgmt, lpbPath(id, ""))
	item := itemsOf(d)[0]
	assert.EqualValues(t, 3, item["balance_before"])
	assert.EqualValues(t, 1, item["balance_after"])
	assert.Equal(t, []string{"LPB_CREATE", "LPB_SUBMIT", "LPB_APPROVE"}, timelineActions(d))
	pgtest.AssertInvariants(t, e.pool)
}

func TestDisposal_NonSerialFromQuarantine_AndInsufficientStockRollsBack(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	e.receive(f, f.whA, f.a2, f.p2, 5) // toner rusak di rak karantina (non-jual)

	// Dua dokumen masing-masing 3 dari 5: keduanya lolos submit (stok bebas 5), hanya satu yang bisa disetujui.
	first, _ := e.pending(f.staffA, disposal(f.whA, lpbItem(f.p2, f.a2, 3)), nil)
	second, _ := e.pending(f.staffA, disposal(f.whA, lpbItem(f.p2, f.a2, 3)), nil)
	require.Equal(t, 200, e.approve(f.headA, first, nextKey()).status(), "rak non-jual tetap boleh dikeluarkan")
	assert.Equal(t, 2, e.bal(f.p2, f.a2))

	r := e.approve(f.headA, second, nextKey())
	assert.Equal(t, 409, r.status(), r.rec.Body.String())
	assert.Equal(t, "INSUFFICIENT_STOCK", r.code())
	assert.Equal(t, 2, e.bal(f.p2, f.a2), "gagal = saldo tidak berubah")
	assert.Equal(t, "PENDING_APPROVAL", e.get(f.headA, lpbPath(second, "")).str("status"))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM stock_ledgers WHERE lpb_id = $1`, second))

	// Peringatan dini saat submit: meminta lebih dari stok bebas ditolak di tahap pengajuan.
	big := e.mustLPB(f.staffA, disposal(f.whA, lpbItem(f.p2, f.a2, 9)))
	sub := e.post(f.staffA, lpbPath(idOf(big), "/submit"), nil)
	assert.Equal(t, 409, sub.status())
	assert.Equal(t, "INSUFFICIENT_STOCK", sub.code())
	pgtest.AssertInvariants(t, e.pool)
}

func TestDisposal_OnlyBrokenUnitsInTheItemRackCanBeScanned(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	e.receive(f, f.whA, f.a1, f.p1, 1, "GOOD-1") // AVAILABLE di rak jual
	e.receiveBroken(f, f.a2, f.p1, "BRK-1")

	avail := e.mustLPB(f.staffA, disposal(f.whA, lpbItem(f.p1, f.a1, 1)))
	r := e.scan(f.staffA, idOf(avail), itemID(avail, 0), "GOOD-1")
	assert.Equal(t, 422, r.status(), "unit AVAILABLE tidak boleh dihapus")
	assert.Equal(t, "SERIAL_NOT_VALID", r.code())

	wrongRack := e.mustLPB(f.staffA, disposal(f.whA, lpbItem(f.p1, f.a1, 1)))
	r = e.scan(f.staffA, idOf(wrongRack), itemID(wrongRack, 0), "BRK-1")
	assert.Equal(t, 422, r.status(), "unit BROKEN di rak lain bukan milik item ini")
	pgtest.AssertInvariants(t, e.pool)
}

// ================================================================ TRANSFER satu gudang

func TestTransfer_SameWarehouse_MovesBetweenRacksAndAutoApprovesPair(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	a3 := f.newRackIn(e, f.whA, "A-03", map[string]any{"zone": "PRINTER"})
	e.receive(f, f.whA, f.a1, f.p1, 3, "SN-1", "SN-2", "SN-3")
	e.receive(f, f.whA, f.a1, f.p2, 6)

	id, _ := e.pending(f.staffA, transfer(f.whA, toRack(f.p1, f.a1, 2, a3), toRack(f.p2, f.a1, 4, a3)),
		map[int][]string{0: {"sn-1", e.tag("SN-2")}})
	r := e.approve(f.headA, id, nextKey())
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	assert.Equal(t, "APPROVED", r.str("status"))
	require.NotNil(t, r.data()["counterpart_id"])
	inID := int64(r.data()["counterpart_id"].(float64))

	// saldo: keluar dari A-01, masuk ke A-03 (sisa SN-3 tetap di A-01)
	assert.Equal(t, []int{1, 2, 2, 4}, []int{e.bal(f.p1, f.a1), e.bal(f.p1, a3), e.bal(f.p2, f.a1), e.bal(f.p2, a3)})

	// unit: kembali AVAILABLE di rak tujuan, dua riwayat per unit (berangkat lalu tiba)
	for _, sn := range []string{"SN-1", "SN-2"} {
		u := e.unit(sn)
		assert.Equal(t, "AVAILABLE", u.status)
		require.NotNil(t, u.rack)
		assert.Equal(t, a3, *u.rack)
		require.NotNil(t, u.wh)
		assert.Equal(t, f.whA, *u.wh)
	}
	assert.Equal(t, "AVAILABLE", e.unit("SN-3").status)
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM product_serial_histories WHERE from_status = 'AVAILABLE' AND to_status = 'IN_TRANSIT' AND reference_id = $1`, id))
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM product_serial_histories WHERE from_status = 'IN_TRANSIT' AND to_status = 'AVAILABLE' AND reference_id = $1`, inID))

	// pasangan: LPBI disetujui otomatis oleh approver yang sama, dibuat atas nama pembuat OUT, saling menaut
	in := e.get(f.mgmt, lpbPath(inID, ""))
	assert.Equal(t, "TRANSFER_IN", in.str("sub_type"))
	assert.Equal(t, "APPROVED", in.str("status"))
	assert.Equal(t, "INBOUND", in.str("type"))
	assert.Contains(t, in.str("lpb_number"), "LPBI/WH-A/")
	assert.EqualValues(t, f.headAID, in.data()["approved_by"])
	assert.EqualValues(t, f.staffAID, in.data()["created_by"])
	assert.EqualValues(t, id, in.data()["counterpart_id"])
	assert.Equal(t, []string{"LPB_CREATE", "LPB_SUBMIT", "LPB_APPROVE"}, timelineActions(in))
	inItems := itemsOf(in)
	require.Len(t, inItems, 2)
	assert.EqualValues(t, a3, inItems[0]["rack_id"])

	out := e.get(f.mgmt, lpbPath(id, ""))
	assert.EqualValues(t, inID, out.data()["counterpart_id"])
	assert.Equal(t, in.str("lpb_number"), out.str("counterpart_number"))
	outItem := itemsOf(out)[0]
	assert.EqualValues(t, a3, outItem["dest_rack_id"])
	assert.Equal(t, "A-03", outItem["dest_rack_code"])
	assert.Equal(t, "WH-A", outItem["dest_warehouse_code"])
	assert.EqualValues(t, 3, outItem["balance_before"])
	assert.EqualValues(t, 1, outItem["balance_after"])

	// ledger: dua sisi, masing-masing menunjuk dokumennya
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM stock_ledgers WHERE lpb_id = $1 AND direction = 'OUT' AND movement_type = 'TRANSFER_OUT'`, id))
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM stock_ledgers WHERE lpb_id = $1 AND direction = 'IN' AND movement_type = 'TRANSFER_IN' AND created_by = $2`, inID, f.headAID))
	assert.Equal(t, 1, e.auditCount("LPB_APPROVE", inID))
	pgtest.AssertInvariants(t, e.pool)
}

func TestTransfer_MergesItemsHeadedForTheSameDestinationRack(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	a3 := f.newRackIn(e, f.whA, "A-03", map[string]any{"zone": "PRINTER"})
	a4 := f.newRackIn(e, f.whA, "A-04", map[string]any{"zone": "PRINTER"})
	e.receive(f, f.whA, f.a1, f.p2, 5)
	e.receive(f, f.whA, a3, f.p2, 5)

	id, _ := e.pending(f.staffA, transfer(f.whA, toRack(f.p2, f.a1, 1, a4), toRack(f.p2, a3, 2, a4)), nil)
	r := e.approve(f.headA, id, nextKey())
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	inID := int64(r.data()["counterpart_id"].(float64))

	inItems := itemsOf(e.get(f.mgmt, lpbPath(inID, "")))
	require.Len(t, inItems, 1, "dua sumber ke rak tujuan yang sama menjadi satu baris pasangan")
	assert.EqualValues(t, 3, inItems[0]["qty"])
	assert.Equal(t, 3, e.bal(f.p2, a4))
	assert.Equal(t, 4, e.bal(f.p2, f.a1))
	assert.Equal(t, 3, e.bal(f.p2, a3))
	pgtest.AssertInvariants(t, e.pool)
}

// ================================================================ TRANSFER antar gudang

func TestTransfer_CrossWarehouse_PairWaitsForDestinationHead(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	_, headB := e.userToken(domain.RoleHeadWarehouse, &f.whB)
	e.receive(f, f.whA, f.a1, f.p1, 2, "SN-1", "SN-2")
	e.receive(f, f.whA, f.a1, f.p2, 6)

	id, _ := e.pending(f.staffA, transfer(f.whA, toRack(f.p1, f.a1, 2, f.b1), toRack(f.p2, f.a1, 4, f.b1)),
		map[int][]string{0: {"SN-1", "SN-2"}})
	r := e.approve(f.headA, id, nextKey())
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	assert.Equal(t, "APPROVED", r.str("status"))
	inID := int64(r.data()["counterpart_id"].(float64))

	// Sisi keluar selesai: stok asal berkurang, unit di perjalanan (tanpa rak/gudang); sisi tujuan belum ada.
	assert.Equal(t, []int{0, 2}, []int{e.bal(f.p1, f.a1), e.bal(f.p2, f.a1)})
	_, found := e.balance(f.p1, f.b1)
	assert.False(t, found, "stok tujuan belum bertambah sebelum diterima")
	for _, sn := range []string{"SN-1", "SN-2"} {
		u := e.unit(sn)
		assert.Equal(t, "IN_TRANSIT", u.status)
		assert.Nil(t, u.rack)
	}
	in := e.get(headB, lpbPath(inID, ""))
	assert.Equal(t, "PENDING_APPROVAL", in.str("status"))
	assert.EqualValues(t, f.whB, in.data()["warehouse_id"])
	assert.EqualValues(t, f.headAID, in.data()["submitted_by"])
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM stock_ledgers WHERE lpb_id = $1`, inID))
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM lpb_item_serials s JOIN lpb_items i ON i.id = s.lpb_item_id WHERE i.lpb_id = $1`, inID), "unit yang sama tercatat di pasangan")

	// Antrean: hanya Kepala Gudang tujuan yang melihatnya, dan hanya dia yang boleh memutuskan.
	queueB := e.get(headB, "/api/v1/lpb/approvals")
	require.Equal(t, 200, queueB.status())
	assert.Len(t, queueB.list(), 1)
	assert.Equal(t, "TRANSFER_IN", queueB.list()[0]["sub_type"])
	assert.Empty(t, e.get(f.headA, "/api/v1/lpb/approvals").list(), "gudang asal tidak melihatnya")
	wrong := e.approve(f.headA, inID, nextKey())
	assert.Equal(t, 403, wrong.status())
	assert.Equal(t, "FORBIDDEN_WAREHOUSE", wrong.code())

	// Transfer masuk tidak dapat ditolak (barang sudah keluar dari asal).
	rej := e.reject(headB, inID, "barang belum tiba")
	assert.Equal(t, 422, rej.status())
	assert.Contains(t, fieldErrors(rej), "sub_type")
	assert.Equal(t, "PENDING_APPROVAL", e.get(headB, lpbPath(inID, "")).str("status"))

	// Barang tiba: Kepala Gudang tujuan menyetujui → stok dan unit pindah ke gudang tujuan.
	ap := e.approve(headB, inID, nextKey())
	require.Equal(t, 200, ap.status(), ap.rec.Body.String())
	assert.Equal(t, []int{2, 4}, []int{e.bal(f.p1, f.b1), e.bal(f.p2, f.b1)})
	for _, sn := range []string{"SN-1", "SN-2"} {
		u := e.unit(sn)
		assert.Equal(t, "AVAILABLE", u.status)
		require.NotNil(t, u.rack)
		assert.Equal(t, f.b1, *u.rack)
		assert.Equal(t, f.whB, *u.wh)
	}
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM stock_ledgers WHERE lpb_id = $1 AND direction = 'IN' AND movement_type = 'TRANSFER_IN'`, inID))
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM product_serial_histories WHERE from_status = 'IN_TRANSIT' AND to_status = 'AVAILABLE' AND reference_id = $1`, inID))
	assert.Equal(t, 409, e.approve(headB, inID, nextKey()).status(), "tidak dapat disetujui dua kali")
	pgtest.AssertInvariants(t, e.pool)
}

// ================================================================ validasi

func TestTransfer_Validation(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	a3 := f.newRackIn(e, f.whA, "A-03", map[string]any{"zone": "PRINTER"})
	offRack := f.newRackIn(e, f.whA, "A-OFF", nil)
	e.exec(`UPDATE racks SET is_active = FALSE WHERE id = $1`, offRack)
	e.receive(f, f.whA, f.a1, f.p1, 1, "SN-1")
	e.receive(f, f.whA, f.a1, f.p2, 2)
	e.receiveBroken(f, f.a2, f.p3, "BRK-9")

	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"rak tujuan sama dengan rak asal", transfer(f.whA, toRack(f.p2, f.a1, 1, f.a1)), "items[0].dest_rack_id"},
		{"rak tujuan tidak ditemukan", transfer(f.whA, toRack(f.p2, f.a1, 1, 999999)), "items[0].dest_rack_id"},
		{"rak tujuan tidak aktif", transfer(f.whA, toRack(f.p2, f.a1, 1, offRack)), "items[0].dest_rack_id"},
		{"produk serial hanya ke rak yang dapat dijual", transfer(f.whA, toRack(f.p1, f.a1, 1, f.a2)), "items[0].dest_rack_id"},
		{"banyak gudang tujuan dalam satu dokumen", transfer(f.whA, toRack(f.p2, f.a1, 1, a3), toRack(f.p1, f.a1, 1, f.b1)), "items[1].dest_rack_id"},
	}
	for _, c := range cases {
		r := e.newLPB(f.staffA, c.body)
		assert.Equal(t, 422, r.status(), "%s: %s", c.name, r.rec.Body.String())
		assert.Contains(t, fieldErrors(r), c.field, c.name)
	}

	// Hanya unit AVAILABLE di rak item yang boleh di-scan untuk transfer.
	tr := e.mustLPB(f.staffA, transfer(f.whA, toRack(f.p3, f.a2, 1, a3)))
	r := e.scan(f.staffA, idOf(tr), itemID(tr, 0), "BRK-9")
	assert.Equal(t, 422, r.status(), "unit BROKEN tidak boleh dipindahkan")
	assert.Equal(t, "SERIAL_NOT_VALID", r.code())

	// Stok bebas non-SN diperiksa saat submit.
	big := e.mustLPB(f.staffA, transfer(f.whA, toRack(f.p2, f.a1, 9, a3)))
	sub := e.post(f.staffA, lpbPath(idOf(big), "/submit"), nil)
	assert.Equal(t, 409, sub.status())
	assert.Equal(t, "INSUFFICIENT_STOCK", sub.code())

	// Mengubah tujuan lewat PATCH ikut tersimpan.
	ok := e.mustLPB(f.staffA, transfer(f.whA, toRack(f.p2, f.a1, 1, a3)))
	patched := e.patchReq(f.staffA, lpbPath(idOf(ok), ""), map[string]any{"items": []map[string]any{toRack(f.p2, f.a1, 1, f.b1)}})
	require.Equal(t, 200, patched.status(), patched.rec.Body.String())
	assert.EqualValues(t, f.b1, itemsOf(patched)[0]["dest_rack_id"])
	assert.Equal(t, "WH-B", itemsOf(patched)[0]["dest_warehouse_code"])
	pgtest.AssertInvariants(t, e.pool)
}

// ================================================================ atomik

func TestTransfer_FailureRollsBackEverything(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	a3 := f.newRackIn(e, f.whA, "A-03", map[string]any{"zone": "PRINTER"})
	e.receive(f, f.whA, f.a1, f.p1, 2, "SN-1", "SN-2")
	e.receive(f, f.whA, f.a1, f.p2, 5)

	// Dokumen rival menghabiskan toner setelah dokumen utama diajukan.
	main, _ := e.pending(f.staffA, transfer(f.whA, toRack(f.p1, f.a1, 2, a3), toRack(f.p2, f.a1, 4, a3)), map[int][]string{0: {"SN-1", "SN-2"}})
	rival, _ := e.pending(f.staffA, disposal(f.whA, lpbItem(f.p2, f.a1, 3)), nil)
	require.Equal(t, 200, e.approve(f.headA, rival, nextKey()).status())
	docsBefore := e.count(`SELECT count(*) FROM lpb_documents`)
	ledgersBefore := e.count(`SELECT count(*) FROM stock_ledgers`)

	r := e.approve(f.headA, main, nextKey())
	assert.Equal(t, 409, r.status(), r.rec.Body.String())
	assert.Equal(t, "INSUFFICIENT_STOCK", r.code())

	assert.Equal(t, "PENDING_APPROVAL", e.get(f.headA, lpbPath(main, "")).str("status"))
	assert.Equal(t, docsBefore, e.count(`SELECT count(*) FROM lpb_documents`), "pasangan TRANSFER_IN tidak tersisa")
	assert.Equal(t, ledgersBefore, e.count(`SELECT count(*) FROM stock_ledgers`))
	assert.Equal(t, []int{2, 0, 2, 0}, []int{e.bal(f.p1, f.a1), e.bal(f.p1, a3), e.bal(f.p2, f.a1), e.bal(f.p2, a3)})
	for _, sn := range []string{"SN-1", "SN-2"} {
		u := e.unit(sn)
		assert.Equal(t, "AVAILABLE", u.status, "unit kembali utuh karena transaksi dibatalkan")
		require.NotNil(t, u.rack)
		assert.Equal(t, f.a1, *u.rack)
	}
	assert.Nil(t, e.get(f.headA, lpbPath(main, "")).data()["counterpart_id"])
	pgtest.AssertInvariants(t, e.pool)
}

// ================================================================ konkurensi: urutan lock

// Transfer satu gudang yang saling berlawanan arah (A→B dan B→A) pada produk yang sama disetujui serentak.
// Persetujuan transfer satu gudang berjalan bergantian (baris urutan nomor dokumen dikunci lebih dulu) dan saldo
// disentuh berurutan (produk, rak — diuji langsung di TestApplyMoves_TouchesBalancesInProductRackOrder); tes ini
// memastikan hasil serentak tetap konsisten dan tidak ada yang deadlock.
func TestTransfer_OppositeDirectionsConcurrently_NoDeadlock(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	a3 := f.newRackIn(e, f.whA, "A-03", map[string]any{"zone": "PRINTER"})
	e.receive(f, f.whA, f.a1, f.p2, 200)
	e.receive(f, f.whA, a3, f.p2, 200)

	// Jeda kecil di setiap penulisan ledger melebarkan jendela balapan agar tumpang tindih benar-benar terjadi.
	e.exec(`CREATE FUNCTION slow_ledger() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.03); RETURN NEW; END $$`)
	e.exec(`CREATE TRIGGER trg_slow_ledger BEFORE INSERT ON stock_ledgers FOR EACH ROW EXECUTE FUNCTION slow_ledger()`)

	// Banyak dokumen berlawanan arah disetujui serentak agar tumpang tindih lock benar-benar terjadi.
	const rounds, perDirection = 3, 3
	for i := 0; i < rounds; i++ {
		var ids []int64
		for k := 0; k < perDirection; k++ {
			x, _ := e.pending(f.staffA, transfer(f.whA, toRack(f.p2, f.a1, 2, a3)), nil)
			y, _ := e.pending(f.staffA, transfer(f.whA, toRack(f.p2, a3, 3, f.a1)), nil)
			ids = append(ids, x, y)
		}
		var wg sync.WaitGroup
		res := make([]*reply, len(ids))
		for n, id := range ids {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res[n] = e.approve(f.headA, id, nextKey())
			}()
		}
		wg.Wait()
		for n, r := range res {
			require.Equal(t, 200, r.status(), "putaran %d dokumen %d: %s", i, n, r.rec.Body.String())
		}
	}
	moved := rounds * perDirection
	assert.Equal(t, 200-2*moved+3*moved, e.bal(f.p2, f.a1))
	assert.Equal(t, 200+2*moved-3*moved, e.bal(f.p2, a3))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM lpb_documents WHERE sub_type = 'TRANSFER_IN' AND status <> 'APPROVED'`))
	pgtest.AssertInvariants(t, e.pool)
}
