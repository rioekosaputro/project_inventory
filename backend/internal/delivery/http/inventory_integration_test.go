//go:build integration

package http_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
)

// ---- fixture ----
//
//	WH-A: A1 (dapat dijual), A2 (KARANTINA, tidak dapat dijual)     WH-B: B1
//	P1 printer SN, min 5 : A1 on_hand 4 reserved 1 | A2 on_hand 2 | B1 on_hand 6   (total tersedia 3+0+6 = 9; di WH-A saja = 3)
//	P2 toner,      min 10: A1 on_hand 12
//	P3 PC,         min 3 : tanpa stok sama sekali
//	P4 nonaktif,   min 0 : A1 on_hand 1 (nonaktif tetapi masih menyimpan stok)
type invFixture struct {
	admin                                         string
	whA, whB                                      int64
	a1, a2, b1                                    int64
	catPrinter, catToner                          int64
	p1, p2, p3, p4                                int64
	s1, s2, s3, s4, s5                            int64
	staffA, staffB, head, tech, mgmt, driver, biz string
}

func (e *env) addSerial(productID int64, tag, sn, status string, rack, customer *int64) int64 {
	e.t.Helper()
	var id int64
	require.NoError(e.t, e.pool.QueryRow(testCtx(), `
		INSERT INTO product_serials (product_id, asset_tag, serial_number, status, current_rack_id, current_customer_id)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, productID, tag, sn, status, rack, customer).Scan(&id))
	return id
}

func newInvFixture(t *testing.T, e *env) invFixture {
	t.Helper()
	var f invFixture
	f.admin = e.adminToken("admin@x.id")
	f.whA = e.newWarehouse(f.admin, "WH-A")
	f.whB = e.newWarehouse(f.admin, "WH-B")
	f.a1 = idOf(e.newRack(f.admin, f.whA, "A-01", map[string]any{"zone": "PRINTER"}))
	f.a2 = idOf(e.newRack(f.admin, f.whA, "A-02", map[string]any{"zone": "KARANTINA"}))
	f.b1 = idOf(e.newRack(f.admin, f.whB, "B-01", nil))
	f.catPrinter = e.newCategory(f.admin, "Printer", "ASSET")
	f.catToner = e.newCategory(f.admin, "Toner", "CONSUMABLE")
	f.p1 = idOf(e.newProduct(f.admin, f.catPrinter, "PRN-1", map[string]any{"name": "Epson L3210", "has_serial_number": true, "min_stock_alert": 5}))
	f.p2 = idOf(e.newProduct(f.admin, f.catToner, "TN-1", map[string]any{"name": "Toner HP", "min_stock_alert": 10}))
	f.p3 = idOf(e.newProduct(f.admin, f.catPrinter, "PC-1", map[string]any{"name": "PC Dell", "min_stock_alert": 3}))
	f.p4 = idOf(e.newProduct(f.admin, f.catToner, "OLD-1", map[string]any{"name": "Barang Lama"}))

	e.exec(`INSERT INTO stock_balances (product_id, rack_id, qty_on_hand, qty_reserved) VALUES ($1,$2,4,1), ($1,$3,2,0), ($1,$4,6,0)`,
		f.p1, f.a1, f.a2, f.b1)
	e.exec(`INSERT INTO stock_balances (product_id, rack_id, qty_on_hand) VALUES ($1,$2,12)`, f.p2, f.a1)
	e.exec(`INSERT INTO stock_balances (product_id, rack_id, qty_on_hand) VALUES ($1,$2,1)`, f.p4, f.a1)
	e.exec(`UPDATE products SET is_active = FALSE WHERE id = $1`, f.p4)

	customer := int64(7)
	f.s1 = e.addSerial(f.p1, "AST-2611-00001", "SN-AAA", "AVAILABLE", &f.a1, nil)
	f.s2 = e.addSerial(f.p1, "AST-2611-00002", "SN-BBB", "RESERVED", &f.a1, nil)
	f.s3 = e.addSerial(f.p1, "AST-2611-00003", "SN-CCC", "AVAILABLE", &f.b1, nil)
	f.s4 = e.addSerial(f.p1, "AST-2611-00004", "SN-DDD", "RENTED", nil, &customer)
	f.s5 = e.addSerial(f.p1, "AST-2611-00005", "SN-EEE", "LOST", nil, nil)

	f.staffA = e.tokenFor(domain.RoleStaffWarehouse, &f.whA)
	f.staffB = e.tokenFor(domain.RoleStaffWarehouse, &f.whB)
	f.head = e.tokenFor(domain.RoleHeadWarehouse, nil)
	f.tech = e.tokenFor(domain.RoleTechnician, nil)
	f.mgmt = e.tokenFor(domain.RoleManagement, nil)
	f.driver = e.tokenFor(domain.RoleDriver, nil)
	f.biz = e.tokenFor(domain.RoleBusinessAdmin, nil)
	return f
}

func row(items []map[string]any, key string, value any) map[string]any {
	for _, it := range items {
		if fmt.Sprint(it[key]) == fmt.Sprint(value) {
			return it
		}
	}
	return nil
}

func skus(r *reply) []string { return names(r.list(), "sku") }

// ================================================================ RBAC

func TestInventory_RBAC(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)

	paths := []string{
		"/api/v1/inventory/balances",
		"/api/v1/inventory/balances/summary",
		"/api/v1/inventory/serials",
		"/api/v1/inventory/serials/scan/AST-2611-00001",
	}
	for _, p := range paths {
		for name, token := range map[string]string{"admin": f.admin, "management": f.mgmt, "bizadmin": f.biz, "staff": f.staffA, "head": f.head, "technician": f.tech} {
			assert.Equal(t, 200, e.get(token, p).status(), "%s harus boleh membaca %s", name, p)
		}
		forbidden := e.get(f.driver, p)
		assert.Equal(t, 403, forbidden.status(), "driver tidak boleh membaca %s", p)
		assert.Equal(t, "FORBIDDEN", forbidden.code())
		assert.Equal(t, 401, e.call("GET", p, nil).status(), "tanpa token: %s", p)
	}
	// Endpoint baca saja: tidak ada metode tulis.
	assert.Equal(t, 405, e.post(f.admin, "/api/v1/inventory/balances", map[string]any{}).status())
}

// ================================================================ balances

func TestBalances_QtyAvailableDependsOnSellableRack(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)

	r := e.get(f.admin, fmt.Sprintf("/api/v1/inventory/balances?product_id=%d&sort=rack", f.p1))
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	items := r.list()
	require.Len(t, items, 3)

	a1 := row(items, "rack_code", "A-01")
	assert.EqualValues(t, 4, a1["qty_on_hand"])
	assert.EqualValues(t, 1, a1["qty_reserved"])
	assert.EqualValues(t, 3, a1["qty_available"], "on hand − reserved di rak yang dapat dijual")
	assert.Equal(t, true, a1["is_sellable"])
	assert.Equal(t, "PRN-1", a1["sku"])
	assert.Equal(t, "WH-A", a1["warehouse_code"])

	a2 := row(items, "rack_code", "A-02")
	assert.EqualValues(t, 2, a2["qty_on_hand"], "stok fisik di karantina tetap tercatat")
	assert.EqualValues(t, 0, a2["qty_available"], "tetapi tidak dihitung tersedia")
	assert.Equal(t, false, a2["is_sellable"])
	assert.Equal(t, "KARANTINA", a2["zone"])

	b1 := row(items, "rack_code", "B-01")
	assert.EqualValues(t, 6, b1["qty_available"])
}

func TestBalances_ZeroRowsHiddenUnlessRequested(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)
	e.exec(`INSERT INTO stock_balances (product_id, rack_id, qty_on_hand) VALUES ($1,$2,0)`, f.p3, f.a1)

	assert.NotContains(t, skus(e.get(f.admin, "/api/v1/inventory/balances")), "PC-1")
	with := e.get(f.admin, "/api/v1/inventory/balances?include_zero=true")
	assert.Contains(t, skus(with), "PC-1")
	assert.Equal(t, "PC-1", row(with.list(), "sku", "PC-1")["sku"])
}

func TestBalances_LowStockIsPerProductWithinWarehouseScope(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)
	url := fmt.Sprintf("/api/v1/inventory/balances?product_id=%d", f.p1)

	// Admin: total tersedia P1 di semua gudang = 3 + 0 + 6 = 9 ≥ 5 → aman, di SEMUA barisnya.
	for _, it := range e.get(f.admin, url).list() {
		assert.Equal(t, false, it["is_low_stock"], "rak %v", it["rack_code"])
	}
	// Staf WH-A hanya melihat gudangnya: tersedia 3 < 5 → menipis, juga untuk rak karantina.
	scoped := e.get(f.staffA, url).list()
	require.Len(t, scoped, 2)
	for _, it := range scoped {
		assert.Equal(t, true, it["is_low_stock"], "rak %v", it["rack_code"])
	}
	// Filter gudang pilihan pengguna ikut menentukan cakupan penilaian.
	byWH := e.get(f.admin, fmt.Sprintf("%s&warehouse_id=%d", url, f.whA)).list()
	for _, it := range byWH {
		assert.Equal(t, true, it["is_low_stock"])
	}
	// Filter rak tidak mengubah penilaian (tetap tingkat produk).
	oneRack := e.get(f.admin, fmt.Sprintf("%s&rack_id=%d", url, f.a1)).list()
	require.Len(t, oneRack, 1)
	assert.Equal(t, false, oneRack[0]["is_low_stock"])
	// Produk dengan min_stock_alert 0 tidak pernah menipis.
	for _, it := range e.get(f.admin, fmt.Sprintf("/api/v1/inventory/balances?product_id=%d", f.p4)).list() {
		assert.Equal(t, false, it["is_low_stock"])
	}
}

func TestBalances_LowStockFilter(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)

	assert.Empty(t, e.get(f.admin, "/api/v1/inventory/balances?low_stock=true").list(), "tidak ada yang menipis secara global")
	low := e.get(f.staffA, "/api/v1/inventory/balances?low_stock=true")
	assert.ElementsMatch(t, []string{"PRN-1", "PRN-1"}, skus(low), "dua rak P1 di WH-A")
	assert.EqualValues(t, 2, low.meta()["total"])
}

func TestBalances_FiltersSortAndPagination(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)

	assert.EqualValues(t, 5, e.get(f.admin, "/api/v1/inventory/balances").meta()["total"], "P1×3 + P2 + P4")
	assert.Len(t, e.get(f.admin, fmt.Sprintf("/api/v1/inventory/balances?rack_id=%d", f.a1)).list(), 3, "P1, P2, P4 di A-01")
	assert.Len(t, e.get(f.admin, fmt.Sprintf("/api/v1/inventory/balances?warehouse_id=%d", f.whB)).list(), 1)
	assert.Equal(t, []string{"TN-1"}, skus(e.get(f.admin, "/api/v1/inventory/balances?q=toner")), "q mencari nama")
	assert.Equal(t, []string{"PRN-1", "PRN-1", "PRN-1"}, skus(e.get(f.admin, "/api/v1/inventory/balances?q=prn")))

	desc := e.get(f.admin, fmt.Sprintf("/api/v1/inventory/balances?product_id=%d&sort=-qty_on_hand", f.p1)).list()
	assert.Equal(t, []string{"B-01", "A-01", "A-02"}, names(desc, "rack_code"))
	avail := e.get(f.admin, fmt.Sprintf("/api/v1/inventory/balances?product_id=%d&sort=qty_available", f.p1)).list()
	assert.Equal(t, []string{"A-02", "A-01", "B-01"}, names(avail, "rack_code"))

	page := e.get(f.admin, "/api/v1/inventory/balances?limit=2&page=3")
	assert.Len(t, page.list(), 1)
	assert.EqualValues(t, 3, page.meta()["total_pages"])

	assert.Equal(t, 400, e.get(f.admin, "/api/v1/inventory/balances?sort=password").status())
	assert.Equal(t, 400, e.get(f.admin, "/api/v1/inventory/balances?sort=sku%3BDROP%20TABLE%20stock_balances").status())
	assert.Equal(t, 400, e.get(f.admin, "/api/v1/inventory/balances?product_id=abc").status())
	assert.Equal(t, 5, e.count(`SELECT count(*) FROM stock_balances`), "tabel utuh")
}

func TestBalances_WarehouseScoping(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)

	a := e.get(f.staffA, "/api/v1/inventory/balances")
	assert.EqualValues(t, 4, a.meta()["total"], "P1×2, P2, P4 — hanya WH-A")
	for _, it := range a.list() {
		assert.Equal(t, "WH-A", it["warehouse_code"])
	}
	b := e.get(f.staffB, "/api/v1/inventory/balances")
	assert.Equal(t, []string{"PRN-1"}, skus(b))

	denied := e.get(f.staffA, fmt.Sprintf("/api/v1/inventory/balances?warehouse_id=%d", f.whB))
	assert.Equal(t, 403, denied.status())
	assert.Equal(t, "FORBIDDEN_WAREHOUSE", denied.code())
	assert.Equal(t, 200, e.get(f.staffA, fmt.Sprintf("/api/v1/inventory/balances?warehouse_id=%d", f.whA)).status())
	// rak milik gudang lain tidak bocor lewat filter rak
	assert.Empty(t, e.get(f.staffA, fmt.Sprintf("/api/v1/inventory/balances?rack_id=%d", f.b1)).list())

	// Akun tanpa batas gudang (kepala gudang global, teknisi) melihat semuanya.
	assert.EqualValues(t, 5, e.get(f.head, "/api/v1/inventory/balances").meta()["total"])
	assert.EqualValues(t, 5, e.get(f.tech, "/api/v1/inventory/balances").meta()["total"])
}

// ================================================================ summary

func TestStockSummary_TotalsAndZeroStockProducts(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)

	r := e.get(f.admin, "/api/v1/inventory/balances/summary")
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	items := r.list()
	assert.Equal(t, []string{"OLD-1", "PC-1", "PRN-1", "TN-1"}, skus(r), "produk nonaktif yang masih punya stok tetap tampil; urut SKU")

	p1 := row(items, "sku", "PRN-1")
	assert.EqualValues(t, 12, p1["qty_on_hand"])
	assert.EqualValues(t, 1, p1["qty_reserved"])
	assert.EqualValues(t, 9, p1["qty_available"], "3 + 0 (karantina) + 6")
	assert.EqualValues(t, 3, p1["rack_count"])
	assert.Equal(t, false, p1["is_low_stock"])
	assert.Equal(t, "Printer", p1["category_name"])
	assert.Equal(t, "ASSET", p1["item_type"])

	p3 := row(items, "sku", "PC-1")
	assert.EqualValues(t, 0, p3["qty_on_hand"], "tanpa stok sama sekali tetap tampil")
	assert.EqualValues(t, 0, p3["rack_count"])
	assert.Equal(t, true, p3["is_low_stock"], "0 < min 3")

	assert.Equal(t, false, row(items, "sku", "TN-1")["is_low_stock"], "12 ≥ 10")
	assert.Equal(t, false, row(items, "sku", "OLD-1")["is_low_stock"], "min 0 tidak pernah menipis")
	assert.Equal(t, false, row(items, "sku", "OLD-1")["is_active"])
}

func TestStockSummary_Filters(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)
	get := func(token, q string) []string { return skus(e.get(token, "/api/v1/inventory/balances/summary"+q)) }

	assert.Equal(t, []string{"PC-1"}, get(f.admin, "?low_stock=true"))
	assert.Equal(t, []string{"OLD-1", "PRN-1", "TN-1"}, get(f.admin, "?has_stock=true"))
	assert.Equal(t, []string{"OLD-1", "TN-1"}, get(f.admin, "?item_type=CONSUMABLE"), "OLD-1 nonaktif tetap tampil karena masih menyimpan stok")
	assert.Equal(t, []string{"OLD-1"}, get(f.admin, "?is_active=false"))
	assert.Equal(t, []string{"PC-1", "PRN-1"}, get(f.admin, fmt.Sprintf("?category_id=%d", f.catPrinter)))
	assert.Equal(t, []string{"PRN-1"}, get(f.admin, "?q=epson"))
	assert.Equal(t, []string{"TN-1", "PRN-1", "PC-1", "OLD-1"}, get(f.admin, "?sort=-sku"))
	assert.Equal(t, []string{"PRN-1", "TN-1", "OLD-1", "PC-1"}, get(f.admin, "?sort=-qty_on_hand,sku"))
	assert.Equal(t, 400, e.get(f.admin, "/api/v1/inventory/balances/summary?sort=min_stock_alert").status())
	assert.Equal(t, 400, e.get(f.admin, "/api/v1/inventory/balances/summary?item_type=LAINNYA").status())
}

func TestStockSummary_WarehouseScopeChangesTotalsAndLowStock(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)

	a := e.get(f.staffA, "/api/v1/inventory/balances/summary")
	p1 := row(a.list(), "sku", "PRN-1")
	assert.EqualValues(t, 6, p1["qty_on_hand"], "4 + 2 di WH-A saja")
	assert.EqualValues(t, 3, p1["qty_available"])
	assert.EqualValues(t, 2, p1["rack_count"])
	assert.Equal(t, true, p1["is_low_stock"], "3 < 5 di cakupan WH-A")
	assert.ElementsMatch(t, []string{"PC-1", "PRN-1"}, skus(e.get(f.staffA, "/api/v1/inventory/balances/summary?low_stock=true")))

	b := e.get(f.staffB, "/api/v1/inventory/balances/summary")
	assert.EqualValues(t, 6, row(b.list(), "sku", "PRN-1")["qty_on_hand"])
	assert.EqualValues(t, 0, row(b.list(), "sku", "TN-1")["qty_on_hand"], "toner hanya ada di WH-A")

	denied := e.get(f.staffA, fmt.Sprintf("/api/v1/inventory/balances/summary?warehouse_id=%d", f.whB))
	assert.Equal(t, 403, denied.status())
	byWH := e.get(f.admin, fmt.Sprintf("/api/v1/inventory/balances/summary?warehouse_id=%d", f.whB))
	assert.EqualValues(t, 6, row(byWH.list(), "sku", "PRN-1")["qty_on_hand"])
}

// ================================================================ serials

func TestSerials_ListAndFilters(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)
	tags := func(token, q string) []string {
		return names(e.get(token, "/api/v1/inventory/serials"+q).list(), "asset_tag")
	}

	all := e.get(f.admin, "/api/v1/inventory/serials")
	require.Equal(t, 200, all.status(), all.rec.Body.String())
	assert.EqualValues(t, 5, all.meta()["total"])
	assert.Equal(t, "AST-2611-00001", all.list()[0]["asset_tag"], "urut asset tag")

	s1 := row(all.list(), "asset_tag", "AST-2611-00001")
	assert.Equal(t, "AVAILABLE", s1["status"])
	assert.Equal(t, "A-01", s1["rack_code"])
	assert.Equal(t, "WH-A", s1["warehouse_code"])
	assert.Equal(t, "PRN-1", s1["sku"])
	assert.Equal(t, "Epson L3210", s1["product_name"])
	assert.Equal(t, "GOOD", s1["condition"])

	rented := row(all.list(), "asset_tag", "AST-2611-00004")
	assert.EqualValues(t, 7, rented["customer_id"])
	assert.Nil(t, rented["rack_code"])
	assert.Nil(t, rented["warehouse_code"])

	assert.Equal(t, []string{"AST-2611-00002"}, tags(f.admin, "?status=RESERVED"))
	assert.Equal(t, []string{"AST-2611-00004"}, tags(f.admin, "?customer_id=7"))
	assert.Equal(t, []string{"AST-2611-00001", "AST-2611-00002"}, tags(f.admin, fmt.Sprintf("?warehouse_id=%d", f.whA)))
	assert.Equal(t, []string{"AST-2611-00003"}, tags(f.admin, fmt.Sprintf("?rack_id=%d", f.b1)))
	assert.Len(t, tags(f.admin, fmt.Sprintf("?product_id=%d", f.p1)), 5)
	assert.Empty(t, tags(f.admin, fmt.Sprintf("?product_id=%d", f.p2)))
	assert.Equal(t, []string{"AST-2611-00003"}, tags(f.admin, "?q=ccc"), "q mencari SN tanpa peduli huruf")
	assert.Equal(t, []string{"AST-2611-00005"}, tags(f.admin, "?q=2611-00005"), "q mencari sebagian asset tag")
	assert.Len(t, tags(f.admin, "?q=epson"), 5, "q juga mencari nama produk")

	assert.Equal(t, []string{"SN-EEE", "SN-DDD", "SN-CCC", "SN-BBB", "SN-AAA"},
		names(e.get(f.admin, "/api/v1/inventory/serials?sort=-serial_number").list(), "serial_number"))

	invalid := e.get(f.admin, "/api/v1/inventory/serials?status=HILANG")
	assert.Equal(t, 422, invalid.status())
	assert.Contains(t, fieldErrors(invalid), "status")
	assert.Equal(t, 400, e.get(f.admin, "/api/v1/inventory/serials?sort=purchase_price").status(), "harga beli tidak diekspos")
	assert.NotContains(t, all.rec.Body.String(), "purchase_price")
}

func TestSerials_WarehouseScoping(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)
	tags := func(token string) []string {
		return names(e.get(token, "/api/v1/inventory/serials").list(), "asset_tag")
	}

	assert.Equal(t, []string{"AST-2611-00001", "AST-2611-00002", "AST-2611-00004", "AST-2611-00005"}, tags(f.staffA),
		"unit di WH-A + unit yang tidak berada di gudang mana pun (disewa/hilang); tidak melihat B-01")
	assert.Equal(t, []string{"AST-2611-00003", "AST-2611-00004", "AST-2611-00005"}, tags(f.staffB))
	assert.Len(t, tags(f.head), 5)

	denied := e.get(f.staffA, fmt.Sprintf("/api/v1/inventory/serials?warehouse_id=%d", f.whB))
	assert.Equal(t, 403, denied.status())
	assert.Empty(t, e.get(f.staffA, fmt.Sprintf("/api/v1/inventory/serials?rack_id=%d", f.b1)).list(), "rak gudang lain tidak bocor")
}

// ================================================================ scan

func TestScan_ByAssetTagReturnsUnitAndFiveLatestHistories(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)
	admin := f.admin
	var userID int64
	require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT id FROM users WHERE email='admin@x.id'`).Scan(&userID))
	for i := 1; i <= 7; i++ {
		e.exec(`INSERT INTO product_serial_histories (product_serial_id, from_status, to_status, from_rack_id, to_rack_id, event_type, reference_type, reference_id, user_id, notes, created_at)
			VALUES ($1, $2, 'AVAILABLE', $3, $4, $5, 'LPB', $6, $7, $8, NOW() - ($9::text || ' hours')::interval)`,
			f.s1, nilIf(i == 1, "RESERVED"), nilInt(i == 1, f.a2), f.a1, fmt.Sprintf("EVENT_%d", i), int64(100+i), userID, fmt.Sprintf("catatan %d", i), fmt.Sprint(10-i))
	}

	r := e.get(admin, "/api/v1/inventory/serials/scan/ast-2611-00001") // huruf kecil: asset tag tidak peka huruf
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	assert.Equal(t, "AST-2611-00001", r.str("asset_tag"))
	assert.Equal(t, "SN-AAA", r.str("serial_number"))
	assert.Equal(t, "AVAILABLE", r.str("status"))
	assert.Equal(t, "A-01", r.str("rack_code"))
	assert.Equal(t, "WH-A", r.str("warehouse_code"))
	assert.Equal(t, "PRN-1", r.str("sku"))

	hist, ok := r.data()["history"].([]any)
	require.True(t, ok)
	require.Len(t, hist, 5, "tepat 5 histori terakhir dari 7")
	first := hist[0].(map[string]any)
	assert.Equal(t, "EVENT_7", first["event_type"], "terbaru dulu")
	assert.Equal(t, "EVENT_3", hist[4].(map[string]any)["event_type"])
	assert.Equal(t, "admin@x.id", first["user_name"])
	assert.Equal(t, "A-01", first["to_rack_code"])
	assert.EqualValues(t, 107, first["reference_id"])
}

