//go:build integration

package http_test

import (
	"fmt"
	nethttp "net/http"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/testutil/pgtest"
)

// ---- helpers ----

func withHeader(k, v string) mod { return func(r *nethttp.Request) { r.Header.Set(k, v) } }

var keySeq atomic.Int64

// nextKey dipanggil juga dari goroutine (tes konkurensi): wajib aman terhadap race.
func nextKey() string {
	return fmt.Sprintf("test-key-%06d", keySeq.Add(1))
}

func (e *env) approve(token string, id int64, key string) *reply {
	e.t.Helper()
	mods := []mod{withBearer(token)}
	if key != "" {
		mods = append(mods, withHeader("Idempotency-Key", key))
	}
	return e.call("POST", lpbPath(id, "/approve"), nil, mods...)
}

func (e *env) reject(token string, id int64, reason string) *reply {
	e.t.Helper()
	return e.post(token, lpbPath(id, "/reject"), map[string]any{"reason": reason})
}

func (e *env) setSetting(key, jsonValue string) {
	e.t.Helper()
	e.exec(`INSERT INTO app_settings (key, value) VALUES ($1, $2::jsonb) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, jsonValue)
}

// pending membuat LPB, memindai SN, dan mengajukannya; mengembalikan id dan id item per baris.
func (e *env) pending(creator string, body map[string]any, serials map[int][]string) (int64, []int64) {
	e.t.Helper()
	r := e.mustLPB(creator, body)
	id := idOf(r)
	items := itemsOf(r)
	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = int64(it["id"].(float64))
	}
	for idx, sns := range serials {
		for _, sn := range sns {
			s := e.scan(creator, id, ids[idx], sn)
			require.Equal(e.t, 201, s.status(), "scan %s: %s", sn, s.rec.Body.String())
		}
	}
	sub := e.post(creator, lpbPath(id, "/submit"), nil)
	require.Equal(e.t, 200, sub.status(), sub.rec.Body.String())
	e.clk.Advance(time.Second) // pengajuan berikutnya lebih baru (jam uji tidak berjalan sendiri)
	return id, ids
}

func (e *env) balance(product, rack int64) (onHand int, found bool) {
	e.t.Helper()
	rows, err := e.pool.Query(testCtx(), `SELECT qty_on_hand FROM stock_balances WHERE product_id = $1 AND rack_id = $2`, product, rack)
	require.NoError(e.t, err)
	defer rows.Close()
	if rows.Next() {
		require.NoError(e.t, rows.Scan(&onHand))
		return onHand, true
	}
	return 0, false
}

// ================================================================ RBAC & syarat

func TestApprove_RBACAndIdempotencyKeyRequired(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1)), nil)

	for name, tok := range map[string]string{"staff": f.staffA, "management": f.mgmt, "bizadmin": f.biz, "driver": f.driver, "technician": f.tech} {
		assert.Equal(t, 403, e.approve(tok, id, nextKey()).status(), "%s approve", name)
		assert.Equal(t, 403, e.reject(tok, id, "alasan").status(), "%s reject", name)
		assert.Equal(t, 403, e.get(tok, "/api/v1/lpb/approvals").status(), "%s antrean", name)
	}
	assert.Equal(t, 401, e.call("POST", lpbPath(id, "/approve"), nil, withHeader("Idempotency-Key", nextKey())).status())

	noKey := e.approve(f.headA, id, "")
	assert.Equal(t, 400, noKey.status())
	assert.Equal(t, "IDEMPOTENCY_KEY_REQUIRED", noKey.code())
	assert.Equal(t, 400, e.approve(f.headA, id, "pendek").status(), "kunci terlalu pendek")
	assert.Equal(t, 400, e.approve(f.headA, id, "kunci dengan spasi!!").status())
	_, found := e.balance(f.p2, f.a1)
	assert.False(t, found, "percobaan yang ditolak tidak mengubah stok")
	assert.Equal(t, "PENDING_APPROVAL", e.get(f.headA, lpbPath(id, "")).str("status"))
}

// ================================================================ approve: efek stok

func TestApprove_Purchase_UpdatesStockSerialsLedgerAndAudit(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	// toner sudah 3 di A-01 lewat stok awal yang disetujui (saldo selalu punya ledger).
	opening, _ := e.pending(f.staffA, map[string]any{"sub_type": "INITIAL_STOCK", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(f.p2, f.a1, 3)}}, nil)
	require.Equal(t, 200, e.approve(f.headA, opening, nextKey()).status())

	id, _ := e.pending(f.staffA, map[string]any{
		"sub_type": "PURCHASE", "warehouse_id": f.whA, "supplier_id": f.supplier, "doc_date": "2026-11-01",
		"items": []map[string]any{
			lpbItem(f.p1, f.a1, 2, map[string]any{"unit_cost": "1500000"}),
			lpbItem(f.p1, f.a2, 1, map[string]any{"unit_cost": "1500000"}),
			lpbItem(f.p2, f.a1, 5, map[string]any{"unit_cost": "250000.50"}),
		},
	}, map[int][]string{0: {"sn-b", "sn-a"}, 1: {"sn-broken"}})
	r := e.approve(f.headA, id, nextKey())
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	assert.Equal(t, "APPROVED", r.str("status"))
	assert.EqualValues(t, f.headAID, r.data()["approved_by"])
	assert.NotNil(t, r.data()["approved_at"])

	// saldo
	p1a1, _ := e.balance(f.p1, f.a1)
	p1a2, _ := e.balance(f.p1, f.a2)
	p2a1, _ := e.balance(f.p2, f.a1)
	assert.Equal(t, 2, p1a1)
	assert.Equal(t, 1, p1a2)
	assert.Equal(t, 8, p2a1, "3 + 5 (upsert pada saldo yang sudah ada)")

	// unit baru: asset tag berurutan per urutan serial number, tanggal & harga dari dokumen
	rows := serialRows(t, e)
	require.Len(t, rows, 3)
	tag := regexp.MustCompile(`^AST-\d{4}-\d{5}$`)
	assert.Regexp(t, tag, rows[0]["asset_tag"])
	byTag := map[string]map[string]any{}
	for _, row := range rows {
		byTag[row["serial_number"].(string)] = row
	}
	require.Contains(t, byTag, "SN-A")
	require.Contains(t, byTag, "SN-B")
	assert.Less(t, byTag["SN-A"]["asset_tag"], byTag["SN-B"]["asset_tag"], "SN-A diberi tag lebih dulu (urut serial number)")
	a := byTag["SN-A"]
	assert.Equal(t, "AVAILABLE", a["status"])
	assert.Equal(t, "GOOD", a["condition"])
	assert.Equal(t, "2026-11-01", a["purchase_date"])
	assert.Equal(t, "1500000.00", a["purchase_price"])
	assert.EqualValues(t, f.supplier, a["supplier_id"])
	assert.EqualValues(t, f.whA, a["current_warehouse_id"])
	assert.EqualValues(t, f.a1, a["current_rack_id"])

	// riwayat unit: satu per unit, oleh approver, merujuk LPB
	assert.Equal(t, 3, e.count(`SELECT count(*) FROM product_serial_histories WHERE event_type = 'LPB_APPROVED' AND from_status IS NULL
	   AND to_status = 'AVAILABLE' AND user_id = $1 AND reference_type = 'LPB' AND reference_id = $2`, f.headAID, id))

	// ledger: satu baris per item, berantai, membawa serial & biaya
	assert.Equal(t, 3, e.count(`SELECT count(*) FROM stock_ledgers WHERE lpb_id = $1 AND direction = 'IN' AND movement_type = 'PURCHASE' AND created_by = $2`, id, f.headAID))
	var before, after, qty int
	var serialIDs []int64
	var cost *string
	require.NoError(t, e.pool.QueryRow(testCtx(), `
		SELECT balance_before, balance_after, qty, serial_ids, unit_cost::text FROM stock_ledgers WHERE lpb_id = $1 AND product_id = $2 AND rack_id = $3`,
		id, f.p2, f.a1).Scan(&before, &after, &qty, &serialIDs, &cost))
	assert.Equal(t, []int{3, 8, 5}, []int{before, after, qty})
	assert.Empty(t, serialIDs, "non-SN tanpa serial_ids")
	require.NotNil(t, cost)
	assert.Equal(t, "250000.50", *cost)
	require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT serial_ids FROM stock_ledgers WHERE lpb_id = $1 AND product_id = $2 AND rack_id = $3`, id, f.p1, f.a1).Scan(&serialIDs))
	assert.Len(t, serialIDs, 2)

	// baris scan terhubung ke unit; token verifikasi acak
	assert.Equal(t, 3, e.count(`SELECT count(*) FROM lpb_item_serials WHERE product_serial_id IS NOT NULL`))
	var token string
	require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT verify_token FROM lpb_documents WHERE id = $1`, id).Scan(&token))
	assert.Regexp(t, `^[A-Za-z0-9_-]{43}$`, token)

	// detail: sebelum → sesudah per item, serial dengan asset tag, timeline dengan approver
	d := e.get(f.mgmt, lpbPath(id, ""))
	detail := itemsOf(d)
	toner := row(detail, "sku", "TN-1")
	assert.EqualValues(t, 3, toner["balance_before"])
	assert.EqualValues(t, 8, toner["balance_after"])
	printerA1 := detail[0]
	assert.NotNil(t, printerA1["serials"].([]any)[0].(map[string]any)["asset_tag"])
	assert.Equal(t, []string{"LPB_CREATE", "LPB_SUBMIT", "LPB_APPROVE"}, timelineActions(d))
	var approver int64
	require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT user_id FROM audit_logs WHERE action = 'LPB_APPROVE' AND entity_id = $1`, id).Scan(&approver))
	assert.Equal(t, f.headAID, approver)

	// inventory API (T1.4) melihat hasilnya
	bal := e.get(f.headA, fmt.Sprintf("/api/v1/inventory/balances?product_id=%d&sort=rack", f.p1))
	assert.EqualValues(t, 2, row(bal.list(), "rack_code", "A-01")["qty_on_hand"])
	scan := e.get(f.headA, "/api/v1/inventory/serials/scan/"+byTag["SN-A"]["asset_tag"].(string))
	require.Equal(t, 200, scan.status())
	assert.Equal(t, "SN-A", scan.str("serial_number"))
	hist := scan.data()["history"].([]any)
	require.Len(t, hist, 1)
	assert.Equal(t, "LPB_APPROVED", hist[0].(map[string]any)["event_type"])

	pgtest.AssertInvariants(t, e.pool)
}

