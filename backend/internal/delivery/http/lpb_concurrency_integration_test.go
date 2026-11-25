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

// ================================================================ T1.9: Konkurensi & Invariant

// Skenario wajib Bagian 11: banyak approve LPB OUT paralel pada stok yang sama.
// Stok awal = 5, 6 LPB keluar masing-masing meminta 3 (semuanya lolos submit saat stok 5).
// Saat disetujui serentak: tepat 1 berhasil, 5 ditolak 409 INSUFFICIENT_STOCK, saldo akhir 2 (tidak pernah negatif).
func TestConcurrency_ParallelOutboundApprove_SameStock_NeverNegative(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	e.receive(f, f.whA, f.a1, f.p2, 5)

	const n = 6
	ids := make([]int64, n)
	for i := range ids {
		ids[i], _ = e.pending(f.staffA, disposal(f.whA, lpbItem(f.p2, f.a1, 3)), nil)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]*reply, n)
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = e.approve(f.headA, id, nextKey())
		}()
	}
	close(start)
	wg.Wait()

	ok, insufficient := 0, 0
	for _, r := range results {
		switch {
		case r.status() == 200:
			ok++
		case r.status() == 409 && r.code() == "INSUFFICIENT_STOCK":
			insufficient++
		default:
			t.Errorf("hasil tak terduga: %d %s", r.status(), r.rec.Body.String())
		}
	}
	assert.Equal(t, 1, ok, "tepat 1 LPB keluar yang berhasil")
	assert.Equal(t, n-1, insufficient, "sisanya ditolak karena stok tidak mencukupi")
	assert.Equal(t, 2, e.bal(f.p2, f.a1), "saldo tidak pernah negatif (5 - 3 = 2)")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM stock_ledgers WHERE direction = 'OUT' AND product_id = $1 AND rack_id = $2`, f.p2, f.a1))
	pgtest.AssertInvariants(t, e.pool)
}

// Transfer keluar dan penghapusan bersaing pada stok non-SN yang sama (stok 10, 6 dokumen @4).
// Tepat 2 berhasil (8 keluar, sisa 2), 4 gagal 409 INSUFFICIENT_STOCK, tanpa dokumen pasangan TRANSFER_IN yatim.
func TestConcurrency_ParallelTransferAndDisposal_SameStock_ExactFit(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	a3 := f.newRackIn(e, f.whA, "A-03", map[string]any{"zone": "PRINTER"})
	e.receive(f, f.whA, f.a1, f.p2, 10)

	var ids []int64
	for i := 0; i < 3; i++ {
		tr, _ := e.pending(f.staffA, transfer(f.whA, toRack(f.p2, f.a1, 4, a3)), nil)
		dp, _ := e.pending(f.staffA, disposal(f.whA, lpbItem(f.p2, f.a1, 4)), nil)
		ids = append(ids, tr, dp)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]*reply, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = e.approve(f.headA, id, nextKey())
		}()
	}
	close(start)
	wg.Wait()

	ok, insufficient := 0, 0
	for _, r := range results {
		switch {
		case r.status() == 200:
			ok++
		case r.status() == 409 && r.code() == "INSUFFICIENT_STOCK":
			insufficient++
		default:
			t.Errorf("hasil tak terduga: %d %s", r.status(), r.rec.Body.String())
		}
	}
	assert.Equal(t, 2, ok, "stok 10 hanya cukup untuk 2 dokumen @4")
	assert.Equal(t, len(ids)-2, insufficient)
	assert.Equal(t, 2, e.bal(f.p2, f.a1))
	approvedTransfers := e.count(`SELECT count(*) FROM lpb_documents WHERE sub_type = 'TRANSFER_OUT' AND status = 'APPROVED'`)
	approvedDisposals := e.count(`SELECT count(*) FROM lpb_documents WHERE sub_type = 'DISPOSAL' AND status = 'APPROVED'`)
	assert.Equal(t, 2, approvedTransfers+approvedDisposals)
	assert.Equal(t, approvedTransfers*4, e.bal(f.p2, a3))
	assert.Equal(t, approvedTransfers, e.count(`SELECT count(*) FROM lpb_documents WHERE sub_type = 'TRANSFER_IN' AND status = 'APPROVED'`),
		"tidak ada pasangan TRANSFER_IN tersisa dari transaksi yang gagal")
	pgtest.AssertInvariants(t, e.pool)
}

// Campuran IN (+3 x 5 = +15) dan OUT (-2 x 5 = -10) disetujui serentak pada saldo awal 10.
// Semuanya harus berhasil, saldo akhir 15, dan rantai saldo ledger (I5b) tetap utuh tanpa celah.
func TestConcurrency_ParallelMixedInboundAndOutbound_SameBalance_ConsistentChain(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	e.receive(f, f.whA, f.a1, f.p2, 10)

	const perSide = 5
	var ids []int64
	for i := 0; i < perSide; i++ {
		inID, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 3)), nil)
		outID, _ := e.pending(f.staffA, disposal(f.whA, lpbItem(f.p2, f.a1, 2)), nil)
		ids = append(ids, inID, outID)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]*reply, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = e.approve(f.headA, id, nextKey())
		}()
	}
	close(start)
	wg.Wait()

	for i, r := range results {
		require.Equal(t, 200, r.status(), "dokumen %d: %s", i, r.rec.Body.String())
	}
	assert.Equal(t, 10+perSide*3-perSide*2, e.bal(f.p2, f.a1))
	assert.Equal(t, 1+len(ids), e.count(`SELECT count(*) FROM stock_ledgers WHERE product_id = $1 AND rack_id = $2`, f.p2, f.a1))
	pgtest.AssertInvariants(t, e.pool)
}

// Balapan approve vs reject pada LPB yang sama: tepat satu keputusan menang, sisanya 409 LPB_NOT_PENDING.
func TestConcurrency_ParallelApproveAndReject_SameLPB_OnlyOneWins(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 4)), nil)

	const perAction = 4
	var wg sync.WaitGroup
	start := make(chan struct{})
	approveRes := make([]*reply, perAction)
	rejectRes := make([]*reply, perAction)

	for i := 0; i < perAction; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			approveRes[i] = e.approve(f.headA, id, nextKey())
		}()
		go func() {
			defer wg.Done()
			<-start
			rejectRes[i] = e.reject(f.headA, id, "tolak paralel")
		}()
	}
	close(start)
	wg.Wait()

	approveOK, rejectOK, notPending := 0, 0, 0
	for _, r := range approveRes {
		switch {
		case r.status() == 200:
			approveOK++
		case r.status() == 409 && r.code() == "LPB_NOT_PENDING":
			notPending++
		default:
			t.Errorf("approve tak terduga: %d %s", r.status(), r.rec.Body.String())
		}
	}
	for _, r := range rejectRes {
		switch {
		case r.status() == 200:
			rejectOK++
		case r.status() == 409 && r.code() == "LPB_NOT_PENDING":
			notPending++
		default:
			t.Errorf("reject tak terduga: %d %s", r.status(), r.rec.Body.String())
		}
	}

	assert.Equal(t, 1, approveOK+rejectOK, "tepat satu keputusan yang menang")
	assert.Equal(t, 2*perAction-1, notPending)
	if approveOK == 1 {
		assert.Equal(t, "APPROVED", e.get(f.headA, lpbPath(id, "")).str("status"))
		assert.Equal(t, 4, e.bal(f.p2, f.a1))
	} else {
		assert.Equal(t, "REJECTED", e.get(f.headA, lpbPath(id, "")).str("status"))
		assert.Equal(t, 0, e.bal(f.p2, f.a1))
	}
	pgtest.AssertInvariants(t, e.pool)
}

// Approve paralel pada LPB pasangan TRANSFER_IN antar gudang dengan kunci idempotensi berbeda:
// tepat satu berhasil, unit & saldo tujuan hanya bertambah sekali.
func TestConcurrency_ParallelTransferInApprove_OnlyOneWins(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	_, headB := e.userToken(domain.RoleHeadWarehouse, &f.whB)
	e.receive(f, f.whA, f.a1, f.p1, 2, "TR-SN-1", "TR-SN-2")
	e.receive(f, f.whA, f.a1, f.p2, 5)

	outID, _ := e.pending(f.staffA, transfer(f.whA, toRack(f.p1, f.a1, 2, f.b1), toRack(f.p2, f.a1, 5, f.b1)),
		map[int][]string{0: {"TR-SN-1", "TR-SN-2"}})
	outRes := e.approve(f.headA, outID, nextKey())
	require.Equal(t, 200, outRes.status(), outRes.rec.Body.String())
	inID := int64(outRes.data()["counterpart_id"].(float64))

	const n = 6
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]*reply, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = e.approve(headB, inID, nextKey())
		}()
	}
	close(start)
	wg.Wait()

	ok, notPending := 0, 0
	for _, r := range results {
		switch {
		case r.status() == 200:
			ok++
		case r.status() == 409 && r.code() == "LPB_NOT_PENDING":
			notPending++
		default:
			t.Errorf("hasil tak terduga: %d %s", r.status(), r.rec.Body.String())
		}
	}
	assert.Equal(t, 1, ok)
	assert.Equal(t, n-1, notPending)
	assert.Equal(t, 2, e.bal(f.p1, f.b1))
	assert.Equal(t, 5, e.bal(f.p2, f.b1))
	assert.Equal(t, "AVAILABLE", e.unit("TR-SN-1").status)
	assert.Equal(t, "AVAILABLE", e.unit("TR-SN-2").status)
	pgtest.AssertInvariants(t, e.pool)
}

// Dua LPB PURCHASE dengan SN sama di-approve serentak (mensimulasikan bentrokan unik pada tahap approve):
// tepat 1 berhasil, 1 ditolak 409 SERIAL_EXISTS dan rollback penuh (termasuk item non-SN di dokumen gagal).
func TestConcurrency_ParallelInboundApprove_DuplicateSerial_OnlyOneWins(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)

	id1, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 1), lpbItem(f.p2, f.a1, 3)), map[int][]string{0: {"SN-RACE"}})
	id2, items2 := e.pending(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 1), lpbItem(f.p2, f.a1, 3)), map[int][]string{0: {"SN-OTHER"}})
	// Paksa baris scan LPB kedua memakai SN yang sama agar keduanya beradu di tahap ApproveLPB.
	e.exec(`UPDATE lpb_item_serials SET serial_number = 'SN-RACE' WHERE lpb_item_id = $1`, items2[0])

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]*reply, 2)
	for i, id := range []int64{id1, id2} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = e.approve(f.headA, id, nextKey())
		}()
	}
	close(start)
	wg.Wait()

	ok, dup := 0, 0
	for _, r := range results {
		switch {
		case r.status() == 200:
			ok++
		case r.status() == 409 && r.code() == "SERIAL_EXISTS":
			dup++
		default:
			t.Errorf("hasil tak terduga: %d %s", r.status(), r.rec.Body.String())
		}
	}
	assert.Equal(t, 1, ok, "tepat 1 PURCHASE yang berhasil")
	assert.Equal(t, 1, dup, "satunya ditolak SERIAL_EXISTS")
	assert.Equal(t, 1, e.bal(f.p1, f.a1))
	assert.Equal(t, 3, e.bal(f.p2, f.a1), "item non-SN pada dokumen yang gagal ikut di-rollback")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM product_serials WHERE serial_number = 'SN-RACE'`))
	pgtest.AssertInvariants(t, e.pool)
}
