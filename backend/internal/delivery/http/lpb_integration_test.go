//go:build integration

package http_test

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

// ---- fixture ----
//
//	WH-A: A1 (dapat dijual), A2 (KARANTINA)   WH-B: B1   supplier SUP-1
//	P1 printer ber-SN, P2 toner non-SN, P3 PC ber-SN
type lpbFx struct {
	admin, staffA, staff2A, headA, headG, staffB, mgmt, biz, driver, tech string
	staffAID, staff2AID, headAID                                          int64
	whA, whB, a1, a2, b1, supplier, p1, p2, p3                            int64
}

func (e *env) userToken(role domain.Role, warehouseID *int64) (int64, string) {
	e.t.Helper()
	email := fmt.Sprintf("%s-%d@x.id", strings.ToLower(string(role)), userSeq.Add(1))
	id := e.addUser(email, role)
	return id, e.mint(id, role, warehouseID)
}

func newLPBFixture(t *testing.T, e *env) lpbFx {
	t.Helper()
	var f lpbFx
	f.admin = e.adminToken("admin@x.id")
	f.whA = e.newWarehouse(f.admin, "WH-A")
	f.whB = e.newWarehouse(f.admin, "WH-B")
	f.a1 = idOf(e.newRack(f.admin, f.whA, "A-01", map[string]any{"zone": "PRINTER"}))
	f.a2 = idOf(e.newRack(f.admin, f.whA, "A-02", map[string]any{"zone": "KARANTINA"}))
	f.b1 = idOf(e.newRack(f.admin, f.whB, "B-01", nil))
	f.supplier = idOf(e.post(f.admin, "/api/v1/suppliers", map[string]any{"code": "SUP-1", "name": "PT Maju Jaya"}))
	cat := e.newCategory(f.admin, "Printer", "ASSET")
	toner := e.newCategory(f.admin, "Toner", "CONSUMABLE")
	f.p1 = idOf(e.newProduct(f.admin, cat, "PRN-1", map[string]any{"name": "Epson L3210", "has_serial_number": true}))
	f.p2 = idOf(e.newProduct(f.admin, toner, "TN-1", map[string]any{"name": "Toner HP"}))
	f.p3 = idOf(e.newProduct(f.admin, cat, "PC-1", map[string]any{"name": "PC Dell", "has_serial_number": true}))

	f.staffAID, f.staffA = e.userToken(domain.RoleStaffWarehouse, &f.whA)
	f.staff2AID, f.staff2A = e.userToken(domain.RoleStaffWarehouse, &f.whA)
	f.headAID, f.headA = e.userToken(domain.RoleHeadWarehouse, &f.whA)
	_, f.headG = e.userToken(domain.RoleHeadWarehouse, nil)
	_, f.staffB = e.userToken(domain.RoleStaffWarehouse, &f.whB)
	_, f.mgmt = e.userToken(domain.RoleManagement, nil)
	_, f.biz = e.userToken(domain.RoleBusinessAdmin, nil)
	_, f.driver = e.userToken(domain.RoleDriver, nil)
	_, f.tech = e.userToken(domain.RoleTechnician, nil)
	return f
}

func lpbItem(product, rack int64, qty int, extra ...map[string]any) map[string]any {
	m := map[string]any{"product_id": product, "rack_id": rack, "qty": qty}
	for _, x := range extra {
		for k, v := range x {
			m[k] = v
		}
	}
	return m
}

func (f lpbFx) purchase(items ...map[string]any) map[string]any {
	return map[string]any{"sub_type": "PURCHASE", "warehouse_id": f.whA, "supplier_id": f.supplier, "items": items}
}

func (e *env) newLPB(token string, body map[string]any) *reply {
	e.t.Helper()
	return e.post(token, "/api/v1/lpb", body)
}

func (e *env) mustLPB(token string, body map[string]any) *reply {
	e.t.Helper()
	r := e.newLPB(token, body)
	require.Equal(e.t, 201, r.status(), r.rec.Body.String())
	return r
}

func lpbPath(id int64, suffix string) string { return fmt.Sprintf("/api/v1/lpb/%d%s", id, suffix) }

func itemsOf(r *reply) []map[string]any {
	raw, _ := r.data()["items"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, x := range raw {
		out = append(out, x.(map[string]any))
	}
	return out
}

func itemID(r *reply, idx int) int64 { return int64(itemsOf(r)[idx]["id"].(float64)) }

func (e *env) scan(token string, lpb, item int64, sn string, extra ...map[string]any) *reply {
	e.t.Helper()
	body := map[string]any{"serial_number": sn}
	for _, x := range extra {
		for k, v := range x {
			body[k] = v
		}
	}
	return e.post(token, fmt.Sprintf("/api/v1/lpb/%d/items/%d/serials", lpb, item), body)
}

func timelineActions(r *reply) []string {
	raw, _ := r.data()["timeline"].([]any)
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		out = append(out, x.(map[string]any)["action"].(string))
	}
	return out
}

func (e *env) lockRack(rackID, warehouseID int64) {
	e.t.Helper()
	var opname int64
	require.NoError(e.t, e.pool.QueryRow(testCtx(), `
		INSERT INTO stock_opnames (opname_number, warehouse_id, started_by)
		VALUES ('OPN/' || $1::bigint::text, $2, (SELECT id FROM users LIMIT 1)) RETURNING id`, rackID, warehouseID).Scan(&opname))
	e.exec(`UPDATE racks SET locked_by_opname_id = $2 WHERE id = $1`, rackID, opname)
}

func (e *env) auditCount(action string, lpbID int64) int {
	return e.count(`SELECT count(*) FROM audit_logs WHERE action = $1 AND entity_type = 'lpb' AND entity_id = $2`, action, lpbID)
}

// ================================================================ RBAC