func serialRows(t *testing.T, e *env) []map[string]any {
	rows, err := e.pool.Query(testCtx(), `
		SELECT asset_tag, serial_number, status, condition, purchase_date::text, purchase_price::text,
		       supplier_id, current_warehouse_id, current_rack_id FROM product_serials ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var tag, sn, status, cond string
		var date, price *string
		var sup, wh, rack *int64
		require.NoError(t, rows.Scan(&tag, &sn, &status, &cond, &date, &price, &sup, &wh, &rack))
		m := map[string]any{"asset_tag": tag, "serial_number": sn, "status": status, "condition": cond}
		if date != nil {
			m["purchase_date"] = *date
		}
		if price != nil {
			m["purchase_price"] = *price
		}
		if sup != nil {
			m["supplier_id"] = *sup
		}
		if wh != nil {
			m["current_warehouse_id"] = *wh
		}
		if rack != nil {
			m["current_rack_id"] = *rack
		}
		out = append(out, m)
	}
	return out
}

func TestApprove_InitialStock_AndConditions(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)

	r0 := e.mustLPB(f.staffA, map[string]any{"sub_type": "INITIAL_STOCK", "warehouse_id": f.whA,
		"items": []map[string]any{lpbItem(f.p1, f.a1, 2), lpbItem(f.p1, f.a2, 1), lpbItem(f.p2, f.a1, 4)}})
	id, items := idOf(r0), []int64{itemID(r0, 0), itemID(r0, 1), itemID(r0, 2)}
	// kondisi unit ditentukan saat scan: MINOR_DEFECT tetap AVAILABLE, BROKEN (karantina) → BROKEN.
	require.Equal(t, 201, e.scan(f.staffA, id, items[0], "SN-OK").status())
	require.Equal(t, 201, e.scan(f.staffA, id, items[0], "SN-MINOR", map[string]any{"condition": "MINOR_DEFECT"}).status())
	require.Equal(t, 201, e.scan(f.staffA, id, items[1], "SN-BRK", map[string]any{"condition": "BROKEN"}).status())
	require.Equal(t, 200, e.post(f.staffA, lpbPath(id, "/submit"), nil).status())

	r := e.approve(f.headA, id, nextKey())
	require.Equal(t, 200, r.status(), r.rec.Body.String())

	status := map[string]string{}
	for _, row := range serialRows(t, e) {
		status[row["serial_number"].(string)] = row["status"].(string) + "/" + row["condition"].(string)
	}
	assert.Equal(t, map[string]string{"SN-OK": "AVAILABLE/GOOD", "SN-MINOR": "AVAILABLE/MINOR_DEFECT", "SN-BRK": "BROKEN/BROKEN"}, status)
	assert.Equal(t, 3, e.count(`SELECT count(*) FROM stock_ledgers WHERE movement_type = 'INITIAL_STOCK'`))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM product_serials WHERE supplier_id IS NOT NULL`), "stok awal tanpa supplier")
	pgtest.AssertInvariants(t, e.pool)
}

