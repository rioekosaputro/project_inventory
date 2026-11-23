//go:build integration

package http_test

import (
	"fmt"
	nethttp "net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
)

// ---- helpers ----

func (e *env) adminToken(email string) string {
	e.t.Helper()
	id := e.addUser(email, domain.RoleAdmin)
	return e.mint(id, domain.RoleAdmin, nil)
}

func (e *env) addWarehouse(code string, active bool) int64 {
	e.t.Helper()
	var id int64
	require.NoError(e.t, e.pool.QueryRow(testCtx(),
		`INSERT INTO warehouses (code, name, is_active) VALUES ($1, $1, $2) RETURNING id`, code, active).Scan(&id))
	return id
}

func (e *env) createUser(token string, body map[string]any) *reply {
	return e.call("POST", "/api/v1/users", body, withBearer(token))
}

func idOf(r *reply) int64 { return int64(r.data()["id"].(float64)) }

func fieldErrors(r *reply) map[string]string {
	out := map[string]string{}
	if errs, ok := r.body["errors"].([]any); ok {
		for _, x := range errs {
			m := x.(map[string]any)
			out[m["field"].(string)] = m["message"].(string)
		}
	}
	return out
}

func path(id int64, suffix string) string { return fmt.Sprintf("/api/v1/users/%d%s", id, suffix) }

// ================================================================ RBAC

func TestUsers_OnlyAdminMayAccess(t *testing.T) {
	e := newEnv(t)
	target := e.addUser("target@x.id", domain.RoleDriver)

	endpoints := []struct{ method, path string }{
		{"GET", "/api/v1/users"},
		{"POST", "/api/v1/users"},
		{"GET", path(target, "")},
		{"PATCH", path(target, "")},
		{"POST", path(target, "/reset-password")},
	}

	for _, role := range []domain.Role{
		domain.RoleManagement, domain.RoleBusinessAdmin, domain.RoleStaffWarehouse,
		domain.RoleHeadWarehouse, domain.RoleDriver, domain.RoleTechnician,
	} {
		email := strings.ToLower(fmt.Sprintf("%s@x.id", role))
		e.addUser(email, role)
		access, _ := e.mustLogin(email)
		for _, ep := range endpoints {
			r := e.call(ep.method, ep.path, map[string]any{}, withBearer(access))
			assert.Equal(t, 403, r.status(), "%s %s %s", role, ep.method, ep.path)
			assert.Equal(t, "FORBIDDEN", r.code())
		}
	}

	for _, ep := range endpoints {
		assert.Equal(t, 401, e.call(ep.method, ep.path, map[string]any{}).status(), "tanpa token: %s %s", ep.method, ep.path)
	}
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM audit_logs WHERE action LIKE 'USER_%'`), "tidak ada efek samping dari percobaan ditolak")
}

func TestUsers_AdminWhoMustChangePasswordIsBlocked(t *testing.T) {
	e := newEnv(t)
	id := e.addUser("a@x.id", domain.RoleAdmin)
	e.exec(`UPDATE users SET must_change_password = TRUE WHERE id = $1`, id)
	access, _ := e.mustLogin("a@x.id")

	r := e.call("GET", "/api/v1/users", nil, withBearer(access))
	assert.Equal(t, 403, r.status())
	assert.Equal(t, "PASSWORD_CHANGE_REQUIRED", r.code())
}

// ================================================================ create

func TestUsers_Create_ReturnsTemporaryPasswordOnceAndForcesChange(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.addWarehouse("WH-1", true)

	r := e.createUser(admin, map[string]any{
		"name": "  Budi Gudang ", "email": "Budi@Contoh.ID", "phone": " 0812-3456 ", "role": "STAFF_WAREHOUSE", "warehouse_id": wh,
	})

	require.Equal(t, 201, r.status(), r.rec.Body.String())
	temp := r.str("temporary_password")
	assert.Len(t, temp, 16)
	assert.Equal(t, "budi@contoh.id", r.str("email"), "email dinormalisasi lowercase")
	assert.Equal(t, "Budi Gudang", r.str("name"), "nama di-trim")
	assert.Equal(t, "0812-3456", r.str("phone"))
	assert.Equal(t, true, r.data()["must_change_password"])
	assert.Equal(t, true, r.data()["is_active"])
	assert.EqualValues(t, wh, r.data()["warehouse_id"])
	assert.NotContains(t, r.rec.Body.String(), "password_hash")
	assert.NotContains(t, r.rec.Body.String(), "$2a$")

	// Password sementara tidak disimpan polos dan tidak ada di audit.
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM users WHERE password_hash = $1`, temp))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'USER_CREATE' AND entity_id = $1`, idOf(r)))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM audit_logs WHERE after_data::text LIKE '%' || $1 || '%' OR after_data::text LIKE '%$2a$%'`, temp))

	// Tampilan berikutnya tidak lagi memuat password sementara.
	got := e.call("GET", path(idOf(r), ""), nil, withBearer(admin))
	assert.Equal(t, 200, got.status())
	assert.NotContains(t, got.rec.Body.String(), temp)
	assert.NotContains(t, got.rec.Body.String(), "temporary_password")
}