func TestLPB_RBAC(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	created := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 5)))
	id := idOf(created)

	canRead := map[string]string{"staff": f.staffA, "head": f.headA, "management": f.mgmt, "admin": f.admin}
	cannotRead := map[string]string{"bizadmin": f.biz, "driver": f.driver, "technician": f.tech}
	for name, tok := range canRead {
		assert.Equal(t, 200, e.get(tok, "/api/v1/lpb").status(), "%s list", name)
		assert.Equal(t, 200, e.get(tok, lpbPath(id, "")).status(), "%s detail", name)
	}
	for name, tok := range cannotRead {
		assert.Equal(t, 403, e.get(tok, "/api/v1/lpb").status(), "%s list", name)
		assert.Equal(t, 403, e.get(tok, lpbPath(id, "")).status(), "%s detail", name)
	}

	// Tulis: hanya staf/kepala gudang (+ADMIN). Manajemen hanya membaca.
	for name, tok := range map[string]string{"management": f.mgmt, "bizadmin": f.biz, "driver": f.driver, "technician": f.tech} {
		assert.Equal(t, 403, e.newLPB(tok, f.purchase(lpbItem(f.p2, f.a1, 1))).status(), "%s create", name)
		assert.Equal(t, 403, e.patchReq(tok, lpbPath(id, ""), map[string]any{"notes": "x"}).status(), "%s patch", name)
		assert.Equal(t, 403, e.post(tok, lpbPath(id, "/submit"), nil).status(), "%s submit", name)
		assert.Equal(t, 403, e.post(tok, lpbPath(id, "/cancel"), nil).status(), "%s cancel", name)
		assert.Equal(t, 403, e.post(tok, lpbPath(id, "/revise"), nil).status(), "%s revise", name)
	}
	for _, p := range []string{"/api/v1/lpb", lpbPath(id, "")} {
		assert.Equal(t, 401, e.call("GET", p, nil).status(), "tanpa token %s", p)
	}
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM lpb_documents`), "percobaan ditolak tidak membuat dokumen")
}

// ================================================================ buat

func TestLPB_Create_Purchase(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)

	r := e.mustLPB(f.staffA, map[string]any{
		"sub_type": "PURCHASE", "warehouse_id": f.whA, "supplier_id": f.supplier, "reference_number": " INV-2026-001 ",
		"notes": " Pembelian printer ", "items": []map[string]any{
			lpbItem(f.p1, f.a1, 2, map[string]any{"unit_cost": "1500000"}),
			lpbItem(f.p2, f.a1, 10, map[string]any{"unit_cost": "250000.50", "notes": "dus"}),
		},
	})

	assert.Regexp(t, regexp.MustCompile(`^LPBI/WH-A/\d{4}/\d{2}/0001$`), r.str("lpb_number"))
	assert.Equal(t, "INBOUND", r.str("type"))
	assert.Equal(t, "PURCHASE", r.str("sub_type"))
	assert.Equal(t, "DRAFT", r.str("status"))
	assert.Equal(t, clock.DateWIB(e.clk.Now()), r.str("doc_date"), "tanggal bisnis WIB")
	assert.Equal(t, "WH-A", r.str("warehouse_code"))
	assert.Equal(t, "PT Maju Jaya", r.str("supplier_name"))
	assert.Equal(t, "INV-2026-001", r.str("reference_number"), "di-trim")
	assert.Equal(t, "PURCHASE_INVOICE", r.str("reference_type"))
	assert.Equal(t, "Pembelian printer", r.str("notes"))
	assert.EqualValues(t, f.staffAID, r.data()["created_by"])

	items := itemsOf(r)
	require.Len(t, items, 2)
	assert.Equal(t, "PRN-1", items[0]["sku"])
	assert.Equal(t, "A-01", items[0]["rack_code"])
	assert.Equal(t, "1500000.00", items[0]["unit_cost"], "uang string 2 desimal")
	assert.Equal(t, true, items[0]["has_serial_number"])
	assert.EqualValues(t, 0, items[0]["scanned_count"])
	assert.Equal(t, "250000.50", items[1]["unit_cost"])
	assert.Equal(t, "dus", items[1]["notes"])

	assert.Equal(t, []string{"LPB_CREATE"}, timelineActions(r))
	tl := r.data()["timeline"].([]any)[0].(map[string]any)
	assert.Contains(t, tl["user_name"], "staff_warehouse")

	// Nomor berikutnya berurutan; keluar memakai prefix LPBO.
	second := e.mustLPB(f.staffA, f.purchase())
	assert.Regexp(t, `/0002$`, second.str("lpb_number"))
	assert.Equal(t, 0, len(itemsOf(second)), "draft boleh dibuat tanpa item; wajib minimal 1 saat submit")

	// Pembelian tanpa nomor referensi: reference_type kosong.
	noRef := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1)))
	assert.Nil(t, noRef.data()["reference_type"])
}

func TestLPB_Create_InitialStockAndDisposalUsePrefixes(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)

	init := e.mustLPB(f.staffA, map[string]any{"sub_type": "INITIAL_STOCK", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(f.p2, f.a1, 3)}})
	assert.Regexp(t, `^LPBI/WH-A/`, init.str("lpb_number"))
	assert.Equal(t, "INBOUND", init.str("type"))
	assert.Nil(t, init.data()["supplier_id"])

	disp := e.mustLPB(f.staffA, map[string]any{"sub_type": "DISPOSAL", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(f.p1, f.a2, 1)}})
	assert.Regexp(t, `^LPBO/WH-A/`, disp.str("lpb_number"))
	assert.Equal(t, "OUTBOUND", disp.str("type"))
	assert.Regexp(t, `/0001$`, disp.str("lpb_number"), "urutan OUTBOUND terpisah dari INBOUND")
}

func TestLPB_Create_Validation(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	e.exec(`UPDATE racks SET is_active = FALSE WHERE id = $1`, f.a2)
	cat := e.newCategory(f.admin, "Lama", "ASSET")
	inactiveP := idOf(e.newProduct(f.admin, cat, "OLD-1", nil))
	e.exec(`UPDATE products SET is_active = FALSE WHERE id = $1`, inactiveP)
	inactiveSup := idOf(e.post(f.admin, "/api/v1/suppliers", map[string]any{"code": "SUP-OFF", "name": "Off"}))
	e.exec(`UPDATE suppliers SET is_active = FALSE WHERE id = $1`, inactiveSup)
	tomorrow := time.Now().In(clock.WIB).AddDate(0, 0, 1).Format("2006-01-02")

	base := func(mod func(map[string]any)) map[string]any {
		b := f.purchase(lpbItem(f.p2, f.a1, 1))
		mod(b)
		return b
	}
	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"jenis tidak dikenal", base(func(b map[string]any) { b["sub_type"] = "NGAWUR" }), "sub_type"},
		{"jenis otomatis tidak dapat dibuat manual (RENTAL_DELIVERY)", base(func(b map[string]any) { b["sub_type"] = "RENTAL_DELIVERY"; delete(b, "supplier_id") }), "sub_type"},
		{"TRANSFER_OUT belum dapat dibuat manual", base(func(b map[string]any) { b["sub_type"] = "TRANSFER_OUT"; delete(b, "supplier_id") }), "sub_type"},
		{"pembelian wajib supplier", base(func(b map[string]any) { delete(b, "supplier_id") }), "supplier_id"},
		{"supplier hanya untuk pembelian", base(func(b map[string]any) { b["sub_type"] = "INITIAL_STOCK" }), "supplier_id"},
		{"supplier tidak ditemukan", base(func(b map[string]any) { b["supplier_id"] = 999999 }), "supplier_id"},
		{"supplier tidak aktif", base(func(b map[string]any) { b["supplier_id"] = inactiveSup }), "supplier_id"},
		{"tanggal di masa depan", base(func(b map[string]any) { b["doc_date"] = tomorrow }), "doc_date"},
		{"nomor referensi terlalu panjang", base(func(b map[string]any) { b["reference_number"] = strings.Repeat("x", 61) }), "reference_number"},
		{"produk tidak ditemukan", base(func(b map[string]any) { b["items"] = []map[string]any{lpbItem(999999, f.a1, 1)} }), "items[0].product_id"},
		{"produk tidak aktif", base(func(b map[string]any) { b["items"] = []map[string]any{lpbItem(inactiveP, f.a1, 1)} }), "items[0].product_id"},
		{"rak tidak ditemukan", base(func(b map[string]any) { b["items"] = []map[string]any{lpbItem(f.p2, 999999, 1)} }), "items[0].rack_id"},
		{"rak tidak aktif", base(func(b map[string]any) { b["items"] = []map[string]any{lpbItem(f.p2, f.a2, 1)} }), "items[0].rack_id"},
		{"rak milik gudang lain", base(func(b map[string]any) { b["items"] = []map[string]any{lpbItem(f.p2, f.b1, 1)} }), "items[0].rack_id"},
		{"produk+rak ganda", base(func(b map[string]any) { b["items"] = []map[string]any{lpbItem(f.p2, f.a1, 1), lpbItem(f.p2, f.a1, 2)} }), "items[1]"},
		{"biaya tidak valid", base(func(b map[string]any) {
			b["items"] = []map[string]any{lpbItem(f.p2, f.a1, 1, map[string]any{"unit_cost": "12.345"})}
		}), "items[0].unit_cost"},
		{"biaya negatif", base(func(b map[string]any) {
			b["items"] = []map[string]any{lpbItem(f.p2, f.a1, 1, map[string]any{"unit_cost": "-1"})}
		}), "items[0].unit_cost"},
		{"qty nol", base(func(b map[string]any) { b["items"] = []map[string]any{lpbItem(f.p2, f.a1, 0)} }), "items[0].qty"},
		{"qty melebihi batas", base(func(b map[string]any) { b["items"] = []map[string]any{lpbItem(f.p2, f.a1, 1000001)} }), "items[0].qty"},
		{"biaya pada pengeluaran (disposal)", map[string]any{"sub_type": "DISPOSAL", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(f.p1, f.a1, 1, map[string]any{"unit_cost": "100"})}}, "items[0].unit_cost"},
	}
	for _, tc := range cases {
		r := e.newLPB(f.staffA, tc.body)
		assert.Contains(t, []int{400, 422}, r.status(), tc.name+": "+r.rec.Body.String())
		assert.Contains(t, fieldErrors(r), tc.field, tc.name+": "+r.rec.Body.String())
	}
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM lpb_documents`), "tidak ada yang tersimpan")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM document_sequences`), "kegagalan tidak menghabiskan nomor dokumen")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM audit_logs WHERE action LIKE 'LPB_%'`))

	tooMany := make([]map[string]any, 201)
	for i := range tooMany {
		tooMany[i] = lpbItem(f.p2, f.a1, 1)
	}
	assert.Equal(t, 400, e.newLPB(f.staffA, f.purchase(tooMany...)).status(), "maksimal 200 item")

	// gudang bermasalah
	e.exec(`UPDATE warehouses SET is_active = FALSE WHERE id = $1`, f.whB)
	_, tokenB := e.userToken(domain.RoleHeadWarehouse, nil)
	off := e.newLPB(tokenB, map[string]any{"sub_type": "INITIAL_STOCK", "warehouse_id": f.whB})
	assert.Equal(t, 422, off.status())
	assert.Contains(t, fieldErrors(off), "warehouse_id")
	missing := e.newLPB(tokenB, map[string]any{"sub_type": "INITIAL_STOCK", "warehouse_id": 999999})
	assert.Equal(t, 422, missing.status())
}