// ================================================================ aturan approver

func TestApprove_SelfApprovalRules(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id, _ := e.pending(f.headA, f.purchase(lpbItem(f.p2, f.a1, 2)), nil) // kepala gudang membuat sendiri

	self := e.approve(f.headA, id, nextKey())
	assert.Equal(t, 403, self.status())
	assert.Equal(t, "SELF_APPROVE_FORBIDDEN", self.code())
	_, found := e.balance(f.p2, f.a1)
	assert.False(t, found)

	// Kepala gudang lain boleh.
	_, otherHead := e.userToken(domain.RoleHeadWarehouse, &f.whA)
	ok := e.approve(otherHead, id, nextKey())
	assert.Equal(t, 200, ok.status(), ok.rec.Body.String())

	// Pengaturan lpb.allow_self_approve = true mengizinkan.
	e.setSetting("lpb.allow_self_approve", "true")
	id2, _ := e.pending(f.headA, f.purchase(lpbItem(f.p2, f.a1, 1)), nil)
	assert.Equal(t, 200, e.approve(f.headA, id2, nextKey()).status())
	e.setSetting("lpb.allow_self_approve", "false")
	id3, _ := e.pending(f.headA, f.purchase(lpbItem(f.p2, f.a1, 1)), nil)
	assert.Equal(t, 403, e.approve(f.headA, id3, nextKey()).status())
	pgtest.AssertInvariants(t, e.pool)
}