func TestUsers_Create_NewUserCanLoginWithTempPasswordThenMustChangeIt(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	r := e.createUser(admin, map[string]any{"name": "Sari", "email": "sari@x.id", "role": "MANAGEMENT"})
	temp := r.str("temporary_password")

	login := e.login("sari@x.id", temp)
	require.Equal(t, 200, login.status())
	assert.Equal(t, true, login.data()["user"].(map[string]any)["must_change_password"])
	access := login.str("access_token")

	cp := e.call("POST", "/api/v1/auth/change-password",
		map[string]string{"current_password": temp, "new_password": "Sandi-Sari-2026"}, withBearer(access))
	require.Equal(t, 200, cp.status(), cp.rec.Body.String())

	assert.Equal(t, 401, e.login("sari@x.id", temp).status())
	assert.Equal(t, 200, e.login("sari@x.id", "Sandi-Sari-2026").status())
}

func TestUsers_Create_TemporaryPasswordsAreUnique(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")

	a := e.createUser(admin, map[string]any{"name": "A", "email": "a1@x.id", "role": "DRIVER"})
	b := e.createUser(admin, map[string]any{"name": "B", "email": "b1@x.id", "role": "DRIVER"})

	assert.NotEqual(t, a.str("temporary_password"), b.str("temporary_password"))
}

func TestUsers_Create_DuplicateEmailCaseInsensitive_409(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	require.Equal(t, 201, e.createUser(admin, map[string]any{"name": "A", "email": "dup@x.id", "role": "DRIVER"}).status())

	r := e.createUser(admin, map[string]any{"name": "B", "email": "DUP@X.ID", "role": "DRIVER"})

	assert.Equal(t, 409, r.status())
	assert.Equal(t, "EMAIL_TAKEN", r.code())
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM users`), "hanya admin + user pertama")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'USER_CREATE'`), "percobaan gagal tidak diaudit sebagai sukses")
}

