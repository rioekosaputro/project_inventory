//go:build integration

package http_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
)

func TestCustomer_RBAC(t *testing.T) {
	e := newEnv(t)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	mgmt := e.tokenFor(domain.RoleManagement, nil)
	tech := e.tokenFor(domain.RoleTechnician, nil)
	staff := e.tokenFor(domain.RoleStaffWarehouse, nil)
	driver := e.tokenFor(domain.RoleDriver, nil)

	payload := map[string]any{
		"customer_code":       "CUST-RBAC-01",
		"company_name":        "PT Sinar Mandiri",
		"pic_name":            "Budi Santoso",
		"phone":               "081234567890",
		"billing_address":     "Jl. Pemuda No. 10, Semarang",
		"billing_postal_code": "50132",
	}

	// BUSINESS_ADMIN boleh membuat pelanggan & site.
	res := e.post(biz, "/api/v1/customers", payload)
	require.Equal(t, 201, res.status(), res.rec.Body.String())
	cid := int64(res.data()["id"].(float64))

	// MANAGEMENT & TECHNICIAN hanya baca.
	assert.Equal(t, 200, e.get(mgmt, "/api/v1/customers").status())
	assert.Equal(t, 200, e.get(tech, fmt.Sprintf("/api/v1/customers/%d", cid)).status())
	assert.Equal(t, 200, e.get(tech, fmt.Sprintf("/api/v1/customers/%d/assets", cid)).status())
	assert.Equal(t, 403, e.post(mgmt, "/api/v1/customers", payload).status())
	assert.Equal(t, 403, e.patchReq(tech, fmt.Sprintf("/api/v1/customers/%d", cid), map[string]any{"pic_name": "X"}).status())

	// STAFF_WAREHOUSE & DRIVER tidak punya akses pelanggan.
	assert.Equal(t, 403, e.get(staff, "/api/v1/customers").status())
	assert.Equal(t, 403, e.get(driver, "/api/v1/customers").status())
}

func TestCustomer_CRUDAndValidation(t *testing.T) {
	e := newEnv(t)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)

	// Buat pelanggan PKP.
	createRes := e.post(biz, "/api/v1/customers", map[string]any{
		"customer_code":       "cust-001",
		"company_name":        "PT Maju Bersama",
		"pic_name":            "Siti Rahayu",
		"phone":               "024-8310001",
		"email":               "siti@majubersama.co.id",
		"is_pkp":              true,
		"npwp":                "01.234.567.8-504.000",
		"tax_address":         "Jl. Pandanaran No. 50, Semarang",
		"billing_address":     "Jl. Pandanaran No. 50, Semarang",
		"billing_postal_code": "50241",
		"billing_email":       "finance@majubersama.co.id",
		"payment_term_days":   45,
	})
	require.Equal(t, 201, createRes.status(), createRes.rec.Body.String())
	d := createRes.data()
	cid := int64(d["id"].(float64))
	assert.Equal(t, "CUST-001", d["customer_code"])
	assert.Equal(t, true, d["is_pkp"])
	assert.EqualValues(t, 45, d["payment_term_days"])

	// Kode duplikat → 409.
	dup := e.post(biz, "/api/v1/customers", map[string]any{
		"customer_code":       "CUST-001",
		"company_name":        "PT Lain",
		"pic_name":            "Andi",
		"phone":               "0812000",
		"billing_address":     "Jl. A",
		"billing_postal_code": "50111",
	})
	assert.Equal(t, 409, dup.status())

	// Pelanggan non-PKP.
	nonPKP := e.post(biz, "/api/v1/customers", map[string]any{
		"customer_code":       "CUST-002",
		"company_name":        "CV Berkah Jaya",
		"pic_name":            "Dedi",
		"phone":               "0812111",
		"is_pkp":              false,
		"billing_address":     "Jl. Ahmad Yani No. 2",
		"billing_postal_code": "50242",
	})
	require.Equal(t, 201, nonPKP.status(), nonPKP.rec.Body.String())

	// Filter is_pkp=false & q.
	listRes := e.get(biz, "/api/v1/customers?is_pkp=false&q=Berkah")
	require.Equal(t, 200, listRes.status())
	items := listRes.list()
	require.Len(t, items, 1)
	assert.Equal(t, "CUST-002", items[0]["customer_code"])

	// Update parsial + mengosongkan email ke null.
	patchRes := e.patchReq(biz, fmt.Sprintf("/api/v1/customers/%d", cid), map[string]any{
		"company_name":      "PT Maju Bersama Tbk",
		"email":             nil,
		"payment_term_days": 30,
	})
	require.Equal(t, 200, patchRes.status(), patchRes.rec.Body.String())
	assert.Equal(t, "PT Maju Bersama Tbk", patchRes.data()["company_name"])
	assert.Nil(t, patchRes.data()["email"])
	assert.EqualValues(t, 30, patchRes.data()["payment_term_days"])

	// Validasi email tidak valid → 422.
	badEmail := e.patchReq(biz, fmt.Sprintf("/api/v1/customers/%d", cid), map[string]any{
		"billing_email": "bukan-email",
	})
	assert.Equal(t, 422, badEmail.status())
}

