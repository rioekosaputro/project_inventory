//go:build integration

package http_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestLedger_StockCard_SerialHistory_And_ExcelExport(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	a3 := f.newRackIn(e, f.whA, "A-03", map[string]any{"zone": "PRINTER"})

	// Buat mutasi stok: terima 2 printer SN & 10 toner di A-01, lalu transfer 1 printer & 4 toner ke A-03.
	e.receive(f, f.whA, f.a1, f.p1, 2, "SN-LDG-1", "SN-LDG-2")
	e.receive(f, f.whA, f.a1, f.p2, 10)
	trID, _ := e.pending(f.staffA, transfer(f.whA, toRack(f.p1, f.a1, 1, a3), toRack(f.p2, f.a1, 4, a3)),
		map[int][]string{0: {"SN-LDG-1"}})
	require.Equal(t, 200, e.approve(f.headA, trID, nextKey()).status())

	// 1. RBAC untuk /ledgers dan /stock-card (ledger.read: MGMT, STAFF, HEAD, ADMIN; BIZ_ADMIN/TECH/DRIVER tidak).
	assert.Equal(t, 403, e.get(f.biz, "/api/v1/inventory/ledgers").status())
	assert.Equal(t, 403, e.get(f.tech, "/api/v1/inventory/ledgers").status())
	assert.Equal(t, 403, e.get(f.driver, "/api/v1/inventory/stock-card?product_id=1").status())

	// 2. GET /inventory/ledgers + filter & scoping.
	ledgers := e.get(f.staffA, fmt.Sprintf("/api/v1/inventory/ledgers?product_id=%d", f.p2))
	require.Equal(t, 200, ledgers.status(), ledgers.rec.Body.String())
	assert.Len(t, ledgers.list(), 3, "INITIAL_STOCK +10, TRANSFER_OUT -4, TRANSFER_IN +4")
	assert.Equal(t, 403, e.get(f.staffB, fmt.Sprintf("/api/v1/inventory/ledgers?warehouse_id=%d", f.whA)).status())
	assert.Empty(t, e.get(f.staffB, "/api/v1/inventory/ledgers").list(), "gudang B tidak melihat ledger gudang A")

	// 3. GET /inventory/ledgers/export → valid .xlsx
	xlResp := e.get(f.staffA, fmt.Sprintf("/api/v1/inventory/ledgers/export?product_id=%d", f.p2))
	require.Equal(t, 200, xlResp.status())
	assert.Contains(t, xlResp.rec.Header().Get("Content-Type"), "spreadsheetml.sheet")
	xl, err := excelize.OpenReader(bytes.NewReader(xlResp.rec.Body.Bytes()))
	require.NoError(t, err)
	defer func() { _ = xl.Close() }()
	rows, err := xl.GetRows("Buku Besar Stok")
	require.NoError(t, err)
	assert.Len(t, rows, 4, "1 header + 3 baris mutasi toner")

	// 4. GET /inventory/stock-card (per produk + rak A-01: masuk 10, keluar 4, saldo akhir 6).
	card := e.get(f.staffA, fmt.Sprintf("/api/v1/inventory/stock-card?product_id=%d&rack_id=%d", f.p2, f.a1))
	require.Equal(t, 200, card.status(), card.rec.Body.String())
	assert.EqualValues(t, 0, card.data()["opening_balance"])
	assert.EqualValues(t, 10, card.data()["total_in"])
	assert.EqualValues(t, 4, card.data()["total_out"])
	assert.EqualValues(t, 6, card.data()["closing_balance"])
	assert.Len(t, card.data()["movements"].([]any), 2)

	// Export Excel kartu stok (?format=xlsx).
	cardXL := e.get(f.staffA, fmt.Sprintf("/api/v1/inventory/stock-card?product_id=%d&rack_id=%d&format=xlsx", f.p2, f.a1))
	require.Equal(t, 200, cardXL.status())
	assert.Contains(t, cardXL.rec.Header().Get("Content-Type"), "spreadsheetml.sheet")

	// 5. GET /inventory/serials/:id/history (riwayat lengkap SN-LDG-1: masuk A-01 → berangkat → tiba A-03 = 3 histori).
	var serialID int64
	require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT id FROM product_serials WHERE serial_number = 'SN-LDG-1'`).Scan(&serialID))
	hist := e.get(f.staffA, fmt.Sprintf("/api/v1/inventory/serials/%d/history", serialID))
	require.Equal(t, 200, hist.status(), hist.rec.Body.String())
	assert.Equal(t, "SN-LDG-1", hist.str("serial_number"))
	assert.Len(t, hist.data()["history"].([]any), 3)
	assert.Equal(t, 403, e.get(f.staffB, fmt.Sprintf("/api/v1/inventory/serials/%d/history", serialID)).status())
	assert.Equal(t, 404, e.get(f.staffA, "/api/v1/inventory/serials/999999/history").status())
}