func TestLPB_Create_RackLockedByOpname_409(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	e.lockRack(f.a1, f.whA)
	r := e.newLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1)))
	assert.Equal(t, 409, r.status())
	assert.Equal(t, "RACK_LOCKED_BY_OPNAME", r.code())
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM lpb_documents`))
}

func TestLPB_Create_WarehouseScoping(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	r := e.newLPB(f.staffB, f.purchase(lpbItem(f.p2, f.a1, 1))) // staf WH-B membuat di WH-A
	assert.Equal(t, 403, r.status())
	assert.Equal(t, "FORBIDDEN_WAREHOUSE", r.code())
	assert.Equal(t, 201, e.newLPB(f.headG, f.purchase(lpbItem(f.p2, f.a1, 1))).status(), "kepala gudang tanpa batas boleh di gudang mana pun")
	assert.Equal(t, 201, e.newLPB(f.admin, f.purchase(lpbItem(f.p2, f.a1, 1))).status())
}

// ================================================================ baca

func TestLPB_Get_NotFoundAndScoping(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id := idOf(e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1))))

	nf := e.get(f.staffA, lpbPath(999999, ""))
	assert.Equal(t, 404, nf.status())
	assert.Equal(t, "LPB_NOT_FOUND", nf.code())
	assert.Equal(t, 400, e.get(f.staffA, "/api/v1/lpb/abc").status())

	other := e.get(f.staffB, lpbPath(id, ""))
	assert.Equal(t, 403, other.status())
	assert.Equal(t, "FORBIDDEN_WAREHOUSE", other.code())
	assert.Equal(t, 200, e.get(f.mgmt, lpbPath(id, "")).status())
}

func TestLPB_List_FiltersSortAndScoping(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	l1 := e.mustLPB(f.staffA, map[string]any{"sub_type": "PURCHASE", "warehouse_id": f.whA, "supplier_id": f.supplier, "reference_number": "INV-777",
		"items": []map[string]any{lpbItem(f.p2, f.a1, 4), lpbItem(f.p1, f.a1, 2)}})
	l2 := e.mustLPB(f.staff2A, map[string]any{"sub_type": "INITIAL_STOCK", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(f.p2, f.a1, 7)}})
	l3 := e.mustLPB(f.headA, map[string]any{"sub_type": "DISPOSAL", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(f.p1, f.a2, 1)}})
	l4 := e.mustLPB(f.staffB, map[string]any{"sub_type": "INITIAL_STOCK", "warehouse_id": f.whB, "items": []map[string]any{lpbItem(f.p2, f.b1, 1)}})
	e.exec(`UPDATE lpb_documents SET status = 'CANCELLED' WHERE id = $1`, idOf(l2))
	e.exec(`UPDATE lpb_documents SET doc_date = '2026-01-15' WHERE id = $1`, idOf(l3))
	numbers := func(r *reply) []string { return names(r.list(), "lpb_number") }
	get := func(tok, q string) *reply { return e.get(tok, "/api/v1/lpb"+q) }

	all := get(f.admin, "")
	require.Equal(t, 200, all.status(), all.rec.Body.String())
	assert.EqualValues(t, 4, all.meta()["total"])
	assert.Equal(t, l4.str("lpb_number"), all.list()[0]["lpb_number"], "bawaan: terbaru dulu")
	first := row(all.list(), "lpb_number", l1.str("lpb_number"))
	assert.EqualValues(t, 2, first["item_count"])
	assert.EqualValues(t, 6, first["total_qty"])
	assert.Equal(t, "PT Maju Jaya", first["supplier_name"])

	assert.Equal(t, []string{l3.str("lpb_number")}, numbers(get(f.admin, "?type=OUTBOUND")))
	assert.ElementsMatch(t, []string{l2.str("lpb_number"), l4.str("lpb_number")}, numbers(get(f.admin, "?sub_type=INITIAL_STOCK")))
	assert.Equal(t, []string{l2.str("lpb_number")}, numbers(get(f.admin, "?status=CANCELLED")))
	assert.Equal(t, []string{l4.str("lpb_number")}, numbers(get(f.admin, fmt.Sprintf("?warehouse_id=%d", f.whB))))
	assert.Equal(t, []string{l3.str("lpb_number")}, numbers(get(f.admin, "?date_to=2026-02-01")))
	assert.NotContains(t, numbers(get(f.admin, "?date_from=2026-02-01")), l3.str("lpb_number"))
	assert.Equal(t, []string{l1.str("lpb_number")}, numbers(get(f.admin, "?q=inv-777")), "q: nomor referensi")
	assert.Equal(t, []string{l1.str("lpb_number")}, numbers(get(f.admin, "?q=maju")), "q: nama supplier")
	assert.Equal(t, []string{l4.str("lpb_number")}, numbers(get(f.admin, "?q="+url4(l4.str("lpb_number")))), "q: nomor LPB")
	assert.ElementsMatch(t, []string{l2.str("lpb_number")}, numbers(get(f.staff2A, "?mine=true")))

	asc := numbers(get(f.admin, "?sort=lpb_number"))
	assert.Equal(t, 4, len(asc))
	assert.Equal(t, 400, get(f.admin, "?sort=notes").status())
	assert.Equal(t, 400, get(f.admin, "?status=NGAWUR").status())
	assert.Equal(t, 400, get(f.admin, "?sub_type=NGAWUR").status())
	assert.Equal(t, 400, get(f.admin, "?date_from=kemarin").status())

	// Warehouse scoping.
	assert.ElementsMatch(t, []string{l1.str("lpb_number"), l2.str("lpb_number"), l3.str("lpb_number")}, numbers(get(f.staffA, "")))
	assert.Equal(t, []string{l4.str("lpb_number")}, numbers(get(f.staffB, "")))
	denied := get(f.staffA, fmt.Sprintf("?warehouse_id=%d", f.whB))
	assert.Equal(t, 403, denied.status())
	assert.Len(t, get(f.mgmt, "").list(), 4)
}

func url4(s string) string { return strings.ReplaceAll(s, "/", "%2F") }

// ================================================================ ubah

func TestLPB_Update_HeaderPatchAndAudit(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	sup2 := idOf(e.post(f.admin, "/api/v1/suppliers", map[string]any{"code": "SUP-2", "name": "CV Lain"}))
	r := e.mustLPB(f.staffA, map[string]any{"sub_type": "PURCHASE", "warehouse_id": f.whA, "supplier_id": f.supplier, "reference_number": "INV-1", "notes": "awal"})
	id := idOf(r)

	u := e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{
		"notes": nil, "reference_number": "INV-2", "supplier_id": sup2, "doc_date": "2026-01-02",
		"sub_type": "DISPOSAL", "warehouse_id": f.whB, // tidak dapat diubah
	})
	require.Equal(t, 200, u.status(), u.rec.Body.String())
	assert.Nil(t, u.data()["notes"], "null mengosongkan")
	assert.Equal(t, "INV-2", u.str("reference_number"))
	assert.Equal(t, "CV Lain", u.str("supplier_name"))
	assert.Equal(t, "2026-01-02", u.str("doc_date"))
	assert.Equal(t, "PURCHASE", u.str("sub_type"), "jenis tidak berubah")
	assert.Equal(t, "WH-A", u.str("warehouse_code"), "gudang tidak berubah")
	assert.Equal(t, []string{"LPB_CREATE", "LPB_UPDATE"}, timelineActions(u))
	assert.Equal(t, 1, e.auditCount("LPB_UPDATE", id))

	same := e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"reference_number": "INV-2"})
	assert.Equal(t, 200, same.status())
	assert.Equal(t, 1, e.auditCount("LPB_UPDATE", id), "tanpa perubahan: tidak ada audit baru")

	bad := e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"supplier_id": nil})
	assert.Equal(t, 422, bad.status())
	assert.Contains(t, fieldErrors(bad), "supplier_id", "pembelian tidak boleh tanpa supplier")
	assert.Equal(t, 422, e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"supplier_id": 999999}).status())
	future := time.Now().In(clock.WIB).AddDate(0, 0, 2).Format("2006-01-02")
	assert.Equal(t, 422, e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"doc_date": future}).status())
}

func TestLPB_Update_ItemsSyncKeepsScansAndGuardsQty(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	r := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 3), lpbItem(f.p2, f.a1, 5)))
	id := idOf(r)
	snItem := itemID(r, 0)
	require.Equal(t, 201, e.scan(f.staffA, id, snItem, "SN-1").status())
	require.Equal(t, 201, e.scan(f.staffA, id, snItem, "SN-2").status())

	// qty di bawah jumlah scan ditolak; yang sudah ter-scan tidak hilang.
	low := e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"items": []map[string]any{lpbItem(f.p1, f.a1, 1), lpbItem(f.p2, f.a1, 5)}})
	assert.Equal(t, 422, low.status())
	assert.Contains(t, fieldErrors(low), "items[0].qty")
	assert.EqualValues(t, 2, itemsOf(e.get(f.staffA, lpbPath(id, "")))[0]["scanned_count"])

	// ubah qty/biaya, tambah item, buang item lama → item dengan (produk,rak) sama tetap membawa scan-nya.
	u := e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"items": []map[string]any{
		lpbItem(f.p1, f.a1, 4, map[string]any{"unit_cost": "100"}),
		lpbItem(f.p3, f.a1, 1),
	}})
	require.Equal(t, 200, u.status(), u.rec.Body.String())
	items := itemsOf(u)
	require.Len(t, items, 2)
	p1 := row(items, "sku", "PRN-1")
	assert.EqualValues(t, 4, p1["qty"])
	assert.Equal(t, "100.00", p1["unit_cost"])
	assert.EqualValues(t, 2, p1["scanned_count"], "scan dipertahankan")
	assert.EqualValues(t, snItem, p1["id"], "baris item yang sama, bukan dibuat ulang")
	assert.Nil(t, row(items, "sku", "TN-1"), "item yang tidak dikirim dihapus")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM lpb_items WHERE lpb_id = $1 AND product_id = $2`, id, f.p2))

	// Membuang item ber-scan ikut menghapus scan-nya (cascade).
	e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"items": []map[string]any{lpbItem(f.p3, f.a1, 1)}})
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM lpb_item_serials`))

	// Daftar kosong menghapus semua item.
	empty := e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"items": []map[string]any{}})
	assert.Equal(t, 200, empty.status())
	assert.Len(t, itemsOf(empty), 0)

	assert.Equal(t, 3, e.auditCount("LPB_UPDATE", id), "satu audit per permintaan yang berhasil mengubah (yang ditolak 422 tidak mencatat)")
}

func TestLPB_Update_OwnershipAndState(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id := idOf(e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1))))

	other := e.patchReq(f.staff2A, lpbPath(id, ""), map[string]any{"notes": "x"})
	assert.Equal(t, 403, other.status(), "draft milik pembuatnya")
	assert.Equal(t, "FORBIDDEN", other.code())
	assert.Equal(t, 200, e.patchReq(f.headA, lpbPath(id, ""), map[string]any{"notes": "dibantu kepala gudang"}).status())
	assert.Equal(t, 200, e.patchReq(f.admin, lpbPath(id, ""), map[string]any{"notes": "admin"}).status())
	foreign := e.patchReq(f.staffB, lpbPath(id, ""), map[string]any{"notes": "x"})
	assert.Equal(t, 403, foreign.status())
	assert.Equal(t, "FORBIDDEN_WAREHOUSE", foreign.code(), "gudang lain: ditolak karena gudang, bukan kepemilikan")
	assert.Equal(t, 404, e.patchReq(f.staffA, lpbPath(999999, ""), map[string]any{"notes": "x"}).status())

	e.exec(`UPDATE lpb_documents SET status = 'APPROVED' WHERE id = $1`, id)
	locked := e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"notes": "x"})
	assert.Equal(t, 409, locked.status())
	assert.Equal(t, "INVALID_STATE", locked.code())
}

// ================================================================ scan SN

func TestLPB_Scan_Purchase_Flow(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	r := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 2), lpbItem(f.p2, f.a1, 5)))
	id, snItem, nonSN := idOf(r), itemID(r, 0), itemID(r, 1)

	first := e.scan(f.staffA, id, snItem, "  sn-abc-1 ")
	require.Equal(t, 201, first.status(), first.rec.Body.String())
	assert.Equal(t, "SN-ABC-1", first.str("serial_number"), "huruf besar & dipangkas")
	assert.Equal(t, "GOOD", first.str("condition"))
	assert.EqualValues(t, 1, first.field("scanned_count"))
	assert.EqualValues(t, 2, first.field("qty"))
	assert.EqualValues(t, f.staffAID, first.field("scanned_by"))

	// duplikat pada LPB yang sama
	dup := e.scan(f.staffA, id, snItem, "SN-ABC-1")
	assert.Equal(t, 409, dup.status())
	assert.Equal(t, "DUPLICATE_VALUE", dup.code())
	assert.Contains(t, fieldErrors(dup), "serial_number")

	second := e.scan(f.staffA, id, snItem, "SN-ABC-2", map[string]any{"condition": "MINOR_DEFECT"})
	assert.Equal(t, 201, second.status())
	assert.Equal(t, "MINOR_DEFECT", second.str("condition"))

	over := e.scan(f.staffA, id, snItem, "SN-ABC-3")
	assert.Equal(t, 422, over.status(), "melebihi qty")
	assert.Equal(t, "SERIAL_COUNT_MISMATCH", over.code())

	nonSNScan := e.scan(f.staffA, id, nonSN, "SN-X")
	assert.Equal(t, 422, nonSNScan.status(), "produk tanpa SN")
	assert.Contains(t, fieldErrors(nonSNScan), "serial_number")

	assert.Equal(t, 400, e.scan(f.staffA, id, snItem, "SN-9", map[string]any{"condition": "RUSAK"}).status())
	assert.Equal(t, 400, e.scan(f.staffA, id, snItem, "").status())
	assert.Equal(t, 422, e.scan(f.staffA, id, snItem, "AB\x01C").status(), "karakter kontrol ditolak")
	assert.Equal(t, 404, e.scan(f.staffA, id, 999999, "SN-Z").status(), "item tidak ada")

	d := e.get(f.staffA, lpbPath(id, ""))
	serials := itemsOf(d)[0]["serials"].([]any)
	require.Len(t, serials, 2)
	assert.EqualValues(t, 2, itemsOf(d)[0]["scanned_count"])

	// hapus & scan ulang
	rowID := int64(serials[0].(map[string]any)["id"].(float64))
	del := e.call("DELETE", fmt.Sprintf("/api/v1/lpb/%d/items/%d/serials/%d", id, snItem, rowID), nil, withBearer(f.staffA))
	assert.Equal(t, 200, del.status(), del.rec.Body.String())
	assert.Equal(t, 404, e.call("DELETE", fmt.Sprintf("/api/v1/lpb/%d/items/%d/serials/%d", id, snItem, rowID), nil, withBearer(f.staffA)).status(), "sudah terhapus")
	assert.Equal(t, 404, e.call("DELETE", fmt.Sprintf("/api/v1/lpb/%d/items/%d/serials/999999", id, snItem), nil, withBearer(f.staffA)).status())
	assert.Equal(t, 201, e.scan(f.staffA, id, snItem, "SN-ABC-1").status(), "SN yang dihapus dapat dipindai lagi")

	// Pemindaian tidak menyentuh stok maupun daftar unit.
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM product_serials`))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM stock_balances`))
}

func TestLPB_Scan_RulesAgainstExistingData(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	e.addSerial(f.p1, "AST-2611-00001", "SN-EXIST", "AVAILABLE", &f.a1, nil)

	r1 := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 2), lpbItem(f.p1, f.a2, 1)))
	id1, sellable, quarantine := idOf(r1), itemID(r1, 0), itemID(r1, 1)

	exists := e.scan(f.staffA, id1, sellable, "sn-exist")
	assert.Equal(t, 409, exists.status(), "SN sudah terdaftar (tanpa peduli huruf)")
	assert.Equal(t, "SERIAL_EXISTS", exists.code())

	// Produk lain boleh memakai SN yang sama (SN unik per produk).
	other := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p3, f.a1, 1)))
	assert.Equal(t, 201, e.scan(f.staffA, idOf(other), itemID(other, 0), "SN-EXIST").status())

	// Unit rusak hanya di rak non-sellable.
	broken := e.scan(f.staffA, id1, sellable, "SN-BRK", map[string]any{"condition": "BROKEN"})
	assert.Equal(t, 422, broken.status())
	assert.Contains(t, fieldErrors(broken), "condition")
	assert.Equal(t, 201, e.scan(f.staffA, id1, quarantine, "SN-BRK", map[string]any{"condition": "BROKEN"}).status())

	// Dipakai LPB draft lain untuk produk yang sama.
	r2 := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 1)))
	clash := e.scan(f.staffA, idOf(r2), itemID(r2, 0), "SN-BRK")
	assert.Equal(t, 409, clash.status())
	assert.Equal(t, "SERIAL_IN_OTHER_LPB", clash.code())
	assert.Contains(t, clash.str("message")+clash.rec.Body.String(), r1.str("lpb_number"), "menyebut LPB yang memakainya")

	// Setelah LPB pertama dibatalkan, SN bebas lagi.
	require.Equal(t, 200, e.post(f.staffA, lpbPath(id1, "/cancel"), nil).status())
	assert.Equal(t, 201, e.scan(f.staffA, idOf(r2), itemID(r2, 0), "SN-BRK").status())
}

func TestLPB_Scan_Permissions_StateAndRackLock(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	r := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 3)))
	id, item := idOf(r), itemID(r, 0)

	assert.Equal(t, 403, e.scan(f.staff2A, id, item, "SN-1").status(), "bukan pemilik draft")
	assert.Equal(t, 403, e.scan(f.staffB, id, item, "SN-1").status(), "gudang lain")
	assert.Equal(t, 403, e.scan(f.mgmt, id, item, "SN-1").status(), "manajemen hanya membaca")
	assert.Equal(t, 201, e.scan(f.headA, id, item, "SN-1").status(), "kepala gudang boleh membantu")

	e.lockRack(f.a1, f.whA)
	locked := e.scan(f.staffA, id, item, "SN-2")
	assert.Equal(t, 409, locked.status())
	assert.Equal(t, "RACK_LOCKED_BY_OPNAME", locked.code())
	e.exec(`UPDATE racks SET locked_by_opname_id = NULL WHERE id = $1`, f.a1)

	e.exec(`UPDATE lpb_documents SET status = 'PENDING_APPROVAL' WHERE id = $1`, id)
	pending := e.scan(f.staffA, id, item, "SN-2")
	assert.Equal(t, 409, pending.status())
	assert.Equal(t, "INVALID_STATE", pending.code())
	del := e.call("DELETE", fmt.Sprintf("/api/v1/lpb/%d/items/%d/serials/1", id, item), nil, withBearer(f.staffA))
	assert.Equal(t, 409, del.status(), "tidak dapat menghapus scan setelah diajukan")
}

func TestLPB_Scan_ConcurrentSameSerialAcrossDrafts_OnlyOneWins(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)

	const n = 6
	type target struct{ lpb, item int64 }
	targets := make([]target, n)
	for i := range targets {
		r := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 1)))
		targets[i] = target{idOf(r), itemID(r, 0)}
	}

	var wg sync.WaitGroup
	statuses := make(chan int, n)
	start := make(chan struct{})
	for _, tg := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			statuses <- e.scan(f.staffA, tg.lpb, tg.item, "SN-RACE").status()
		}()
	}
	close(start)
	wg.Wait()
	close(statuses)

	created, conflicts := 0, 0
	for s := range statuses {
		switch s {
		case 201:
			created++
		case 409:
			conflicts++
		}
	}
	assert.Equal(t, 1, created, "tepat satu draft yang berhasil memegang SN")
	assert.Equal(t, n-1, conflicts)
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM lpb_item_serials WHERE serial_number = 'SN-RACE'`))
}