func TestCustomer_SitesPrimaryAndCoordinates(t *testing.T) {
	e := newEnv(t)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)

	cRes := e.post(biz, "/api/v1/customers", map[string]any{
		"customer_code":       "CUST-SITE-01",
		"company_name":        "PT Nusantara Logistik",
		"pic_name":            "Rina",
		"phone":               "081299998888",
		"billing_address":     "Jl. Pemuda 1",
		"billing_postal_code": "50132",
	})
	require.Equal(t, 201, cRes.status())
	cid := int64(cRes.data()["id"].(float64))

	// Validasi koordinat: latitude tanpa longitude → 422.
	badCoord := e.post(biz, fmt.Sprintf("/api/v1/customers/%d/sites", cid), map[string]any{
		"site_name":    "Kantor Pusat",
		"pic_name":     "Rina",
		"pic_phone":    "081299998888",
		"address_line": "Jl. Pemuda 1",
		"city":         "Semarang",
		"province":     "Jawa Tengah",
		"postal_code":  "50132",
		"latitude":     -6.9825,
	})
	assert.Equal(t, 422, badCoord.status())

	// Buat site 1 sebagai site utama dengan koordinat lengkap.
	s1Res := e.post(biz, fmt.Sprintf("/api/v1/customers/%d/sites", cid), map[string]any{
		"site_name":    "Kantor Pusat",
		"pic_name":     "Rina",
		"pic_phone":    "081299998888",
		"address_line": "Jl. Pemuda 1",
		"city":         "Semarang",
		"province":     "Jawa Tengah",
		"postal_code":  "50132",
		"latitude":     -6.9825,
		"longitude":    110.4091,
		"is_primary":   true,
	})
	require.Equal(t, 201, s1Res.status(), s1Res.rec.Body.String())
	s1ID := int64(s1Res.data()["id"].(float64))
	assert.Equal(t, true, s1Res.data()["is_primary"])

	// Buat site 2 juga is_primary=true → site 1 otomatis diturunkan jadi non-utama (uq_customer_primary_site terjaga).
	s2Res := e.post(biz, fmt.Sprintf("/api/v1/customers/%d/sites", cid), map[string]any{
		"site_name":    "Cabang Solo",
		"pic_name":     "Hendra",
		"pic_phone":    "081377776666",
		"address_line": "Jl. Slamet Riyadi 100",
		"city":         "Surakarta",
		"province":     "Jawa Tengah",
		"postal_code":  "57141",
		"latitude":     -7.5666,
		"longitude":    110.8167,
		"is_primary":   true,
	})
	require.Equal(t, 201, s2Res.status(), s2Res.rec.Body.String())
	s2ID := int64(s2Res.data()["id"].(float64))

	// Cek daftar site: hanya site 2 yang is_primary=true.
	sitesRes := e.get(biz, fmt.Sprintf("/api/v1/customers/%d/sites", cid))
	require.Equal(t, 200, sitesRes.status())
	sites := sitesRes.list()
	require.Len(t, sites, 2)
	assert.EqualValues(t, s2ID, sites[0]["id"])
	assert.Equal(t, true, sites[0]["is_primary"])
	assert.EqualValues(t, s1ID, sites[1]["id"])
	assert.Equal(t, false, sites[1]["is_primary"])

	// Pindahkan is_primary kembali ke site 1 lewat PATCH /customer-sites/:id.
	promoteRes := e.patchReq(biz, fmt.Sprintf("/api/v1/customer-sites/%d", s1ID), map[string]any{
		"is_primary": true,
	})
	require.Equal(t, 200, promoteRes.status(), promoteRes.rec.Body.String())
	assert.Equal(t, true, promoteRes.data()["is_primary"])

	// Nonaktifkan site 1 → is_primary otomatis false.
	deactRes := e.patchReq(biz, fmt.Sprintf("/api/v1/customer-sites/%d", s1ID), map[string]any{
		"is_active": false,
	})
	require.Equal(t, 200, deactRes.status(), deactRes.rec.Body.String())
	assert.Equal(t, false, deactRes.data()["is_active"])
	assert.Equal(t, false, deactRes.data()["is_primary"])
}