func TestApprove_StateScopeAndUnsupported(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	draft := idOf(e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1))))
	id, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1)), nil)

	notPending := e.approve(f.headA, draft, nextKey())
	assert.Equal(t, 409, notPending.status())
	assert.Equal(t, "LPB_NOT_PENDING", notPending.code())
	nf := e.approve(f.headA, 999999, nextKey())
	assert.Equal(t, 404, nf.status())
	assert.Equal(t, "LPB_NOT_FOUND", nf.code())

	// Kepala gudang WH-B tidak boleh menyetujui dokumen WH-A.
	_, headB := e.userToken(domain.RoleHeadWarehouse, &f.whB)
	wrong := e.approve(headB, id, nextKey())
	assert.Equal(t, 403, wrong.status())
	assert.Equal(t, "FORBIDDEN_WAREHOUSE", wrong.code())
	assert.Equal(t, 200, e.approve(f.headG, id, nextKey()).status(), "kepala gudang tanpa batas boleh")

	again := e.approve(f.headA, id, nextKey())
	assert.Equal(t, 409, again.status(), "sudah APPROVED")
	assert.Equal(t, "LPB_NOT_PENDING", again.code())
}

// ================================================================ atomik & validasi ulang

func TestApprove_FailureRollsBackEverything(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	// Item 1 (produk non-SN, id lebih besar diproses belakangan) dan item 2 (SN): kegagalan di tengah.
	id, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 1), lpbItem(f.p2, f.a1, 5)), map[int][]string{0: {"SN-DUP"}})
	e.addSerial(f.p1, "AST-2611-00099", "SN-DUP", "AVAILABLE", &f.a1, nil) // didaftarkan pihak lain setelah submit
	seqBefore := e.count(`SELECT COALESCE(SUM(last_number), 0) FROM document_sequences`)

	r := e.approve(f.headA, id, nextKey())
	assert.Equal(t, 409, r.status())
	assert.Equal(t, "SERIAL_EXISTS", r.code())

	assert.Equal(t, 0, e.count(`SELECT count(*) FROM stock_balances`), "tidak ada saldo yang berubah")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM stock_ledgers`))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM product_serial_histories`))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM product_serials`), "hanya unit pihak lain")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM lpb_item_serials WHERE product_serial_id IS NOT NULL`))
	assert.Equal(t, seqBefore, e.count(`SELECT COALESCE(SUM(last_number), 0) FROM document_sequences`), "nomor asset tag ikut dibatalkan (tanpa celah)")
	assert.Equal(t, "PENDING_APPROVAL", e.get(f.headA, lpbPath(id, "")).str("status"))
	assert.Equal(t, 0, e.auditCount("LPB_APPROVE", id), "tidak ada audit approve")
	pgtest.AssertInvariants(t, e.pool)
}