func TestUsers_Create_Validation(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	inactiveWH := e.addWarehouse("WH-OFF", false)
	ok := func(over map[string]any) map[string]any {
		b := map[string]any{"name": "N", "email": "n@x.id", "role": "DRIVER"}
		for k, v := range over {
			b[k] = v
		}
		return b
	}

	tests := []struct {
		name   string
		body   map[string]any
		status int
		field  string
	}{
		{"nama kosong", ok(map[string]any{"name": ""}), 400, "name"},
		{"email format salah", ok(map[string]any{"email": "bukan-email"}), 400, "email"},
		{"role kosong", ok(map[string]any{"role": ""}), 400, "role"},
		{"role tidak dikenal", ok(map[string]any{"role": "SUPERUSER"}), 422, "role"},
		{"nama terlalu panjang", ok(map[string]any{"name": string(make([]rune, 0)) + fmt.Sprintf("%0101d", 1)}), 400, "name"},
		{"telepon terlalu panjang", ok(map[string]any{"phone": fmt.Sprintf("%031d", 1)}), 400, "phone"},
		{"warehouse_id untuk role non-gudang", ok(map[string]any{"role": "DRIVER", "warehouse_id": 1}), 422, "warehouse_id"},
		{"gudang tidak ada", ok(map[string]any{"role": "STAFF_WAREHOUSE", "warehouse_id": 99999}), 422, "warehouse_id"},
		{"gudang tidak aktif", ok(map[string]any{"role": "HEAD_WAREHOUSE", "warehouse_id": inactiveWH}), 422, "warehouse_id"},
		{"warehouse_id nol", ok(map[string]any{"role": "STAFF_WAREHOUSE", "warehouse_id": 0}), 400, "warehouse_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := e.createUser(admin, tt.body)
			assert.Equal(t, tt.status, r.status(), r.rec.Body.String())
			assert.Contains(t, fieldErrors(r), tt.field)
		})
	}
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM users`), "tidak ada user yang terbuat dari input tidak valid")
}

func TestUsers_Create_WarehouseScopedRolesAcceptNullForAllWarehouses(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")

	r := e.createUser(admin, map[string]any{"name": "Kepala", "email": "head@x.id", "role": "HEAD_WAREHOUSE"})

	require.Equal(t, 201, r.status())
	assert.Nil(t, r.data()["warehouse_id"], "NULL = semua gudang (plan 5.1)")
}

// ================================================================ read

func TestUsers_Get(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	id := e.addUser("u@x.id", domain.RoleTechnician)

	r := e.call("GET", path(id, ""), nil, withBearer(admin))
	assert.Equal(t, 200, r.status())
	assert.Equal(t, "TECHNICIAN", r.str("role"))
	assert.NotContains(t, r.rec.Body.String(), "password_hash")

	nf := e.call("GET", path(99999, ""), nil, withBearer(admin))
	assert.Equal(t, 404, nf.status())
	assert.Equal(t, "NOT_FOUND", nf.code())

	for _, bad := range []string{"abc", "0", "-5", "1.5"} {
		r := e.call("GET", "/api/v1/users/"+bad, nil, withBearer(admin))
		assert.Equal(t, 400, r.status(), bad)
	}
}

func TestUsers_List_PaginationFiltersAndSort(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.addWarehouse("WH-1", true)
	for i, spec := range []struct {
		name, email string
		role        string
		wh          any
	}{
		{"Anto", "anto@x.id", "DRIVER", nil},
		{"Bima", "bima@x.id", "DRIVER", nil},
		{"Citra", "citra@x.id", "STAFF_WAREHOUSE", wh},
		{"Dedi 100%", "dedi@x.id", "TECHNICIAN", nil},
		{"Eka_Tek", "eka@x.id", "TECHNICIAN", nil},
	} {
		body := map[string]any{"name": spec.name, "email": spec.email, "role": spec.role}
		if spec.wh != nil {
			body["warehouse_id"] = spec.wh
		}
		require.Equal(t, 201, e.createUser(admin, body).status(), i)
	}
	list := func(q string) *reply { return e.call("GET", "/api/v1/users"+q, nil, withBearer(admin)) }
	names := func(r *reply) []string {
		var out []string
		for _, x := range r.body["data"].([]any) {
			out = append(out, x.(map[string]any)["name"].(string))
		}
		return out
	}

	t.Run("paginasi + meta", func(t *testing.T) {
		r := list("?limit=2&page=2&sort=name")
		require.Equal(t, 200, r.status())
		assert.Equal(t, []string{"Bima", "Citra"}, names(r))
		meta := r.body["meta"].(map[string]any)
		assert.EqualValues(t, 6, meta["total"])
		assert.EqualValues(t, 3, meta["total_pages"])
		assert.EqualValues(t, 2, meta["page"])
		assert.EqualValues(t, 2, meta["limit"])
	})
	t.Run("sort menurun", func(t *testing.T) {
		assert.Equal(t, "Eka_Tek", names(list("?sort=-name&limit=1"))[0])
	})
	t.Run("filter role", func(t *testing.T) {
		assert.Equal(t, []string{"Anto", "Bima"}, names(list("?role=DRIVER&sort=name")))
	})
	t.Run("filter warehouse", func(t *testing.T) {
		assert.Equal(t, []string{"Citra"}, names(list(fmt.Sprintf("?warehouse_id=%d", wh))))
	})
	t.Run("pencarian sebagian, tak peka huruf besar", func(t *testing.T) {
		assert.Equal(t, []string{"Citra"}, names(list("?q=CITR")))
		assert.Equal(t, []string{"Bima"}, names(list("?q=bima@x")), "cocok juga pada email")
	})
	t.Run("wildcard LIKE di input pengguna dinetralkan", func(t *testing.T) {
		assert.Equal(t, []string{"Dedi 100%"}, names(list("?q=%25")), "% harfiah, bukan 'cocok semua'")
		assert.Equal(t, []string{"Eka_Tek"}, names(list("?q=a_t")), "_ harfiah, bukan satu karakter bebas")
	})
	t.Run("filter is_active", func(t *testing.T) {
		e.exec(`UPDATE users SET is_active = FALSE WHERE email = 'anto@x.id'`)
		assert.NotContains(t, names(list("?is_active=true")), "Anto")
		assert.Equal(t, []string{"Anto"}, names(list("?is_active=false")))
	})
	t.Run("halaman di luar jangkauan = [] bukan null", func(t *testing.T) {
		r := list("?page=50")
		assert.Equal(t, 200, r.status())
		assert.Equal(t, []any{}, r.body["data"])
	})
	t.Run("input berbahaya/tidak valid ditolak 400", func(t *testing.T) {
		for _, q := range []string{"?sort=password_hash", "?sort=name%3BDROP%20TABLE%20users", "?role=SUPERUSER", "?limit=abc", "?is_active=mungkin", "?warehouse_id=-1"} {
			r := list(q)
			assert.Equal(t, 400, r.status(), q)
		}
		assert.Equal(t, 6, e.count(`SELECT count(*) FROM users`), "tabel users utuh")
	})
	t.Run("password_hash tidak pernah ikut", func(t *testing.T) {
		assert.NotContains(t, list("").rec.Body.String(), "password_hash")
	})
}

// ================================================================ update

func TestUsers_Update_ChangesFieldsAndAuditsOnlyTheDiff(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.addWarehouse("WH-1", true)
	c := e.createUser(admin, map[string]any{"name": "Lama", "email": "u@x.id", "role": "STAFF_WAREHOUSE", "phone": "0811"})
	id := idOf(c)

	r := e.call("PATCH", path(id, ""), map[string]any{"name": "Baru", "role": "HEAD_WAREHOUSE", "warehouse_id": wh}, withBearer(admin))

	require.Equal(t, 200, r.status(), r.rec.Body.String())
	assert.Equal(t, "Baru", r.str("name"))
	assert.Equal(t, "HEAD_WAREHOUSE", r.str("role"))
	assert.Equal(t, "0811", r.str("phone"), "field yang tidak dikirim tidak berubah")
	assert.Equal(t, "u@x.id", r.str("email"), "email tidak dapat diubah lewat PATCH")

	var before, after string
	require.NoError(t, e.pool.QueryRow(testCtx(),
		`SELECT before_data::text, after_data::text FROM audit_logs WHERE action = 'USER_UPDATE' AND entity_id = $1`, id).Scan(&before, &after))
	assert.Contains(t, before, `"name": "Lama"`)
	assert.Contains(t, after, `"name": "Baru"`)
	assert.NotContains(t, after, "phone", "hanya field yang berubah yang diaudit")
	assert.NotContains(t, before+after, "password")
}

func TestUsers_Update_NullClearsPhoneAndWarehouseWhileAbsentKeepsThem(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.addWarehouse("WH-1", true)
	id := idOf(e.createUser(admin, map[string]any{"name": "U", "email": "u@x.id", "role": "STAFF_WAREHOUSE", "phone": "0811", "warehouse_id": wh}))

	r := e.call("PATCH", path(id, ""), map[string]any{"name": "U2"}, withBearer(admin))
	assert.Equal(t, "0811", r.str("phone"))
	assert.EqualValues(t, wh, r.data()["warehouse_id"])

	r = e.call("PATCH", path(id, ""), map[string]any{"phone": nil, "warehouse_id": nil}, withBearer(admin))
	require.Equal(t, 200, r.status(), r.rec.Body.String())
	assert.Nil(t, r.data()["phone"])
	assert.Nil(t, r.data()["warehouse_id"], "null = semua gudang")

	r = e.call("PATCH", path(id, ""), map[string]any{"phone": "  "}, withBearer(admin))
	assert.Nil(t, r.data()["phone"], "telepon kosong/spasi menjadi NULL")
}

func TestUsers_Update_NoChangeWritesNoAudit(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	id := idOf(e.createUser(admin, map[string]any{"name": "Sama", "email": "u@x.id", "role": "DRIVER"}))
	baseline := e.count(`SELECT count(*) FROM audit_logs`)

	r := e.call("PATCH", path(id, ""), map[string]any{"name": "Sama", "role": "DRIVER", "is_active": true}, withBearer(admin))

	assert.Equal(t, 200, r.status())
	assert.Equal(t, baseline, e.count(`SELECT count(*) FROM audit_logs`))
}

func TestUsers_Update_Validation(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	wh := e.addWarehouse("WH-1", true)
	id := idOf(e.createUser(admin, map[string]any{"name": "U", "email": "u@x.id", "role": "STAFF_WAREHOUSE", "warehouse_id": wh}))
	patch := func(body map[string]any) *reply { return e.call("PATCH", path(id, ""), body, withBearer(admin)) }

	assert.Equal(t, 422, patch(map[string]any{"role": "SUPERUSER"}).status())
	assert.Equal(t, 400, patch(map[string]any{"name": ""}).status())
	assert.Equal(t, 400, patch(map[string]any{"is_active": "ya"}).status())
	assert.Equal(t, 404, e.call("PATCH", path(99999, ""), map[string]any{"name": "x"}, withBearer(admin)).status())

	// Berpindah ke role non-gudang sambil masih memegang warehouse_id harus eksplisit dikosongkan.
	r := patch(map[string]any{"role": "DRIVER"})
	assert.Equal(t, 422, r.status())
	assert.Contains(t, fieldErrors(r), "warehouse_id")
	assert.Equal(t, 200, patch(map[string]any{"role": "DRIVER", "warehouse_id": nil}).status())
}

func TestUsers_Deactivate_RevokesSessionsBlocksLoginAndCanBeReactivated(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	id := e.addUser("u@x.id", domain.RoleDriver)
	_, rt := e.mustLogin("u@x.id")
	_, rt2 := e.mustLogin("u@x.id") // dua sesi

	r := e.call("PATCH", path(id, ""), map[string]any{"is_active": false}, withBearer(admin))

	require.Equal(t, 200, r.status())
	assert.Equal(t, false, r.data()["is_active"])
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL`, id),
		"semua refresh token harus dicabut")
	assert.Equal(t, 401, e.refresh(rt).status())
	assert.Equal(t, 401, e.refresh(rt2).status())

	login := e.login("u@x.id", testPassword)
	assert.Equal(t, 403, login.status())
	assert.Equal(t, "ACCOUNT_DISABLED", login.code())
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'USER_DEACTIVATE' AND entity_id = $1`, id))

	// Aktifkan kembali.
	r = e.call("PATCH", path(id, ""), map[string]any{"is_active": true}, withBearer(admin))
	require.Equal(t, 200, r.status())
	assert.Equal(t, 200, e.login("u@x.id", testPassword).status())
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'USER_ACTIVATE' AND entity_id = $1`, id))
}