// ================================================================ disposal

func TestLPB_Disposal_ScanRegisteredUnits(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	broken := e.addSerial(f.p1, "AST-2611-00010", "BRK-1", "BROKEN", &f.a2, nil)
	e.addSerial(f.p1, "AST-2611-00011", "OK-1", "AVAILABLE", &f.a2, nil)
	e.addSerial(f.p1, "AST-2611-00012", "BRK-B", "BROKEN", &f.b1, nil)
	e.addSerial(f.p3, "AST-2611-00013", "BRK-P3", "BROKEN", &f.a2, nil)
	e.addSerial(f.p1, "AST-2611-00014", "BRK-2", "BROKEN", &f.a2, nil)

	r := e.mustLPB(f.staffA, map[string]any{"sub_type": "DISPOSAL", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(f.p1, f.a2, 2)}})
	id, item := idOf(r), itemID(r, 0)

	byTag := e.scan(f.staffA, id, item, "ast-2611-00010")
	require.Equal(t, 201, byTag.status(), byTag.rec.Body.String())
	assert.Equal(t, "BRK-1", byTag.str("serial_number"), "SN diambil dari unit terdaftar")
	assert.Equal(t, "AST-2611-00010", byTag.str("asset_tag"))
	assert.EqualValues(t, broken, byTag.field("product_serial_id"))

	bySN := e.scan(f.staffA, id, item, "brk-2")
	assert.Equal(t, 201, bySN.status(), "boleh scan serial number pabrikan")

	cases := []struct{ name, code string }{
		{"tidak ditemukan", "TIDAK-ADA"},
		{"produk lain", "BRK-P3"},
	}
	r2 := e.mustLPB(f.staffA, map[string]any{"sub_type": "DISPOSAL", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(f.p1, f.a2, 3)}})
	id2, item2 := idOf(r2), itemID(r2, 0)
	for _, tc := range cases {
		x := e.scan(f.staffA, id2, item2, tc.code)
		assert.Equal(t, 422, x.status(), tc.name)
		assert.Contains(t, fieldErrors(x), "serial_number", tc.name)
	}
	notBroken := e.scan(f.staffA, id2, item2, "OK-1")
	assert.Equal(t, 422, notBroken.status())
	assert.Equal(t, "SERIAL_NOT_VALID", notBroken.code(), "hanya unit BROKEN yang dapat di-disposal")
	wrongRack := e.scan(f.staffA, id2, item2, "BRK-B")
	assert.Equal(t, 422, wrongRack.status())
	assert.Equal(t, "SERIAL_NOT_VALID", wrongRack.code(), "unit harus di rak item")

	// Unit yang sudah dipakai draft lain.
	claimed := e.scan(f.staffA, id2, item2, "AST-2611-00010")
	assert.Equal(t, 409, claimed.status())
	assert.Equal(t, "SERIAL_IN_OTHER_LPB", claimed.code())

	// Status unit tidak berubah oleh draft.
	assert.Equal(t, "BROKEN", func() string {
		var s string
		require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT status FROM product_serials WHERE id = $1`, broken).Scan(&s))
		return s
	}())
}

// ================================================================ submit / cancel / revise

func TestLPB_Submit_Success_AuditAndLock(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	r := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 2), lpbItem(f.p2, f.a1, 5)))
	id, snItem := idOf(r), itemID(r, 0)
	e.scan(f.staffA, id, snItem, "SN-1")
	e.scan(f.staffA, id, snItem, "SN-2")

	s := e.post(f.staffA, lpbPath(id, "/submit"), nil)
	require.Equal(t, 200, s.status(), s.rec.Body.String())
	assert.Equal(t, "PENDING_APPROVAL", s.str("status"))
	assert.EqualValues(t, f.staffAID, s.data()["submitted_by"])
	assert.NotNil(t, s.data()["submitted_at"])
	assert.Equal(t, []string{"LPB_CREATE", "LPB_SUBMIT"}, timelineActions(s))

	// Setelah diajukan: dokumen terkunci untuk draft-aksi.
	for name, resp := range map[string]*reply{
		"patch":  e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"notes": "x"}),
		"submit": e.post(f.staffA, lpbPath(id, "/submit"), nil),
		"cancel": e.post(f.staffA, lpbPath(id, "/cancel"), nil),
		"revise": e.post(f.staffA, lpbPath(id, "/revise"), nil),
		"scan":   e.scan(f.staffA, id, snItem, "SN-3"),
	} {
		assert.Equal(t, 409, resp.status(), name)
		assert.Equal(t, "INVALID_STATE", resp.code(), name)
	}

	// Invarian #1: submit tidak menyentuh stok maupun unit.
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM stock_balances`))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM product_serials`))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM stock_ledgers`))
}