func TestApprove_RevalidatesUnderLock(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)

	// rak dikunci opname setelah diajukan
	id1, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1)), nil)
	e.lockRack(f.a1, f.whA)
	locked := e.approve(f.headA, id1, nextKey())
	assert.Equal(t, 409, locked.status())
	assert.Equal(t, "RACK_LOCKED_BY_OPNAME", locked.code())
	e.exec(`UPDATE racks SET locked_by_opname_id = NULL WHERE id = $1`, f.a1)

	// jumlah scan berubah (data dimanipulasi) → tolak
	id2, items := e.pending(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 2)), map[int][]string{0: {"SN-1", "SN-2"}})
	e.exec(`DELETE FROM lpb_item_serials WHERE lpb_item_id = $1 AND serial_number = 'SN-2'`, items[0])
	mismatch := e.approve(f.headA, id2, nextKey())
	assert.Equal(t, 422, mismatch.status())
	assert.Equal(t, "SERIAL_COUNT_MISMATCH", mismatch.code())

	// produk dinonaktifkan; gudang dinonaktifkan
	id3, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p3, f.a1, 1)), map[int][]string{0: {"SN-9"}})
	e.exec(`UPDATE products SET is_active = FALSE WHERE id = $1`, f.p3)
	assert.Equal(t, 422, e.approve(f.headA, id3, nextKey()).status())
	e.exec(`UPDATE products SET is_active = TRUE WHERE id = $1`, f.p3)
	e.exec(`UPDATE warehouses SET is_active = FALSE WHERE id = $1`, f.whA)
	assert.Equal(t, 422, e.approve(f.headG, id3, nextKey()).status())

	assert.Equal(t, 0, e.count(`SELECT count(*) FROM stock_balances`))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM stock_ledgers`))
	pgtest.AssertInvariants(t, e.pool)
}

// ================================================================ idempotency

func TestApprove_Idempotency_ReplayReturnsSameResponseWithoutRerunning(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 5)), nil)
	key := nextKey()

	first := e.approve(f.headA, id, key)
	require.Equal(t, 200, first.status(), first.rec.Body.String())
	assert.Empty(t, first.rec.Header().Get("Idempotency-Replayed"))

	second := e.approve(f.headA, id, key) // klik ganda / retry jaringan
	assert.Equal(t, 200, second.status(), "respons yang sama, bukan 409 NOT_PENDING")
	assert.Equal(t, "true", second.rec.Header().Get("Idempotency-Replayed"))
	assert.Equal(t, first.str("lpb_number"), second.str("lpb_number"))
	assert.Equal(t, "APPROVED", second.str("status"))

	onHand, _ := e.balance(f.p2, f.a1)
	assert.Equal(t, 5, onHand, "stok bertambah tepat sekali")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM stock_ledgers`))
	assert.Equal(t, 1, e.auditCount("LPB_APPROVE", id))

	// Kunci yang sama untuk LPB lain → ditolak, bukan diputar ulang.
	id2, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1)), nil)
	reused := e.approve(f.headA, id2, key)
	assert.Equal(t, 422, reused.status())
	assert.Equal(t, "IDEMPOTENCY_KEY_REUSED", reused.code())
	onHand, _ = e.balance(f.p2, f.a1)
	assert.Equal(t, 5, onHand)

	// Kunci terikat pada pengguna: pengguna lain dapat memakai string yang sama.
	assert.Equal(t, 200, e.approve(f.headG, id2, key).status())

	// Respons kegagalan 4xx juga disimpan: ulangi permintaan yang gagal → hasil gagal yang sama.
	failKey := nextKey()
	notPending := e.approve(f.headA, id, failKey)
	assert.Equal(t, 409, notPending.status())
	replayFail := e.approve(f.headA, id, failKey)
	assert.Equal(t, 409, replayFail.status())
	assert.Equal(t, "true", replayFail.rec.Header().Get("Idempotency-Replayed"))
	pgtest.AssertInvariants(t, e.pool)
}

