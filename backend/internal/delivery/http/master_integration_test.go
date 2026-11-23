//go:build integration

package http_test

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/jwt"
)

// ---- helpers ----

var userSeq atomic.Int64

// tokenFor membuat user ber-role (opsional dibatasi ke satu gudang) dan mengembalikan access token.
func (e *env) tokenFor(role domain.Role, warehouseID *int64) string {
	e.t.Helper()
	email := fmt.Sprintf("%s-%d@x.id", strings.ToLower(string(role)), userSeq.Add(1))
	id := e.addUser(email, role)
	if warehouseID != nil {
		e.exec(`UPDATE users SET warehouse_id = $2 WHERE id = $1`, id, *warehouseID)
	}
	return e.mint(id, role, warehouseID)
}

// mint menerbitkan access token langsung (tanpa login bcrypt cost 12 yang lambat, apalagi dengan -race).
// Alur login sendiri diuji di auth_integration_test.go.
func (e *env) mint(userID int64, role domain.Role, warehouseID *int64) string {
	e.t.Helper()
	m, err := jwt.NewManager(e.cfg.JWTAccessSecret, e.cfg.JWTAccessTTL)
	require.NoError(e.t, err)
	tok, _, err := m.Issue(jwt.Claims{UserID: userID, Role: string(role), WarehouseID: warehouseID}, e.clk.Now())
	require.NoError(e.t, err)
	return tok
}

func (r *reply) list() []map[string]any {
	raw, _ := r.body["data"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, x := range raw {
		out = append(out, x.(map[string]any))
	}
	return out
}

func (r *reply) meta() map[string]any { m, _ := r.body["meta"].(map[string]any); return m }

func (r *reply) field(key string) any { return r.data()[key] }

func names(items []map[string]any, key string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, fmt.Sprint(it[key]))
	}
	return out
}

func (e *env) post(token, path string, body any) *reply {
	e.t.Helper()
	return e.call("POST", path, body, withBearer(token))
}
func (e *env) patchReq(token, path string, body any) *reply {
	e.t.Helper()
	return e.call("PATCH", path, body, withBearer(token))
}
func (e *env) get(token, path string) *reply {
	e.t.Helper()
	return e.call("GET", path, nil, withBearer(token))
}

func (e *env) newWarehouse(admin, code string) int64 {
	e.t.Helper()
	r := e.post(admin, "/api/v1/warehouses", map[string]any{"code": code, "name": "Gudang " + code})
	require.Equal(e.t, 201, r.status(), r.rec.Body.String())
	return idOf(r)
}

func (e *env) newRack(token string, warehouseID int64, code string, extra map[string]any) *reply {
	e.t.Helper()
	body := map[string]any{"warehouse_id": warehouseID, "code": code}
	for k, v := range extra {
		body[k] = v
	}
	return e.post(token, "/api/v1/racks", body)
}

func (e *env) newCategory(admin, name, itemType string) int64 {
	e.t.Helper()
	r := e.post(admin, "/api/v1/categories", map[string]any{"name": name, "item_type": itemType})
	require.Equal(e.t, 201, r.status(), r.rec.Body.String())
	return idOf(r)
}

func (e *env) newProduct(admin string, categoryID int64, sku string, extra map[string]any) *reply {
	e.t.Helper()
	body := map[string]any{"sku": sku, "name": "Produk " + sku, "category_id": categoryID}
	for k, v := range extra {
		body[k] = v
	}
	return e.post(admin, "/api/v1/products", body)
}

func (e *env) seedStock(productID, rackID int64, qty int) {
	e.t.Helper()
	e.exec(`INSERT INTO stock_balances (product_id, rack_id, qty_on_hand) VALUES ($1, $2, $3)`, productID, rackID, qty)
}

// ================================================================ RBAC (matriks 5.1)

func TestMaster_RBACMatrix(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.newWarehouse(admin, "WH-A")
	rack := idOf(e.newRack(admin, wh, "A-01", nil))
	cat := e.newCategory(admin, "Printer", "ASSET")
	prod := idOf(e.newProduct(admin, cat, "PRN-1", nil))
	sup := idOf(e.post(admin, "/api/v1/suppliers", map[string]any{"code": "SUP-1", "name": "Supplier 1"}))

	type resource struct {
		name, base string
		id         int64
		read       []domain.Role // selain ADMIN
		write      []domain.Role
	}
	var (
		m, b, s, h, d, tk = domain.RoleManagement, domain.RoleBusinessAdmin, domain.RoleStaffWarehouse,
			domain.RoleHeadWarehouse, domain.RoleDriver, domain.RoleTechnician
	)
	resources := []resource{
		{"warehouses", "/api/v1/warehouses", wh, []domain.Role{m, b, s, h}, []domain.Role{h}},
		{"racks", "/api/v1/racks", rack, []domain.Role{m, b, s, h}, []domain.Role{s, h}},
		{"categories", "/api/v1/categories", cat, []domain.Role{m, b, s, h}, []domain.Role{h}},
		{"products", "/api/v1/products", prod, []domain.Role{m, b, s, h, tk}, []domain.Role{h}},
		{"suppliers", "/api/v1/suppliers", sup, []domain.Role{m, b, s, h}, []domain.Role{h}},
	}
	has := func(list []domain.Role, r domain.Role) bool {
		for _, x := range list {
			if x == r {
				return true
			}
		}
		return false
	}

	for _, role := range []domain.Role{m, b, s, h, d, tk} {
		token := e.tokenFor(role, nil)
		for _, res := range resources {
			single := fmt.Sprintf("%s/%d", res.base, res.id)
			for _, path := range []string{res.base, single} {
				got := e.get(token, path).status()
				if has(res.read, role) {
					assert.Equal(t, 200, got, "%s GET %s", role, path)
				} else {
					assert.Equal(t, 403, got, "%s GET %s", role, path)
				}
			}
			// Tulis: body kosong. Yang berhak lolos RBAC lalu gagal validasi (400/422), bukan 403.
			for _, call := range []struct{ method, path string }{{"POST", res.base}, {"PATCH", single}} {
				r := e.call(call.method, call.path, map[string]any{}, withBearer(token))
				if has(res.write, role) {
					assert.NotEqual(t, 403, r.status(), "%s %s %s", role, call.method, call.path)
					assert.NotEqual(t, 401, r.status(), "%s %s %s", role, call.method, call.path)
				} else {
					assert.Equal(t, 403, r.status(), "%s %s %s", role, call.method, call.path)
					assert.Equal(t, "FORBIDDEN", r.code())
				}
			}
		}
	}

	for _, res := range resources {
		assert.Equal(t, 401, e.call("GET", res.base, nil).status(), "tanpa token: GET %s", res.base)
		assert.Equal(t, 401, e.call("POST", res.base, map[string]any{}).status(), "tanpa token: POST %s", res.base)
	}
}