func TestLPB_Submit_Validation(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)

	empty := idOf(e.mustLPB(f.staffA, f.purchase()))
	r := e.post(f.staffA, lpbPath(empty, "/submit"), nil)
	assert.Equal(t, 422, r.status())
	assert.Equal(t, "LPB_EMPTY", r.code())

	short := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 3)))
	e.scan(f.staffA, idOf(short), itemID(short, 0), "SN-1")
	mismatch := e.post(f.staffA, lpbPath(idOf(short), "/submit"), nil)
	assert.Equal(t, 422, mismatch.status())
	assert.Equal(t, "SERIAL_COUNT_MISMATCH", mismatch.code())
	assert.Contains(t, mismatch.rec.Body.String(), "ter-scan 1 dari 3")
	assert.Equal(t, "DRAFT", e.get(f.staffA, lpbPath(idOf(short), "")).str("status"), "gagal submit tidak mengubah status")

	// Non-SN keluar: peringatan dini stok tidak cukup.
	cons := e.mustLPB(f.staffA, map[string]any{"sub_type": "DISPOSAL", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(f.p2, f.a1, 5)}})
	low := e.post(f.staffA, lpbPath(idOf(cons), "/submit"), nil)
	assert.Equal(t, 409, low.status())
	assert.Equal(t, "INSUFFICIENT_STOCK", low.code())
	assert.Contains(t, low.rec.Body.String(), "tersedia 0, diminta 5")
	e.exec(`INSERT INTO stock_balances (product_id, rack_id, qty_on_hand, qty_reserved) VALUES ($1, $2, 6, 2)`, f.p2, f.a1)
	still := e.post(f.staffA, lpbPath(idOf(cons), "/submit"), nil)
	assert.Equal(t, 409, still.status(), "tersedia = 6 − 2 reserved = 4 < 5")
	e.exec(`UPDATE stock_balances SET qty_reserved = 1`)
	assert.Equal(t, 200, e.post(f.staffA, lpbPath(idOf(cons), "/submit"), nil).status())

	assert.Equal(t, 403, e.post(f.mgmt, lpbPath(empty, "/submit"), nil).status())
	assert.Equal(t, 403, e.post(f.staff2A, lpbPath(idOf(short), "/submit"), nil).status(), "bukan pemilik")
	assert.Equal(t, 404, e.post(f.staffA, lpbPath(999999, "/submit"), nil).status())
}