func TestApprove_Idempotency_ConcurrentSameKey_ExecutesOnce(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 7)), nil)
	key := nextKey()

	const n = 8
	var wg sync.WaitGroup
	codes := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := e.approve(f.headA, id, key)
			codes <- fmt.Sprintf("%d", r.status())
		}()
	}
	wg.Wait()
	close(codes)
	ok := 0
	for c := range codes {
		switch c {
		case "200", "409": // 200 = asli atau ulang; 409 = sedang diproses
			if c == "200" {
				ok++
			}
		default:
			t.Errorf("status tak terduga %s", c)
		}
	}
	assert.GreaterOrEqual(t, ok, 1)
	onHand, _ := e.balance(f.p2, f.a1)
	assert.Equal(t, 7, onHand, "stok bertambah tepat sekali walau %d permintaan paralel", n)
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM stock_ledgers`))
	assert.Equal(t, 1, e.auditCount("LPB_APPROVE", id))
	pgtest.AssertInvariants(t, e.pool)
}

// ================================================================ konkurensi

func TestApprove_ConcurrentDifferentKeys_OnlyOneWins(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 2), lpbItem(f.p2, f.a1, 3)), map[int][]string{0: {"SN-1", "SN-2"}})

	const n = 6
	var wg sync.WaitGroup
	results := make(chan *reply, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- e.approve(f.headA, id, nextKey())
		}()
	}
	wg.Wait()
	close(results)
	ok, notPending := 0, 0
	for r := range results {
		switch {
		case r.status() == 200:
			ok++
		case r.status() == 409 && r.code() == "LPB_NOT_PENDING":
			notPending++
		default:
			t.Errorf("hasil tak terduga: %d %s", r.status(), r.rec.Body.String())
		}
	}
	assert.Equal(t, 1, ok, "tepat satu approve yang berhasil")
	assert.Equal(t, n-1, notPending)
	onHand, _ := e.balance(f.p1, f.a1)
	assert.Equal(t, 2, onHand)
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM product_serials`), "unit tidak terdaftar ganda")
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM stock_ledgers`))
	pgtest.AssertInvariants(t, e.pool)
}

func TestApprove_ConcurrentDifferentLPBsSameBalance_AllApplyWithoutLoss(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)

	const n = 8
	ids := make([]int64, n)
	for i := range ids {
		ids[i], _ = e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, i+1)), nil) // 1+2+...+8 = 36
	}
	var wg sync.WaitGroup
	statuses := make(chan int, n)
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- e.approve(f.headA, id, nextKey()).status()
		}()
	}
	wg.Wait()
	close(statuses)
	for s := range statuses {
		assert.Equal(t, 200, s)
	}

	onHand, _ := e.balance(f.p2, f.a1)
	assert.Equal(t, 36, onHand, "tidak ada pembaruan yang hilang")
	// Rantai ledger tanpa celah: setiap 'before' sama dengan 'after' baris sebelumnya.
	rows, err := e.pool.Query(testCtx(), `SELECT balance_before, balance_after, qty FROM stock_ledgers WHERE product_id = $1 AND rack_id = $2 ORDER BY id`, f.p2, f.a1)
	require.NoError(t, err)
	defer rows.Close()
	var afters []int
	for rows.Next() {
		var b, a, q int
		require.NoError(t, rows.Scan(&b, &a, &q))
		assert.Equal(t, b+q, a)
		afters = append(afters, a)
	}
	sort.Ints(afters)
	assert.Equal(t, 36, afters[len(afters)-1])
	assert.Len(t, afters, n)
	pgtest.AssertInvariants(t, e.pool)
}

// ================================================================ reject

func TestReject_Flow_ReviseAndApproveAfterwards(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 5)), nil)

	assert.Equal(t, 400, e.post(f.headA, lpbPath(id, "/reject"), map[string]any{}).status(), "alasan wajib")
	assert.Equal(t, 400, e.reject(f.headA, id, "x").status(), "alasan terlalu pendek")
	short := e.reject(f.headA, id, "  ab  ")
	assert.Contains(t, []int{400, 422}, short.status())

	_, headB := e.userToken(domain.RoleHeadWarehouse, &f.whB)
	assert.Equal(t, 403, e.reject(headB, id, "qty salah").status(), "gudang lain")

	r := e.reject(f.headA, id, "  Qty tidak sesuai surat jalan  ")
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	assert.Equal(t, "REJECTED", r.str("status"))
	assert.Equal(t, "Qty tidak sesuai surat jalan", r.str("rejection_reason"))
	assert.EqualValues(t, f.headAID, r.data()["rejected_by"])
	assert.NotNil(t, r.data()["rejected_at"])
	assert.Equal(t, []string{"LPB_CREATE", "LPB_SUBMIT", "LPB_REJECT"}, timelineActions(r))
	var reasonInAudit string
	require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT after_data->>'reason' FROM audit_logs WHERE action = 'LPB_REJECT' AND entity_id = $1`, id).Scan(&reasonInAudit))
	assert.Equal(t, "Qty tidak sesuai surat jalan", reasonInAudit)

	again := e.reject(f.headA, id, "lagi")
	assert.Equal(t, 409, again.status())
	assert.Equal(t, "LPB_NOT_PENDING", again.code())
	assert.Equal(t, 409, e.approve(f.headA, id, nextKey()).status(), "REJECTED tidak dapat disetujui")
	_, found := e.balance(f.p2, f.a1)
	assert.False(t, found, "penolakan tidak mengubah stok")

	// Pembuat merevisi → ubah → ajukan ulang → disetujui.
	require.Equal(t, 200, e.post(f.staffA, lpbPath(id, "/revise"), nil).status())
	require.Equal(t, 200, e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"items": []map[string]any{lpbItem(f.p2, f.a1, 4)}}).status())
	require.Equal(t, 200, e.post(f.staffA, lpbPath(id, "/submit"), nil).status())
	ok := e.approve(f.headA, id, nextKey())
	require.Equal(t, 200, ok.status(), ok.rec.Body.String())
	onHand, _ := e.balance(f.p2, f.a1)
	assert.Equal(t, 4, onHand)
	assert.Equal(t, []string{"LPB_CREATE", "LPB_SUBMIT", "LPB_REJECT", "LPB_REVISE", "LPB_UPDATE", "LPB_SUBMIT", "LPB_APPROVE"}, timelineActions(ok))
	pgtest.AssertInvariants(t, e.pool)
}