// ================================================================ warehouses

func TestWarehouses_Create_NormalizesAndAudits(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")

	r := e.post(admin, "/api/v1/warehouses", map[string]any{
		"code": " wh-smg-01 ", "name": "  Gudang Semarang ", "address": " Jl. Industri 1 ", "phone": " 024-123 ",
		"latitude": -6.9666667, "longitude": 110.4166667,
	})
	require.Equal(t, 201, r.status(), r.rec.Body.String())
	assert.Equal(t, "WH-SMG-01", r.str("code"), "kode huruf besar & trim")
	assert.Equal(t, "Gudang Semarang", r.str("name"))
	assert.Equal(t, "Jl. Industri 1", r.str("address"))
	assert.Equal(t, true, r.field("is_active"))
	assert.InDelta(t, -6.9666667, r.field("latitude").(float64), 1e-7)
	assert.InDelta(t, 110.4166667, r.field("longitude").(float64), 1e-7)

	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action='WAREHOUSE_CREATE' AND entity_type='warehouse' AND entity_id=$1`, idOf(r)))
}

func TestWarehouses_Create_Validation(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")

	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"kode berisi garis miring", map[string]any{"code": "WH/01", "name": "x"}, "code"},
		{"kode berisi spasi", map[string]any{"code": "WH 01", "name": "x"}, "code"},
		{"kode diawali tanda hubung", map[string]any{"code": "-WH", "name": "x"}, "code"},
		{"nama hanya spasi", map[string]any{"code": "WH-1", "name": "   "}, "name"},
		{"latitude tanpa longitude", map[string]any{"code": "WH-1", "name": "x", "latitude": -6.9}, "latitude"},
		{"latitude di luar rentang", map[string]any{"code": "WH-1", "name": "x", "latitude": 91.0, "longitude": 10.0}, "latitude"},
		{"longitude di luar rentang", map[string]any{"code": "WH-1", "name": "x", "latitude": 1.0, "longitude": 181.0}, "longitude"},
	}
	for _, tc := range cases {
		r := e.post(admin, "/api/v1/warehouses", tc.body)
		assert.Contains(t, []int{400, 422}, r.status(), tc.name)
		assert.Contains(t, fieldErrors(r), tc.field, tc.name+": "+r.rec.Body.String())
	}
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM warehouses`), "tidak ada yang tersimpan")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM audit_logs WHERE action LIKE 'WAREHOUSE_%'`))
}

func TestWarehouses_DuplicateCode_409WithField(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	e.newWarehouse(admin, "WH-1")

	r := e.post(admin, "/api/v1/warehouses", map[string]any{"code": "wh-1", "name": "Lain"})
	assert.Equal(t, 409, r.status())
	assert.Equal(t, "DUPLICATE_VALUE", r.code())
	assert.Contains(t, fieldErrors(r), "code")
}

func TestWarehouses_Update_PatchSemanticsCodeImmutableAndAudit(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	r := e.post(admin, "/api/v1/warehouses", map[string]any{
		"code": "WH-1", "name": "Awal", "address": "Alamat", "phone": "123", "latitude": 1.5, "longitude": 2.5,
	})
	id := idOf(r)
	url := fmt.Sprintf("/api/v1/warehouses/%d", id)

	u := e.patchReq(admin, url, map[string]any{"name": "Baru", "phone": nil, "code": "HACK-9", "latitude": nil, "longitude": nil})
	require.Equal(t, 200, u.status(), u.rec.Body.String())
	assert.Equal(t, "Baru", u.str("name"))
	assert.Nil(t, u.field("phone"), "null mengosongkan")
	assert.Equal(t, "Alamat", u.str("address"), "yang tidak dikirim tidak berubah")
	assert.Equal(t, "WH-1", u.str("code"), "kode tidak dapat diubah")
	assert.Nil(t, u.field("latitude"))
	assert.Nil(t, u.field("longitude"))

	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action='WAREHOUSE_UPDATE' AND entity_id=$1`, id))

	// Tanpa perubahan: tidak ada tulisan dan tidak ada audit baru.
	same := e.patchReq(admin, url, map[string]any{"name": "Baru"})
	assert.Equal(t, 200, same.status())
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action='WAREHOUSE_UPDATE' AND entity_id=$1`, id))

	// Menonaktifkan & mengaktifkan kembali → entri audit tersendiri.
	assert.Equal(t, 200, e.patchReq(admin, url, map[string]any{"is_active": false}).status())
	assert.Equal(t, 200, e.patchReq(admin, url, map[string]any{"is_active": true}).status())
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action='WAREHOUSE_DEACTIVATE' AND entity_id=$1`, id))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action='WAREHOUSE_ACTIVATE' AND entity_id=$1`, id))
}

func TestWarehouses_Deactivate_BlockedWhileHoldingStock(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.newWarehouse(admin, "WH-1")
	rack := idOf(e.newRack(admin, wh, "A-01", nil))
	cat := e.newCategory(admin, "Toner", "CONSUMABLE")
	prod := idOf(e.newProduct(admin, cat, "TN-1", nil))
	e.seedStock(prod, rack, 5)
	url := fmt.Sprintf("/api/v1/warehouses/%d", wh)

	r := e.patchReq(admin, url, map[string]any{"is_active": false})
	assert.Equal(t, 409, r.status())
	assert.Equal(t, "LOCATION_HAS_STOCK", r.code())
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM warehouses WHERE id=$1 AND is_active`, wh), "tetap aktif")

	e.exec(`UPDATE stock_balances SET qty_on_hand = 0`)
	assert.Equal(t, 200, e.patchReq(admin, url, map[string]any{"is_active": false}).status(), "kosong → boleh dinonaktifkan")
}

