//go:build integration

package http_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboard_WarehouseSummary(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	// Set min_stock_alert=10 untuk toner (f.p2) sehingga stok 3 dianggap menipis.
	e.exec(`UPDATE products SET min_stock_alert = 10 WHERE id = $1`, f.p2)
	e.receive(f, f.whA, f.a1, f.p1, 2, "SN-DB-1", "SN-DB-2")
	e.receive(f, f.whA, f.a1, f.p2, 3)

	// 1 LPB menunggu persetujuan + 1 opname berjalan di WH-A.
	_, _ = e.pending(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 5)), nil)
	require.Equal(t, 201, e.post(f.staffA, "/api/v1/inventory/opnames", map[string]any{
		"warehouse_id": f.whA, "rack_ids": []int64{f.a2},
	}).status())

	// RBAC: driver/technician tidak punya dashboard.read.
	assert.Equal(t, 403, e.get(f.driver, "/api/v1/dashboard/summary").status())
	assert.Equal(t, 403, e.get(f.tech, "/api/v1/dashboard/summary").status())
	assert.Equal(t, 403, e.get(f.staffB, fmt.Sprintf("/api/v1/dashboard/summary?warehouse_id=%d", f.whA)).status())

	r := e.get(f.headA, "/api/v1/dashboard/summary")
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	d := r.data()
	assert.EqualValues(t, 3, d["total_active_products"])
	assert.EqualValues(t, 1, d["low_stock_products"])
	assert.EqualValues(t, 2, d["safe_stock_products"])
	assert.EqualValues(t, 5, d["total_qty_available"])
	assert.EqualValues(t, 2, d["serial_units_available"])
	assert.EqualValues(t, 1, d["pending_lpb_approvals"])
	assert.EqualValues(t, 1, d["active_opnames"])
	assert.Len(t, d["low_stock_items"].([]any), 1)
	assert.Len(t, d["recent_pending_lpbs"].([]any), 1)
	assert.Len(t, d["recent_movements"].([]any), 2)
}