// ================================================================ antrean persetujuan

func TestApprovals_QueueOldestFirstScopedAndFiltered(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	first, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1)), nil)
	second, _ := e.pending(f.staff2A, map[string]any{"sub_type": "INITIAL_STOCK", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(f.p2, f.a1, 2)}}, nil)
	third, _ := e.pending(f.staffB, map[string]any{"sub_type": "INITIAL_STOCK", "warehouse_id": f.whB, "items": []map[string]any{lpbItem(f.p2, f.b1, 3)}}, nil)
	_ = e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 9))) // masih DRAFT: tidak masuk antrean
	done, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1)), nil)
	require.Equal(t, 200, e.approve(f.headA, done, nextKey()).status()) // sudah disetujui: tidak masuk antrean

	ids := func(r *reply) []int64 {
		out := []int64{}
		for _, it := range r.list() {
			out = append(out, int64(it["id"].(float64)))
		}
		return out
	}
	q := e.get(f.headA, "/api/v1/lpb/approvals")
	require.Equal(t, 200, q.status(), q.rec.Body.String())
	assert.Equal(t, []int64{first, second}, ids(q), "hanya PENDING di WH-A, tertua dulu")
	assert.EqualValues(t, 2, q.meta()["total"])
	assert.EqualValues(t, 1, q.list()[0]["item_count"])
	assert.NotNil(t, q.list()[0]["submitted_at"])

	assert.Equal(t, []int64{third}, ids(e.get(f.admin, fmt.Sprintf("/api/v1/lpb/approvals?warehouse_id=%d", f.whB))))
	assert.Len(t, ids(e.get(f.admin, "/api/v1/lpb/approvals")), 3)
	assert.Equal(t, []int64{second}, ids(e.get(f.headA, "/api/v1/lpb/approvals?sub_type=INITIAL_STOCK")))
	assert.Equal(t, []int64{second, first}, ids(e.get(f.headA, "/api/v1/lpb/approvals?sort=-submitted_at")))
	denied := e.get(f.headA, fmt.Sprintf("/api/v1/lpb/approvals?warehouse_id=%d", f.whB))
	assert.Equal(t, 403, denied.status())
	assert.Equal(t, 400, e.get(f.headA, "/api/v1/lpb/approvals?sort=notes").status())

	// rute statis tidak bentrok dengan /lpb/:id
	assert.Equal(t, 200, e.get(f.headA, lpbPath(first, "")).status())
}