func TestWarehouses_List_FilterSortPaginate(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	for _, code := range []string{"WH-C", "WH-A", "WH-B"} {
		e.newWarehouse(admin, code)
	}
	e.exec(`UPDATE warehouses SET is_active = FALSE WHERE code = 'WH-B'`)

	all := e.get(admin, "/api/v1/warehouses")
	assert.Equal(t, []string{"WH-A", "WH-B", "WH-C"}, names(all.list(), "code"), "default urut kode")
	assert.EqualValues(t, 3, all.meta()["total"])

	desc := e.get(admin, "/api/v1/warehouses?sort=-code")
	assert.Equal(t, []string{"WH-C", "WH-B", "WH-A"}, names(desc.list(), "code"))

	active := e.get(admin, "/api/v1/warehouses?is_active=true")
	assert.Equal(t, []string{"WH-A", "WH-C"}, names(active.list(), "code"))

	page := e.get(admin, "/api/v1/warehouses?limit=2&page=2")
	assert.Equal(t, []string{"WH-C"}, names(page.list(), "code"))
	assert.EqualValues(t, 2, page.meta()["total_pages"])

	q := e.get(admin, "/api/v1/warehouses?q=wh-b")
	assert.Equal(t, []string{"WH-B"}, names(q.list(), "code"), "pencarian tidak peka huruf besar/kecil")

	assert.Equal(t, 400, e.get(admin, "/api/v1/warehouses?sort=password").status(), "kolom di luar whitelist ditolak")
	assert.Equal(t, 400, e.get(admin, "/api/v1/warehouses?sort=code%3BDROP%20TABLE%20warehouses").status())
	assert.Equal(t, 3, e.count(`SELECT count(*) FROM warehouses`))
}

func TestWarehouses_Search_WildcardsAreLiteral(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	e.newWarehouse(admin, "WH-1")
	e.newWarehouse(admin, "WH-2")

	for _, q := range []string{"%25", "_", "%25WH%25"} { // %, _ , %WH% (URL-encoded)
		r := e.get(admin, "/api/v1/warehouses?q="+q)
		assert.Equal(t, 200, r.status())
		assert.Empty(t, r.list(), "wildcard dari pengguna harus dianggap karakter biasa: q=%s", q)
	}
}

func TestWarehouses_GetNotFoundAndBadID(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	assert.Equal(t, 404, e.get(admin, "/api/v1/warehouses/999999").status())
	assert.Equal(t, 400, e.get(admin, "/api/v1/warehouses/abc").status())
	assert.Equal(t, 404, e.patchReq(admin, "/api/v1/warehouses/999999", map[string]any{"name": "x"}).status())
}

func TestWarehouses_Scoping_HeadOfOneWarehouse(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	whA := e.newWarehouse(admin, "WH-A")
	whB := e.newWarehouse(admin, "WH-B")
	head := e.tokenFor(domain.RoleHeadWarehouse, &whA)

	list := e.get(head, "/api/v1/warehouses")
	assert.Equal(t, []string{"WH-A"}, names(list.list(), "code"), "hanya gudang sendiri")

	assert.Equal(t, 200, e.get(head, fmt.Sprintf("/api/v1/warehouses/%d", whA)).status())
	other := e.get(head, fmt.Sprintf("/api/v1/warehouses/%d", whB))
	assert.Equal(t, 403, other.status())
	assert.Equal(t, "FORBIDDEN_WAREHOUSE", other.code())

	assert.Equal(t, 200, e.patchReq(head, fmt.Sprintf("/api/v1/warehouses/%d", whA), map[string]any{"name": "Baru"}).status())
	assert.Equal(t, 403, e.patchReq(head, fmt.Sprintf("/api/v1/warehouses/%d", whB), map[string]any{"name": "Baru"}).status())
	assert.Equal(t, "Gudang WH-B", func() string {
		var n string
		require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT name FROM warehouses WHERE id=$1`, whB).Scan(&n))
		return n
	}(), "gudang lain tidak berubah")

	create := e.post(head, "/api/v1/warehouses", map[string]any{"code": "WH-NEW", "name": "Baru"})
	assert.Equal(t, 403, create.status(), "akun satu gudang tidak boleh membuat gudang baru")

	// Kepala gudang tanpa batasan boleh membuat gudang baru.
	global := e.tokenFor(domain.RoleHeadWarehouse, nil)
	assert.Equal(t, 201, e.post(global, "/api/v1/warehouses", map[string]any{"code": "WH-NEW", "name": "Baru"}).status())
	assert.Len(t, e.get(global, "/api/v1/warehouses").list(), 3)
}

// ================================================================ racks

func TestRacks_Create_DefaultsAndNormalization(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.newWarehouse(admin, "WH-A")

	normal := e.newRack(admin, wh, " a-02-01 ", map[string]any{"zone": " printer ", "description": " Rak printer "})
	require.Equal(t, 201, normal.status(), normal.rec.Body.String())
	assert.Equal(t, "A-02-01", normal.str("code"))
	assert.Equal(t, "PRINTER", normal.str("zone"))
	assert.Equal(t, "Rak printer", normal.str("description"))
	assert.Equal(t, true, normal.field("is_sellable"), "zona biasa dapat dijual")
	assert.Equal(t, "WH-A", normal.str("warehouse_code"))
	assert.Nil(t, normal.field("locked_by_opname_id"))

	quarantine := e.newRack(admin, wh, "Q-01", map[string]any{"zone": "karantina"})
	assert.Equal(t, false, quarantine.field("is_sellable"), "KARANTINA bawaannya tidak dapat dijual")
	workshop := e.newRack(admin, wh, "W-01", map[string]any{"zone": "WORKSHOP"})
	assert.Equal(t, false, workshop.field("is_sellable"))
	forced := e.newRack(admin, wh, "Q-02", map[string]any{"zone": "KARANTINA", "is_sellable": true})
	assert.Equal(t, true, forced.field("is_sellable"), "nilai eksplisit menang")
	noZone := e.newRack(admin, wh, "N-01", nil)
	assert.Equal(t, true, noZone.field("is_sellable"))
	assert.Nil(t, noZone.field("zone"))

	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action='RACK_CREATE' AND entity_id=$1`, idOf(normal)))
}