func TestLPB_Submit_RevalidatesAfterDataChanged(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)

	// SN baru didaftarkan pihak lain setelah di-scan.
	r := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 1)))
	e.scan(f.staffA, idOf(r), itemID(r, 0), "SN-LATE")
	e.addSerial(f.p1, "AST-2611-00001", "SN-LATE", "AVAILABLE", &f.a1, nil)
	late := e.post(f.staffA, lpbPath(idOf(r), "/submit"), nil)
	assert.Equal(t, 409, late.status())
	assert.Equal(t, "SERIAL_EXISTS", late.code())

	// Rak dikunci opname setelah draft dibuat.
	r2 := e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1)))
	e.lockRack(f.a1, f.whA)
	locked := e.post(f.staffA, lpbPath(idOf(r2), "/submit"), nil)
	assert.Equal(t, 409, locked.status())
	assert.Equal(t, "RACK_LOCKED_BY_OPNAME", locked.code())
	e.exec(`UPDATE racks SET locked_by_opname_id = NULL WHERE id = $1`, f.a1)

	// Produk dinonaktifkan setelah draft dibuat.
	e.exec(`UPDATE products SET is_active = FALSE WHERE id = $1`, f.p2)
	inactive := e.post(f.staffA, lpbPath(idOf(r2), "/submit"), nil)
	assert.Equal(t, 422, inactive.status())
	assert.Contains(t, fieldErrors(inactive), "items[0].product_id")

	// Disposal: unit tidak lagi BROKEN saat submit.
	e.addSerial(f.p3, "AST-2611-00020", "BRK-X", "BROKEN", &f.a2, nil)
	d := e.mustLPB(f.staffA, map[string]any{"sub_type": "DISPOSAL", "warehouse_id": f.whA, "items": []map[string]any{lpbItem(f.p3, f.a2, 1)}})
	require.Equal(t, 201, e.scan(f.staffA, idOf(d), itemID(d, 0), "BRK-X").status())
	e.exec(`UPDATE product_serials SET status = 'MAINTENANCE' WHERE asset_tag = 'AST-2611-00020'`)
	notBroken := e.post(f.staffA, lpbPath(idOf(d), "/submit"), nil)
	assert.Equal(t, 422, notBroken.status())
	assert.Equal(t, "SERIAL_NOT_VALID", notBroken.code())
}