func TestApprove_DetailShowsCurrentStockBeforeApproval(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	e.exec(`INSERT INTO stock_balances (product_id, rack_id, qty_on_hand) VALUES ($1, $2, 3)`, f.p2, f.a1)
	id, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 5)), nil)

	d := e.get(f.headA, lpbPath(id, ""))
	it := itemsOf(d)[0]
	assert.EqualValues(t, 3, it["current_qty"], "saldo saat ini untuk tampilan sebelum → sesudah")
	assert.Nil(t, it["balance_before"], "belum ada ledger sebelum approve")
}

// Dua dokumen menyentuh saldo yang sama dengan urutan item berlawanan: urutan lock (produk, rak)
// yang tetap mencegah deadlock — semuanya harus berhasil, tanpa 500.
func TestApprove_ConcurrentOppositeItemOrder_NoDeadlock(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	toner := e.newCategory(f.admin, "Tinta", "CONSUMABLE")
	p4 := idOf(e.newProduct(f.admin, toner, "INK-1", nil))

	const pairs = 6
	var ids []int64
	for i := 0; i < pairs; i++ {
		a, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1), lpbItem(p4, f.a1, 1)), nil)
		b, _ := e.pending(f.staffA, f.purchase(lpbItem(p4, f.a1, 2), lpbItem(f.p2, f.a1, 2)), nil)
		ids = append(ids, a, b)
	}
	var wg sync.WaitGroup
	statuses := make(chan int, len(ids))
	start := make(chan struct{})
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			statuses <- e.approve(f.headA, id, nextKey()).status()
		}()
	}
	close(start)
	wg.Wait()
	close(statuses)
	for s := range statuses {
		assert.Equal(t, 200, s, "tidak boleh deadlock/500")
	}
	p2, _ := e.balance(f.p2, f.a1)
	p4q, _ := e.balance(p4, f.a1)
	assert.Equal(t, pairs*3, p2)
	assert.Equal(t, pairs*3, p4q)
	pgtest.AssertInvariants(t, e.pool)
}