func TestRacks_DuplicateCodePerWarehouse(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	whA := e.newWarehouse(admin, "WH-A")
	whB := e.newWarehouse(admin, "WH-B")
	require.Equal(t, 201, e.newRack(admin, whA, "A-01", nil).status())

	dup := e.newRack(admin, whA, "a-01", nil)
	assert.Equal(t, 409, dup.status())
	assert.Equal(t, "DUPLICATE_VALUE", dup.code())
	assert.Contains(t, fieldErrors(dup), "code")

	assert.Equal(t, 201, e.newRack(admin, whB, "A-01", nil).status(), "kode sama boleh di gudang lain")
}

func TestRacks_Create_RejectsBadInput(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.newWarehouse(admin, "WH-A")
	inactive := e.newWarehouse(admin, "WH-OFF")
	e.exec(`UPDATE warehouses SET is_active = FALSE WHERE id = $1`, inactive)

	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"kode dengan spasi", map[string]any{"warehouse_id": wh, "code": "A 01"}, "code"},
		{"gudang tidak ada", map[string]any{"warehouse_id": 999999, "code": "A-01"}, "warehouse_id"},
		{"gudang tidak aktif", map[string]any{"warehouse_id": inactive, "code": "A-01"}, "warehouse_id"},
		{"zona terlalu panjang", map[string]any{"warehouse_id": wh, "code": "A-01", "zone": strings.Repeat("Z", 31)}, "zone"},
	}
	for _, tc := range cases {
		r := e.post(admin, "/api/v1/racks", tc.body)
		assert.Contains(t, []int{400, 403, 422}, r.status(), tc.name)
		assert.Contains(t, fieldErrors(r), tc.field, tc.name+": "+r.rec.Body.String())
	}
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM racks`))
}

func TestRacks_Update_PatchAndImmutableFields(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.newWarehouse(admin, "WH-A")
	other := e.newWarehouse(admin, "WH-B")
	id := idOf(e.newRack(admin, wh, "A-01", map[string]any{"zone": "PRINTER", "description": "lama"}))
	url := fmt.Sprintf("/api/v1/racks/%d", id)

	r := e.patchReq(admin, url, map[string]any{"zone": "karantina", "is_sellable": false, "description": nil, "code": "X-99", "warehouse_id": other})
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	assert.Equal(t, "KARANTINA", r.str("zone"))
	assert.Equal(t, false, r.field("is_sellable"))
	assert.Nil(t, r.field("description"))
	assert.Equal(t, "A-01", r.str("code"), "kode tidak dapat diubah")
	assert.EqualValues(t, wh, r.field("warehouse_id"), "gudang tidak dapat dipindah")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action='RACK_UPDATE' AND entity_id=$1`, id))
}

func TestRacks_Deactivate_BlockedWithStockOrSerials(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.newWarehouse(admin, "WH-A")
	rackStock := idOf(e.newRack(admin, wh, "A-01", nil))
	rackSerial := idOf(e.newRack(admin, wh, "A-02", nil))
	rackEmpty := idOf(e.newRack(admin, wh, "A-03", nil))
	cat := e.newCategory(admin, "Printer", "ASSET")
	prodQty := idOf(e.newProduct(admin, cat, "PRN-Q", nil))
	prodSN := idOf(e.newProduct(admin, cat, "PRN-S", map[string]any{"has_serial_number": true}))
	e.seedStock(prodQty, rackStock, 3)
	e.exec(`INSERT INTO product_serials (product_id, asset_tag, serial_number, current_rack_id) VALUES ($1,'AST-2611-00001','SN1',$2)`, prodSN, rackSerial)

	for _, id := range []int64{rackStock, rackSerial} {
		r := e.patchReq(admin, fmt.Sprintf("/api/v1/racks/%d", id), map[string]any{"is_active": false})
		assert.Equal(t, 409, r.status())
		assert.Equal(t, "LOCATION_HAS_STOCK", r.code())
	}
	ok := e.patchReq(admin, fmt.Sprintf("/api/v1/racks/%d", rackEmpty), map[string]any{"is_active": false})
	assert.Equal(t, 200, ok.status())
	assert.Equal(t, false, ok.field("is_active"))
}

func TestRacks_Reactivate_RequiresActiveWarehouse(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.newWarehouse(admin, "WH-A")
	id := idOf(e.newRack(admin, wh, "A-01", nil))
	url := fmt.Sprintf("/api/v1/racks/%d", id)
	require.Equal(t, 200, e.patchReq(admin, url, map[string]any{"is_active": false}).status())
	e.exec(`UPDATE warehouses SET is_active = FALSE WHERE id = $1`, wh)

	r := e.patchReq(admin, url, map[string]any{"is_active": true})
	assert.Equal(t, 422, r.status())
	assert.Contains(t, fieldErrors(r), "is_active")
}

func TestRacks_LockedByOpname_CannotBeEdited(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.newWarehouse(admin, "WH-A")
	id := idOf(e.newRack(admin, wh, "A-01", nil))
	var opnameID int64
	require.NoError(t, e.pool.QueryRow(testCtx(), `
		INSERT INTO stock_opnames (opname_number, warehouse_id, started_by)
		VALUES ('OPN/1', $1, (SELECT id FROM users LIMIT 1)) RETURNING id`, wh).Scan(&opnameID))
	e.exec(`UPDATE racks SET locked_by_opname_id = $2 WHERE id = $1`, id, opnameID)

	r := e.patchReq(admin, fmt.Sprintf("/api/v1/racks/%d", id), map[string]any{"zone": "BARU"})
	assert.Equal(t, 409, r.status())
	assert.Equal(t, "RACK_LOCKED_BY_OPNAME", r.code())
	got := e.get(admin, fmt.Sprintf("/api/v1/racks/%d", id))
	assert.EqualValues(t, opnameID, got.field("locked_by_opname_id"))
}