func TestLPB_Submit_ConcurrentDoubleClick_OnlyOneSucceeds(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id := idOf(e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1))))

	var wg sync.WaitGroup
	res := make(chan int, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res <- e.post(f.staffA, lpbPath(id, "/submit"), nil).status()
		}()
	}
	wg.Wait()
	close(res)
	ok, conflict := 0, 0
	for s := range res {
		switch s {
		case 200:
			ok++
		case 409:
			conflict++
		}
	}
	assert.Equal(t, 1, ok)
	assert.Equal(t, 5, conflict)
	assert.Equal(t, 1, e.auditCount("LPB_SUBMIT", id), "satu audit saja")
}

func TestLPB_Cancel(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id := idOf(e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1))))

	assert.Equal(t, 403, e.post(f.staff2A, lpbPath(id, "/cancel"), nil).status(), "bukan pemilik")
	c := e.post(f.staffA, lpbPath(id, "/cancel"), nil)
	require.Equal(t, 200, c.status())
	assert.Equal(t, "CANCELLED", c.str("status"))
	assert.Equal(t, []string{"LPB_CREATE", "LPB_CANCEL"}, timelineActions(c))
	again := e.post(f.staffA, lpbPath(id, "/cancel"), nil)
	assert.Equal(t, 409, again.status())
	assert.Equal(t, "INVALID_STATE", again.code())
	assert.Equal(t, 409, e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"notes": "x"}).status())
	assert.Equal(t, 409, e.post(f.staffA, lpbPath(id, "/submit"), nil).status())

	// Yang sudah diajukan tidak dapat dibatalkan pembuatnya.
	pending := idOf(e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1))))
	require.Equal(t, 200, e.post(f.staffA, lpbPath(pending, "/submit"), nil).status())
	assert.Equal(t, 409, e.post(f.staffA, lpbPath(pending, "/cancel"), nil).status())
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM lpb_documents WHERE status = 'APPROVED'`))
}

func TestLPB_Revise_RejectedBackToDraft(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id := idOf(e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1))))

	assert.Equal(t, 409, e.post(f.staffA, lpbPath(id, "/revise"), nil).status(), "hanya REJECTED yang dapat direvisi")

	e.exec(`UPDATE lpb_documents SET status = 'REJECTED', rejected_by = $2, rejected_at = NOW(), rejection_reason = 'qty salah',
	        submitted_by = $3, submitted_at = NOW() WHERE id = $1`, id, f.headAID, f.staffAID)
	assert.Equal(t, 403, e.post(f.staff2A, lpbPath(id, "/revise"), nil).status(), "bukan pemilik")
	rv := e.post(f.staffA, lpbPath(id, "/revise"), nil)
	require.Equal(t, 200, rv.status(), rv.rec.Body.String())
	assert.Equal(t, "DRAFT", rv.str("status"))
	assert.Equal(t, "qty salah", rv.str("rejection_reason"), "alasan tetap terlihat saat merevisi")
	assert.Nil(t, rv.data()["submitted_at"])
	assert.Equal(t, 1, e.auditCount("LPB_REVISE", id))

	// Dapat diedit lagi, lalu diajukan ulang: alasan penolakan dibersihkan.
	assert.Equal(t, 200, e.patchReq(f.staffA, lpbPath(id, ""), map[string]any{"notes": "diperbaiki"}).status())
	re := e.post(f.staffA, lpbPath(id, "/submit"), nil)
	require.Equal(t, 200, re.status())
	assert.Nil(t, re.data()["rejection_reason"])
	assert.Nil(t, re.data()["rejected_at"])
}

func TestLPB_Timeline_OrderedWithActors(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	id := idOf(e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 1))))
	e.patchReq(f.headA, lpbPath(id, ""), map[string]any{"notes": "cek"})
	e.post(f.staffA, lpbPath(id, "/submit"), nil)

	d := e.get(f.mgmt, lpbPath(id, ""))
	assert.Equal(t, []string{"LPB_CREATE", "LPB_UPDATE", "LPB_SUBMIT"}, timelineActions(d))
	tl := d.data()["timeline"].([]any)
	assert.EqualValues(t, f.staffAID, tl[0].(map[string]any)["user_id"])
	assert.EqualValues(t, f.headAID, tl[1].(map[string]any)["user_id"], "kepala gudang yang mengubah")
	assert.EqualValues(t, f.staffAID, tl[2].(map[string]any)["user_id"])
}