func nilIf(cond bool, v string) any {
	if cond {
		return v
	}
	return nil
}
func nilInt(cond bool, v int64) any {
	if cond {
		return v
	}
	return nil
}

func TestScan_BySerialNumber_ExactAndCaseInsensitive(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)

	exact := e.get(f.admin, "/api/v1/inventory/serials/scan/SN-CCC")
	assert.Equal(t, 200, exact.status())
	assert.Equal(t, "AST-2611-00003", exact.str("asset_tag"))
	lower := e.get(f.admin, "/api/v1/inventory/serials/scan/sn-ccc")
	assert.Equal(t, 200, lower.status())
	assert.Equal(t, "AST-2611-00003", lower.str("asset_tag"))
	assert.Empty(t, lower.data()["history"], "tanpa histori → array kosong")
	assert.NotNil(t, lower.data()["history"])

	spaced := e.get(f.admin, "/api/v1/inventory/serials/scan/%20SN-AAA%20")
	assert.Equal(t, 200, spaced.status(), "spasi di tepi dipangkas (scanner)")
}

func TestScan_RentedUnitShowsCustomer(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)
	r := e.get(f.staffA, "/api/v1/inventory/serials/scan/AST-2611-00004")
	require.Equal(t, 200, r.status(), "unit di pelanggan terlihat oleh akun satu gudang")
	assert.Equal(t, "RENTED", r.str("status"))
	assert.EqualValues(t, 7, r.data()["customer_id"])
	assert.Nil(t, r.data()["rack_code"])
}