func TestRacks_Scoping_StaffOfOneWarehouse(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	whA := e.newWarehouse(admin, "WH-A")
	whB := e.newWarehouse(admin, "WH-B")
	rackA := idOf(e.newRack(admin, whA, "A-01", nil))
	rackB := idOf(e.newRack(admin, whB, "B-01", nil))
	staff := e.tokenFor(domain.RoleStaffWarehouse, &whA)

	list := e.get(staff, "/api/v1/racks")
	assert.Equal(t, []string{"A-01"}, names(list.list(), "code"), "hanya rak gudang sendiri")
	asked := e.get(staff, fmt.Sprintf("/api/v1/racks?warehouse_id=%d", whB))
	assert.Equal(t, 403, asked.status())
	assert.Equal(t, "FORBIDDEN_WAREHOUSE", asked.code())

	assert.Equal(t, 200, e.get(staff, fmt.Sprintf("/api/v1/racks/%d", rackA)).status())
	assert.Equal(t, 403, e.get(staff, fmt.Sprintf("/api/v1/racks/%d", rackB)).status())

	assert.Equal(t, 201, e.newRack(staff, whA, "A-02", nil).status(), "staf boleh membuat rak di gudangnya")
	assert.Equal(t, 403, e.newRack(staff, whB, "B-02", nil).status())
	assert.Equal(t, 403, e.patchReq(staff, fmt.Sprintf("/api/v1/racks/%d", rackB), map[string]any{"zone": "X"}).status())
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM racks WHERE code='B-02'`))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM racks WHERE id=$1 AND zone='X'`, rackB))
}

func TestRacks_List_Filters(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	whA := e.newWarehouse(admin, "WH-A")
	whB := e.newWarehouse(admin, "WH-B")
	e.newRack(admin, whA, "A-01", map[string]any{"zone": "PRINTER"})
	e.newRack(admin, whA, "A-02", map[string]any{"zone": "KARANTINA"})
	e.newRack(admin, whB, "B-01", map[string]any{"zone": "PRINTER"})

	all := e.get(admin, "/api/v1/racks")
	assert.Equal(t, []string{"A-01", "A-02", "B-01"}, names(all.list(), "code"), "urut gudang lalu kode")
	assert.Equal(t, []string{"B-01"}, names(e.get(admin, fmt.Sprintf("/api/v1/racks?warehouse_id=%d", whB)).list(), "code"))
	assert.Equal(t, []string{"A-01", "B-01"}, names(e.get(admin, "/api/v1/racks?zone=printer").list(), "code"))
	assert.Equal(t, []string{"A-02"}, names(e.get(admin, "/api/v1/racks?is_sellable=false").list(), "code"))
	assert.Equal(t, []string{"A-02"}, names(e.get(admin, "/api/v1/racks?q=karan").list(), "code"), "q mencari juga pada zona")
}

// ================================================================ categories

func TestCategories_CRUDAndDuplicate(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")

	r := e.post(admin, "/api/v1/categories", map[string]any{"name": "  Printer ", "item_type": "ASSET"})
	require.Equal(t, 201, r.status(), r.rec.Body.String())
	assert.Equal(t, "Printer", r.str("name"))
	assert.Equal(t, "ASSET", r.str("item_type"))
	assert.Equal(t, true, r.field("is_active"))
	id := idOf(r)

	dup := e.post(admin, "/api/v1/categories", map[string]any{"name": "Printer", "item_type": "CONSUMABLE"})
	assert.Equal(t, 409, dup.status())
	assert.Contains(t, fieldErrors(dup), "name")

	for _, bad := range []map[string]any{
		{"name": "X", "item_type": "LAINNYA"},
		{"name": "X"},
		{"item_type": "ASSET"},
		{"name": "   ", "item_type": "ASSET"},
	} {
		assert.Contains(t, []int{400, 422}, e.post(admin, "/api/v1/categories", bad).status(), "%v", bad)
	}

	u := e.patchReq(admin, fmt.Sprintf("/api/v1/categories/%d", id), map[string]any{"name": "Printer Laser", "is_active": false})
	require.Equal(t, 200, u.status())
	assert.Equal(t, "Printer Laser", u.str("name"))
	assert.Equal(t, false, u.field("is_active"))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action='CATEGORY_DEACTIVATE' AND entity_id=$1`, id))

	// Ganti nama menjadi nama kategori lain → 409.
	other := e.newCategory(admin, "PC", "ASSET")
	clash := e.patchReq(admin, fmt.Sprintf("/api/v1/categories/%d", other), map[string]any{"name": "Printer Laser"})
	assert.Equal(t, 409, clash.status())
	assert.Contains(t, fieldErrors(clash), "name")
}

func TestCategories_ItemTypeLockedOnceUsedByProducts(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	used := e.newCategory(admin, "Printer", "ASSET")
	free := e.newCategory(admin, "Kosong", "ASSET")
	require.Equal(t, 201, e.newProduct(admin, used, "PRN-1", nil).status())

	r := e.patchReq(admin, fmt.Sprintf("/api/v1/categories/%d", used), map[string]any{"item_type": "CONSUMABLE"})
	assert.Equal(t, 409, r.status())
	assert.Equal(t, "CATEGORY_IN_USE", r.code())

	ok := e.patchReq(admin, fmt.Sprintf("/api/v1/categories/%d", free), map[string]any{"item_type": "CONSUMABLE"})
	assert.Equal(t, 200, ok.status())
	assert.Equal(t, "CONSUMABLE", ok.str("item_type"))

	// Mengganti nama tetap boleh walau dipakai produk.
	assert.Equal(t, 200, e.patchReq(admin, fmt.Sprintf("/api/v1/categories/%d", used), map[string]any{"name": "Printer Kantor"}).status())
}

func TestCategories_ListFilters(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	e.newCategory(admin, "Printer", "ASSET")
	e.newCategory(admin, "Toner", "CONSUMABLE")
	off := e.newCategory(admin, "Lama", "ASSET")
	e.exec(`UPDATE categories SET is_active = FALSE WHERE id = $1`, off)

	assert.Equal(t, []string{"Lama", "Printer", "Toner"}, names(e.get(admin, "/api/v1/categories").list(), "name"))
	assert.Equal(t, []string{"Toner"}, names(e.get(admin, "/api/v1/categories?item_type=CONSUMABLE").list(), "name"))
	assert.Equal(t, []string{"Printer", "Toner"}, names(e.get(admin, "/api/v1/categories?is_active=true").list(), "name"))
	assert.Equal(t, 400, e.get(admin, "/api/v1/categories?item_type=LAINNYA").status())
}

// ================================================================ products

func TestProducts_Create_NormalizesAndFormatsMoneyAsString(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	cat := e.newCategory(admin, "Printer", "ASSET")

	r := e.newProduct(admin, cat, " prn-l3210 ", map[string]any{
		"name": " Epson L3210 ", "brand": "Epson", "model": "L3210", "has_serial_number": true, "min_stock_alert": 2,
		"default_monthly_rate": "550000", "default_sell_price": "1250000.5", "description": " Printer tinta ",
	})
	require.Equal(t, 201, r.status(), r.rec.Body.String())
	assert.Equal(t, "PRN-L3210", r.str("sku"), "SKU huruf besar & trim")
	assert.Equal(t, "Epson L3210", r.str("name"))
	assert.Equal(t, "unit", r.str("unit"), "satuan bawaan")
	assert.Equal(t, "550000.00", r.str("default_monthly_rate"), "uang berupa string 2 desimal")
	assert.Equal(t, "1250000.50", r.str("default_sell_price"))
	assert.Equal(t, "Printer", r.str("category_name"))
	assert.Equal(t, "ASSET", r.str("item_type"))
	assert.Equal(t, true, r.field("has_serial_number"))
	assert.EqualValues(t, 2, r.field("min_stock_alert"))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action='PRODUCT_CREATE' AND entity_id=$1`, idOf(r)))

	bare := e.newProduct(admin, cat, "PRN-2", nil)
	require.Equal(t, 201, bare.status())
	assert.Nil(t, bare.field("default_monthly_rate"))
	assert.Nil(t, bare.field("default_sell_price"))
	assert.Equal(t, false, bare.field("has_serial_number"))
	assert.EqualValues(t, 0, bare.field("min_stock_alert"))
}