// ================================================================ safeguards

func TestUsers_AdminCannotDeactivateDemoteOrResetThemselves(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	self := int64(e.count(`SELECT id FROM users WHERE email = 'admin@x.id'`))

	r := e.call("PATCH", path(self, ""), map[string]any{"is_active": false}, withBearer(admin))
	assert.Equal(t, 422, r.status())
	assert.Contains(t, fieldErrors(r), "is_active")

	r = e.call("PATCH", path(self, ""), map[string]any{"role": "MANAGEMENT"}, withBearer(admin))
	assert.Equal(t, 422, r.status())
	assert.Contains(t, fieldErrors(r), "role")

	r = e.call("POST", path(self, "/reset-password"), nil, withBearer(admin))
	assert.Equal(t, 422, r.status())

	// Mengubah nama sendiri tetap boleh.
	assert.Equal(t, 200, e.call("PATCH", path(self, ""), map[string]any{"name": "Nama Baru"}, withBearer(admin)).status())
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM users WHERE email = 'admin@x.id' AND is_active AND role = 'ADMIN'`))
}

func TestUsers_LastActiveAdminCannotBeDeactivatedOrDemoted(t *testing.T) {
	e := newEnv(t)
	adminA := e.adminToken("a@x.id")
	adminB := e.adminToken("b@x.id")
	idA := int64(e.count(`SELECT id FROM users WHERE email = 'a@x.id'`))
	idB := int64(e.count(`SELECT id FROM users WHERE email = 'b@x.id'`))

	// Dua admin aktif: A boleh menonaktifkan B.
	require.Equal(t, 200, e.call("PATCH", path(idB, ""), map[string]any{"is_active": false}, withBearer(adminA)).status())

	// Kini A satu-satunya. Token B (stateless, masih berlaku ≤15 menit) mencoba menyingkirkan A.
	r := e.call("PATCH", path(idA, ""), map[string]any{"is_active": false}, withBearer(adminB))
	assert.Equal(t, 409, r.status())
	assert.Equal(t, "LAST_ADMIN", r.code())

	r = e.call("PATCH", path(idA, ""), map[string]any{"role": "MANAGEMENT"}, withBearer(adminB))
	assert.Equal(t, 409, r.status())
	assert.Equal(t, "LAST_ADMIN", r.code())

	assert.Equal(t, 1, e.count(`SELECT count(*) FROM users WHERE role = 'ADMIN' AND is_active`))
}

func TestUsers_TwoAdminsDeactivatingEachOtherConcurrently_OneAlwaysSurvives(t *testing.T) {
	e := newEnv(t)
	for round := 0; round < 4; round++ {
		a := fmt.Sprintf("a%d@x.id", round)
		b := fmt.Sprintf("b%d@x.id", round)
		tokA, tokB := e.adminToken(a), e.adminToken(b)
		idA := int64(e.count(`SELECT id FROM users WHERE email = $1`, a))
		idB := int64(e.count(`SELECT id FROM users WHERE email = $1`, b))
		// Sisakan tepat dua admin aktif untuk ronde ini.
		e.exec(`UPDATE users SET is_active = FALSE WHERE role = 'ADMIN' AND id NOT IN ($1, $2)`, idA, idB)

		var wg sync.WaitGroup
		codes := make([]int, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			codes[0] = e.call("PATCH", path(idB, ""), map[string]any{"is_active": false}, withBearer(tokA)).status()
		}()
		go func() {
			defer wg.Done()
			codes[1] = e.call("PATCH", path(idA, ""), map[string]any{"is_active": false}, withBearer(tokB)).status()
		}()
		wg.Wait()

		assert.ElementsMatch(t, []int{200, 409}, codes, "ronde %d: tepat satu berhasil", round)
		assert.Equal(t, 1, e.count(`SELECT count(*) FROM users WHERE role = 'ADMIN' AND is_active`),
			"ronde %d: harus tersisa tepat satu ADMIN aktif", round)
	}
}

// ================================================================ reset password

func TestUsers_ResetPassword_FullFlow(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	id := e.addUser("lupa@x.id", domain.RoleStaffWarehouse)
	_, rt := e.mustLogin("lupa@x.id")

	r := e.call("POST", path(id, "/reset-password"), nil, withBearer(admin))

	require.Equal(t, 200, r.status(), r.rec.Body.String())
	temp := r.str("temporary_password")
	assert.Len(t, temp, 16)

	assert.Equal(t, 401, e.login("lupa@x.id", testPassword).status(), "password lama tidak berlaku lagi")
	assert.Equal(t, 401, e.refresh(rt).status(), "semua sesi lama dicabut")

	login := e.login("lupa@x.id", temp)
	require.Equal(t, 200, login.status())
	assert.Equal(t, true, login.data()["user"].(map[string]any)["must_change_password"], "wajib ganti password")

	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'USER_PASSWORD_RESET' AND entity_id = $1`, id))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM audit_logs WHERE after_data::text LIKE '%' || $1 || '%'`, temp), "password tidak boleh masuk audit")

	again := e.call("POST", path(id, "/reset-password"), nil, withBearer(admin))
	assert.NotEqual(t, temp, again.str("temporary_password"), "setiap reset menghasilkan password baru")

	assert.Equal(t, 404, e.call("POST", path(99999, "/reset-password"), nil, withBearer(admin)).status())
}

func TestUsers_ResetPassword_ClearsMustChangeOnlyAfterUserChangesIt(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	id := e.addUser("u@x.id", domain.RoleDriver)

	temp := e.call("POST", path(id, "/reset-password"), nil, withBearer(admin)).str("temporary_password")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM users WHERE id = $1 AND must_change_password`, id))

	access := e.login("u@x.id", temp).str("access_token")
	require.Equal(t, 200, e.call("POST", "/api/v1/auth/change-password",
		map[string]string{"current_password": temp, "new_password": "Sandi-Baru-2026"}, withBearer(access)).status())

	assert.Equal(t, 1, e.count(`SELECT count(*) FROM users WHERE id = $1 AND NOT must_change_password`, id))
}

// ================================================================ clock

func TestUsers_DeactivationTimestampsUseInjectedClock(t *testing.T) {
	e := newEnv(t)
	admin := e.adminToken("admin@x.id")
	id := e.addUser("u@x.id", domain.RoleDriver)
	e.mustLogin("u@x.id")

	e.clk.Advance(2 * time.Hour)
	require.Equal(t, 200, e.call("PATCH", path(id, ""), map[string]any{"is_active": false}, withBearer(adminAfter(e, admin))).status())

	var revokedAt time.Time
	require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT revoked_at FROM refresh_tokens WHERE user_id = $1`, id).Scan(&revokedAt))
	assert.WithinDuration(t, e.clk.Now(), revokedAt, time.Second)
}

// adminAfter: access token 15 menit sudah kedaluwarsa setelah jam maju 2 jam; login ulang admin.
func adminAfter(e *env, _ string) string {
	e.t.Helper()
	r := e.login("admin@x.id", testPassword)
	if r.status() != nethttp.StatusOK {
		e.t.Fatalf("login ulang admin gagal: %s", r.rec.Body.String())
	}
	return r.str("access_token")
}