func TestScan_NotFound_Ambiguous_Invalid(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)

	nf := e.get(f.admin, "/api/v1/inventory/serials/scan/TIDAK-ADA")
	assert.Equal(t, 404, nf.status())
	assert.Equal(t, "NOT_FOUND", nf.code())
	assert.Contains(t, nf.rec.Body.String(), "TIDAK-ADA")

	// SN sama pada dua produk berbeda: ambigu, jangan menebak.
	e.addSerial(f.p3, "AST-2611-00006", "SN-AAA", "AVAILABLE", &f.a1, nil)
	amb := e.get(f.admin, "/api/v1/inventory/serials/scan/SN-AAA")
	assert.Equal(t, 409, amb.status())
	assert.Equal(t, "SERIAL_AMBIGUOUS", amb.code())
	// asset tag selalu pasti
	assert.Equal(t, 200, e.get(f.admin, "/api/v1/inventory/serials/scan/AST-2611-00001").status())
	assert.Equal(t, "SN-AAA", e.get(f.admin, "/api/v1/inventory/serials/scan/AST-2611-00006").str("serial_number"))

	long := e.get(f.admin, "/api/v1/inventory/serials/scan/"+strings.Repeat("X", 101))
	assert.Equal(t, 422, long.status())
	assert.Contains(t, fieldErrors(long), "code")
	blank := e.get(f.admin, "/api/v1/inventory/serials/scan/%20%20")
	assert.Equal(t, 422, blank.status())
}