func TestProducts_Create_Validation(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	asset := e.newCategory(admin, "Printer", "ASSET")
	consumable := e.newCategory(admin, "Toner", "CONSUMABLE")
	inactive := e.newCategory(admin, "Lama", "ASSET")
	e.exec(`UPDATE categories SET is_active = FALSE WHERE id = $1`, inactive)

	cases := []struct {
		name  string
		cat   int64
		extra map[string]any
		sku   string
		field string
	}{
		{"SKU dengan spasi", asset, nil, "PRN 1", "sku"},
		{"kategori tidak ada", 999999, nil, "P-1", "category_id"},
		{"kategori tidak aktif", inactive, nil, "P-2", "category_id"},
		{"consumable tidak boleh ber-SN", consumable, map[string]any{"has_serial_number": true}, "P-3", "has_serial_number"},
		{"min stok negatif", asset, map[string]any{"min_stock_alert": -1}, "P-4", "min_stock_alert"},
		{"tarif lebih dari 2 desimal", asset, map[string]any{"default_monthly_rate": "100.123"}, "P-5", "default_monthly_rate"},
		{"tarif negatif", asset, map[string]any{"default_monthly_rate": "-5"}, "P-6", "default_monthly_rate"},
		{"tarif notasi ilmiah", asset, map[string]any{"default_sell_price": "1e3"}, "P-7", "default_sell_price"},
		{"tarif bukan angka", asset, map[string]any{"default_sell_price": "abc"}, "P-8", "default_sell_price"},
		{"tarif melebihi NUMERIC(15,2)", asset, map[string]any{"default_sell_price": "12345678901234"}, "P-9", "default_sell_price"},
		{"satuan terlalu panjang", asset, map[string]any{"unit": strings.Repeat("u", 21)}, "P-10", "unit"},
	}
	for _, tc := range cases {
		r := e.newProduct(admin, tc.cat, tc.sku, tc.extra)
		assert.Contains(t, []int{400, 422}, r.status(), tc.name+": "+r.rec.Body.String())
		assert.Contains(t, fieldErrors(r), tc.field, tc.name+": "+r.rec.Body.String())
	}
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM products`))

	// Uang sebagai angka JSON (bukan string) ditolak: kontrak plan mewajibkan string.
	num := e.post(admin, "/api/v1/products", map[string]any{"sku": "P-N", "name": "x", "category_id": asset, "default_monthly_rate": 550000})
	assert.Equal(t, 400, num.status())
}

func TestProducts_DuplicateSKU_CaseInsensitive(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	cat := e.newCategory(admin, "Printer", "ASSET")
	require.Equal(t, 201, e.newProduct(admin, cat, "PRN-1", nil).status())

	r := e.newProduct(admin, cat, "prn-1", nil)
	assert.Equal(t, 409, r.status())
	assert.Equal(t, "DUPLICATE_VALUE", r.code())
	assert.Contains(t, fieldErrors(r), "sku")
}

func TestProducts_Update_MoneyPatchAndCategoryMove(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	printers := e.newCategory(admin, "Printer", "ASSET")
	pcs := e.newCategory(admin, "PC", "ASSET")
	toner := e.newCategory(admin, "Toner", "CONSUMABLE")
	id := idOf(e.newProduct(admin, printers, "PRN-1", map[string]any{"default_monthly_rate": "500000", "brand": "Epson"}))
	url := fmt.Sprintf("/api/v1/products/%d", id)

	r := e.patchReq(admin, url, map[string]any{"default_monthly_rate": "600000.50", "default_sell_price": "1000", "brand": nil, "category_id": pcs})
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	assert.Equal(t, "600000.50", r.str("default_monthly_rate"))
	assert.Equal(t, "1000.00", r.str("default_sell_price"))
	assert.Nil(t, r.field("brand"))
	assert.Equal(t, "PC", r.str("category_name"))

	cleared := e.patchReq(admin, url, map[string]any{"default_monthly_rate": nil})
	assert.Equal(t, 200, cleared.status())
	assert.Nil(t, cleared.field("default_monthly_rate"), "null mengosongkan tarif")
	assert.Equal(t, "1000.00", cleared.str("default_sell_price"), "yang tidak dikirim tetap")

	// Pindah ke kategori CONSUMABLE: produk ber-SN dilarang.
	sn := e.patchReq(admin, url, map[string]any{"has_serial_number": true})
	require.Equal(t, 200, sn.status())
	toConsumable := e.patchReq(admin, url, map[string]any{"category_id": toner})
	assert.Equal(t, 422, toConsumable.status())
	assert.Contains(t, fieldErrors(toConsumable), "has_serial_number")

	bad := e.patchReq(admin, url, map[string]any{"default_sell_price": "1.234"})
	assert.Equal(t, 422, bad.status())
	assert.Contains(t, fieldErrors(bad), "default_sell_price")

	// 3 perubahan efektif (tarif+kategori, kosongkan tarif, aktifkan SN); 2 permintaan ditolak tidak menulis audit.
	assert.Equal(t, 3, e.count(`SELECT count(*) FROM audit_logs WHERE action='PRODUCT_UPDATE' AND entity_id=$1`, id))
}

func TestProducts_SKUAndSerialModeLockedAfterMovements(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.newWarehouse(admin, "WH-A")
	rack := idOf(e.newRack(admin, wh, "A-01", nil))
	cat := e.newCategory(admin, "Printer", "ASSET")
	used := idOf(e.newProduct(admin, cat, "PRN-USED", nil))
	fresh := idOf(e.newProduct(admin, cat, "PRN-FRESH", nil))
	e.seedStock(used, rack, 1)

	usedURL := fmt.Sprintf("/api/v1/products/%d", used)
	r := e.patchReq(admin, usedURL, map[string]any{"sku": "PRN-NEW"})
	assert.Equal(t, 409, r.status())
	assert.Equal(t, "PRODUCT_IN_USE", r.code())
	assert.Equal(t, 409, e.patchReq(admin, usedURL, map[string]any{"has_serial_number": true}).status())

	// Atribut lain tetap boleh diubah, termasuk menonaktifkan.
	ok := e.patchReq(admin, usedURL, map[string]any{"name": "Nama Baru", "min_stock_alert": 5, "is_active": false})
	assert.Equal(t, 200, ok.status(), ok.rec.Body.String())
	// Mengirim nilai yang sama (tidak berubah) tidak terhitung perubahan terkunci.
	assert.Equal(t, 200, e.patchReq(admin, usedURL, map[string]any{"sku": "prn-used", "has_serial_number": false}).status())

	freshURL := fmt.Sprintf("/api/v1/products/%d", fresh)
	assert.Equal(t, 200, e.patchReq(admin, freshURL, map[string]any{"sku": "PRN-RENAMED", "has_serial_number": true}).status(),
		"produk belum dipakai: bebas diubah")
}

func TestProducts_List_FiltersAndTechnicianCanRead(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	printers := e.newCategory(admin, "Printer", "ASSET")
	toner := e.newCategory(admin, "Toner", "CONSUMABLE")
	e.newProduct(admin, printers, "PRN-EPS", map[string]any{"name": "Epson L3210", "brand": "Epson", "has_serial_number": true})
	e.newProduct(admin, printers, "PRN-HP", map[string]any{"name": "HP LaserJet", "brand": "HP", "model": "M15"})
	e.newProduct(admin, toner, "TN-HP", map[string]any{"name": "Toner HP 05A", "brand": "HP"})

	tech := e.tokenFor(domain.RoleTechnician, nil)
	all := e.get(tech, "/api/v1/products")
	require.Equal(t, 200, all.status())
	assert.Equal(t, []string{"Epson L3210", "HP LaserJet", "Toner HP 05A"}, names(all.list(), "name"))

	assert.Equal(t, []string{"TN-HP"}, names(e.get(admin, "/api/v1/products?item_type=CONSUMABLE").list(), "sku"))
	assert.Equal(t, []string{"PRN-EPS"}, names(e.get(admin, "/api/v1/products?has_serial_number=true").list(), "sku"))
	assert.Equal(t, []string{"PRN-EPS", "PRN-HP"}, names(e.get(admin, fmt.Sprintf("/api/v1/products?category_id=%d&sort=sku", printers)).list(), "sku"))
	assert.Equal(t, []string{"PRN-HP", "TN-HP"}, names(e.get(admin, "/api/v1/products?q=hp&sort=sku").list(), "sku"), "q mencari SKU/nama/merek/model")
	assert.Equal(t, []string{"PRN-HP"}, names(e.get(admin, "/api/v1/products?q=m15").list(), "sku"))
	assert.Equal(t, []string{"TN-HP", "PRN-HP", "PRN-EPS"}, names(e.get(admin, "/api/v1/products?sort=-category,-sku").list(), "sku"))
	assert.Equal(t, 400, e.get(admin, "/api/v1/products?sort=default_sell_price").status())

	// Teknisi hanya membaca produk.
	assert.Equal(t, 403, e.post(tech, "/api/v1/products", map[string]any{"sku": "X", "name": "x", "category_id": printers}).status())
}

// ================================================================ suppliers

func TestSuppliers_CRUD(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")

	r := e.post(admin, "/api/v1/suppliers", map[string]any{
		"code": " sup-001 ", "name": " PT Maju Jaya ", "pic_name": "Budi", "phone": "021-555",
		"email": " Sales@Maju.CO.ID ", "npwp": "01.234.567.8-901.000", "address": "Jl. Raya 1",
	})
	require.Equal(t, 201, r.status(), r.rec.Body.String())
	assert.Equal(t, "SUP-001", r.str("code"))
	assert.Equal(t, "PT Maju Jaya", r.str("name"))
	assert.Equal(t, "sales@maju.co.id", r.str("email"), "email huruf kecil")
	id := idOf(r)
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action='SUPPLIER_CREATE' AND entity_id=$1`, id))

	dup := e.post(admin, "/api/v1/suppliers", map[string]any{"code": "SUP-001", "name": "Lain"})
	assert.Equal(t, 409, dup.status())
	assert.Contains(t, fieldErrors(dup), "code")

	badEmail := e.post(admin, "/api/v1/suppliers", map[string]any{"code": "SUP-2", "name": "x", "email": "bukan-email"})
	assert.Equal(t, 422, badEmail.status())
	assert.Contains(t, fieldErrors(badEmail), "email")
	assert.Equal(t, 422, e.post(admin, "/api/v1/suppliers", map[string]any{"code": "sup 3", "name": "x"}).status())

	url := fmt.Sprintf("/api/v1/suppliers/%d", id)
	u := e.patchReq(admin, url, map[string]any{"name": "PT Maju Mundur", "email": nil, "phone": "", "code": "HACK", "is_active": false})
	require.Equal(t, 200, u.status(), u.rec.Body.String())
	assert.Equal(t, "PT Maju Mundur", u.str("name"))
	assert.Nil(t, u.field("email"))
	assert.Nil(t, u.field("phone"), "string kosong dianggap dikosongkan")
	assert.Equal(t, "SUP-001", u.str("code"), "kode tidak dapat diubah")
	assert.Equal(t, false, u.field("is_active"))
	assert.Equal(t, "Budi", u.str("pic_name"))

	assert.Equal(t, []string{"SUP-001"}, names(e.get(admin, "/api/v1/suppliers?q=budi").list(), "code"), "q mencari juga PIC")
	assert.Empty(t, e.get(admin, "/api/v1/suppliers?is_active=true").list())
}