func TestCustomer_InstalledAssetsAndDeactivateGuard(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)

	// Buat pelanggan & site.
	cRes := e.post(biz, "/api/v1/customers", map[string]any{
		"customer_code":       "CUST-AST-01",
		"company_name":        "PT Bank Daerah",
		"pic_name":            "Tono",
		"phone":               "024-3540000",
		"billing_address":     "Jl. Pemuda 142",
		"billing_postal_code": "50132",
	})
	require.Equal(t, 201, cRes.status())
	cid := int64(cRes.data()["id"].(float64))

	sRes := e.post(biz, fmt.Sprintf("/api/v1/customers/%d/sites", cid), map[string]any{
		"site_name":    "Cabang Pemuda",
		"pic_name":     "Tono",
		"pic_phone":    "024-3540000",
		"address_line": "Jl. Pemuda 142",
		"city":         "Semarang",
		"province":     "Jawa Tengah",
		"postal_code":  "50132",
		"is_primary":   true,
	})
	require.Equal(t, 201, sRes.status())
	sid := int64(sRes.data()["id"].(float64))

	// Masukkan 1 unit berserial berstatus RENTED yang terpasang di pelanggan & site tersebut.
	e.exec(`INSERT INTO product_serials (product_id, serial_number, asset_tag, status, condition, current_customer_id, current_site_id)
	        VALUES ($1, 'SN-RENT-001', 'AST-RENT-001', 'RENTED', 'GOOD', $2, $3)`, f.p1, cid, sid)

	// GET /customers/:id/assets menampilkan unit terpasang.
	assetsRes := e.get(biz, fmt.Sprintf("/api/v1/customers/%d/assets", cid))
	require.Equal(t, 200, assetsRes.status(), assetsRes.rec.Body.String())
	assets := assetsRes.list()
	require.Len(t, assets, 1)
	assert.Equal(t, "SN-RENT-001", assets[0]["serial_number"])
	assert.Equal(t, "Cabang Pemuda", assets[0]["site_name"])

	// GET /customers/:id mengembalikan ringkasan site_count=1 & active_asset_count=1.
	detRes := e.get(biz, fmt.Sprintf("/api/v1/customers/%d", cid))
	require.Equal(t, 200, detRes.status())
	assert.EqualValues(t, 1, detRes.data()["site_count"])
	assert.EqualValues(t, 1, detRes.data()["active_asset_count"])
	assert.Len(t, detRes.data()["sites"].([]any), 1)

	// Menonaktifkan site atau pelanggan yang masih punya unit RENTED ditolak (422).
	assert.Equal(t, 422, e.patchReq(biz, fmt.Sprintf("/api/v1/customer-sites/%d", sid), map[string]any{"is_active": false}).status())
	assert.Equal(t, 422, e.patchReq(biz, fmt.Sprintf("/api/v1/customers/%d", cid), map[string]any{"is_active": false}).status())
}