func TestScan_WarehouseScoping(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)

	assert.Equal(t, 200, e.get(f.staffA, "/api/v1/inventory/serials/scan/AST-2611-00001").status())
	other := e.get(f.staffA, "/api/v1/inventory/serials/scan/AST-2611-00003") // di B-01
	assert.Equal(t, 403, other.status())
	assert.Equal(t, "FORBIDDEN_WAREHOUSE", other.code())
	assert.NotContains(t, other.rec.Body.String(), "SN-CCC", "detail unit gudang lain tidak bocor")
	assert.Equal(t, 200, e.get(f.head, "/api/v1/inventory/serials/scan/AST-2611-00003").status())
	assert.Equal(t, 200, e.get(f.tech, "/api/v1/inventory/serials/scan/AST-2611-00003").status())
}

// ================================================================ keamanan

func TestInventory_ReadOnly_DoesNotChangeAnything(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)
	before := e.count(`SELECT COALESCE(SUM(qty_on_hand + qty_reserved), 0) FROM stock_balances`)
	serialsBefore := e.count(`SELECT count(*) FROM product_serials`)

	for _, p := range []string{
		"/api/v1/inventory/balances", "/api/v1/inventory/balances/summary",
		"/api/v1/inventory/serials", "/api/v1/inventory/serials/scan/AST-2611-00001",
	} {
		e.get(f.admin, p)
	}
	assert.Equal(t, before, e.count(`SELECT COALESCE(SUM(qty_on_hand + qty_reserved), 0) FROM stock_balances`))
	assert.Equal(t, serialsBefore, e.count(`SELECT count(*) FROM product_serials`))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM stock_ledgers`))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM audit_logs WHERE entity_type IN ('stock','serial')`))
}

func TestInventory_SearchWildcardsAreLiteral(t *testing.T) {
	e := newEnv(t)
	f := newInvFixture(t, e)
	for _, p := range []string{"balances", "balances/summary", "serials"} {
		for _, q := range []string{"%25", "_", "%25PRN%25"} {
			r := e.get(f.admin, "/api/v1/inventory/"+p+"?q="+q)
			assert.Equal(t, 200, r.status())
			assert.Empty(t, r.list(), "%s q=%s", p, q)
		}
	}
}