// ================================================================ audit & atomicity

func TestMaster_AuditCapturesBeforeAfterOfChangedFieldsOnly(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	cat := e.newCategory(admin, "Printer", "ASSET")
	id := idOf(e.newProduct(admin, cat, "PRN-1", map[string]any{"default_monthly_rate": "100"}))

	require.Equal(t, 200, e.patchReq(admin, fmt.Sprintf("/api/v1/products/%d", id),
		map[string]any{"name": "Nama Baru", "default_monthly_rate": "250.50"}).status())

	var before, after string
	require.NoError(t, e.pool.QueryRow(testCtx(), `
		SELECT before_data::text, after_data::text FROM audit_logs
		WHERE action='PRODUCT_UPDATE' AND entity_id=$1`, id).Scan(&before, &after))
	assert.Contains(t, before, `"Produk PRN-1"`)
	assert.Contains(t, after, `"Nama Baru"`)
	assert.Contains(t, before, `"100.00"`)
	assert.Contains(t, after, `"250.50"`)
	assert.NotContains(t, after, "sku", "field yang tidak berubah tidak dicatat")
}

func TestMaster_ConcurrentSameCode_OnlyOneWins(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")

	const n = 8
	results := make(chan int, n)
	for i := 0; i < n; i++ {
		go func() {
			results <- e.call("POST", "/api/v1/warehouses", map[string]any{"code": "WH-RACE", "name": "x"}, withBearer(admin)).status()
		}()
	}
	created, conflicts := 0, 0
	for i := 0; i < n; i++ {
		switch <-results {
		case 201:
			created++
		case 409:
			conflicts++
		}
	}
	assert.Equal(t, 1, created)
	assert.Equal(t, n-1, conflicts)
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM warehouses WHERE code='WH-RACE'`))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action='WAREHOUSE_CREATE'`), "gagal → tidak ada audit (rollback)")
}

// Setiap perubahan data tercatat SIAPA pelakunya (user_id), tanpa usecase harus mengisinya.
func TestAudit_RecordsAuthenticatedActor(t *testing.T) {
	e := newEnv(t)
	adminID := e.addUser("admin@x.id", domain.RoleAdmin)
	admin, _ := e.mustLogin("admin@x.id")
	headID := e.addUser("head@x.id", domain.RoleHeadWarehouse)
	head, _ := e.mustLogin("head@x.id")

	wh := idOf(e.post(head, "/api/v1/warehouses", map[string]any{"code": "WH-1", "name": "G"}))
	require.Equal(t, 200, e.patchReq(head, fmt.Sprintf("/api/v1/warehouses/%d", wh), map[string]any{"name": "Baru"}).status())
	cat := e.post(admin, "/api/v1/categories", map[string]any{"name": "Printer", "item_type": "ASSET"})
	require.Equal(t, 201, cat.status())
	user := e.createUser(admin, map[string]any{"name": "Budi", "email": "budi@x.id", "role": "DRIVER"})
	require.Equal(t, 201, user.status())

	actorOf := func(action string, entityID int64) int64 {
		var id *int64
		require.NoError(t, e.pool.QueryRow(testCtx(),
			`SELECT user_id FROM audit_logs WHERE action=$1 AND entity_id=$2`, action, entityID).Scan(&id))
		require.NotNil(t, id, "%s harus punya pelaku", action)
		return *id
	}
	assert.Equal(t, headID, actorOf("WAREHOUSE_CREATE", wh))
	assert.Equal(t, headID, actorOf("WAREHOUSE_UPDATE", wh))
	assert.Equal(t, adminID, actorOf("CATEGORY_CREATE", idOf(cat)))
	assert.Equal(t, adminID, actorOf("USER_CREATE", idOf(user)))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM audit_logs WHERE user_id IS NULL AND action NOT IN ('LOGIN_FAILED')`),
		"tidak ada baris audit tanpa pelaku")
}
