//go:build integration

package http_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
)

func seedCustomerWithSite(t *testing.T, e *env, bizToken, code string) (int64, int64) {
	t.Helper()
	cRes := e.post(bizToken, "/api/v1/customers", map[string]any{
		"customer_code":       code,
		"company_name":        "PT " + code,
		"pic_name":            "Andi",
		"phone":               "08123456789",
		"is_pkp":              true,
		"billing_address":     "Jl. Pemuda 1",
		"billing_postal_code": "50132",
		"payment_term_days":   30,
	})
	require.Equal(t, 201, cRes.status(), cRes.rec.Body.String())
	cid := int64(cRes.data()["id"].(float64))

	sRes := e.post(bizToken, fmt.Sprintf("/api/v1/customers/%d/sites", cid), map[string]any{
		"site_name":    "Kantor Pusat",
		"pic_name":     "Andi",
		"pic_phone":    "08123456789",
		"address_line": "Jl. Pemuda 1",
		"city":         "Semarang",
		"province":     "Jawa Tengah",
		"postal_code":  "50132",
		"is_primary":   true,
	})
	require.Equal(t, 201, sRes.status(), sRes.rec.Body.String())
	sid := int64(sRes.data()["id"].(float64))
	return cid, sid
}

func TestContract_RBACAndValidation(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	mgmt := e.tokenFor(domain.RoleManagement, nil)
	staff := e.tokenFor(domain.RoleStaffWarehouse, nil)
	tech := e.tokenFor(domain.RoleTechnician, nil)

	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-CTR-RBAC")

	validBody := map[string]any{
		"customer_id":     cid,
		"start_date":      "2026-01-01",
		"duration_months": 12,
		"items": []map[string]any{
			{
				"product_id":       f.p1, // ASSET
				"customer_site_id": sid,
				"qty":              5,
				"monthly_rate":     "500000.00",
			},
		},
	}

	// STAFF & TECH tidak punya contract.read.
	assert.Equal(t, 403, e.get(staff, "/api/v1/contracts").status())
	assert.Equal(t, 403, e.get(tech, "/api/v1/contracts").status())
	// MANAGEMENT boleh baca, tapi tidak boleh buat.
	assert.Equal(t, 200, e.get(mgmt, "/api/v1/contracts").status())
	assert.Equal(t, 403, e.post(mgmt, "/api/v1/contracts", validBody).status())

	// Validasi: produk CONSUMABLE (f.p2) ditolak di kontrak sewa (422).
	badProd := map[string]any{
		"customer_id":     cid,
		"start_date":      "2026-01-01",
		"duration_months": 12,
		"items": []map[string]any{
			{"product_id": f.p2, "qty": 2, "monthly_rate": "100000.00"},
		},
	}
	assert.Equal(t, 422, e.post(biz, "/api/v1/contracts", badProd).status())

	// Validasi: end_date <= start_date ditolak (422).
	badDates := map[string]any{
		"customer_id":     cid,
		"start_date":      "2026-06-01",
		"end_date":        "2026-05-01",
		"duration_months": 12,
		"items": []map[string]any{
			{"product_id": f.p1, "qty": 1, "monthly_rate": "500000.00"},
		},
	}
	assert.Equal(t, 422, e.post(biz, "/api/v1/contracts", badDates).status())

	// Buat kontrak valid → end_date dihitung otomatis 2026-12-31, estimated_monthly = 2500000.00.
	res := e.post(biz, "/api/v1/contracts", validBody)
	require.Equal(t, 201, res.status(), res.rec.Body.String())
	d := res.data()
	assert.Equal(t, "DRAFT", d["status"])
	assert.Equal(t, "2026-01-01", d["start_date"])
	assert.Equal(t, "2026-12-31", d["end_date"])
	assert.Equal(t, "2500000.00", d["estimated_monthly"])
	assert.EqualValues(t, 5, d["total_qty"])
	assert.Contains(t, d["contract_number"].(string), "KTR/GLOBAL/")
}

func TestContract_Lifecycle_DraftUpdateActivateRenewTerminate(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-CTR-LIFE")

	// 1. Create DRAFT.
	createRes := e.post(biz, "/api/v1/contracts", map[string]any{
		"customer_id":          cid,
		"start_date":           "2026-01-01",
		"end_date":             "2026-12-31",
		"duration_months":      12,
		"billing_day_of_month": 5,
		"billing_mode":         "PRORATA",
		"deposit_amount":       "1000000.00",
		"items": []map[string]any{
			{"product_id": f.p1, "customer_site_id": sid, "qty": 4, "monthly_rate": "500000.00"},
		},
	})
	require.Equal(t, 201, createRes.status(), createRes.rec.Body.String())
	ctrID := int64(createRes.data()["id"].(float64))

	// 2. Update DRAFT (ubah qty & tarif).
	updRes := e.patchReq(biz, fmt.Sprintf("/api/v1/contracts/%d", ctrID), map[string]any{
		"billing_mode": "FULL_MONTH",
		"items": []map[string]any{
			{"product_id": f.p1, "customer_site_id": sid, "qty": 6, "monthly_rate": "550000.00"},
		},
	})
	require.Equal(t, 200, updRes.status(), updRes.rec.Body.String())
	assert.Equal(t, "FULL_MONTH", updRes.data()["billing_mode"])
	assert.Equal(t, "3300000.00", updRes.data()["estimated_monthly"])

	// DRAFT tidak bisa langsung di-terminate atau di-renew (409).
	assert.Equal(t, 409, e.post(biz, fmt.Sprintf("/api/v1/contracts/%d/terminate", ctrID), map[string]any{
		"termination_reason": "Batal sebelum aktif",
	}).status())
	assert.Equal(t, 409, e.post(biz, fmt.Sprintf("/api/v1/contracts/%d/renew", ctrID), nil).status())

	// 3. Activate DRAFT → ACTIVE.
	actRes := e.post(biz, fmt.Sprintf("/api/v1/contracts/%d/activate", ctrID), nil)
	require.Equal(t, 200, actRes.status(), actRes.rec.Body.String())
	assert.Equal(t, "ACTIVE", actRes.data()["status"])
	assert.NotNil(t, actRes.data()["activated_at"])

	// Kontrak ACTIVE tidak bisa diubah via PATCH (409).
	assert.Equal(t, 409, e.patchReq(biz, fmt.Sprintf("/api/v1/contracts/%d", ctrID), map[string]any{
		"notes": "Coba ubah saat aktif",
	}).status())

	// 4. Renew ACTIVE → buat kontrak DRAFT baru berlanjut dari 2027-01-01 s/d 2027-12-31.
	renRes := e.post(biz, fmt.Sprintf("/api/v1/contracts/%d/renew", ctrID), map[string]any{
		"notes": "Perpanjangan tahun kedua",
	})
	require.Equal(t, 201, renRes.status(), renRes.rec.Body.String())
	renData := renRes.data()
	assert.Equal(t, "DRAFT", renData["status"])
	assert.EqualValues(t, ctrID, renData["renewed_from_id"])
	assert.Equal(t, "2027-01-01", renData["start_date"])
	assert.Equal(t, "2027-12-31", renData["end_date"])
	assert.Equal(t, "3300000.00", renData["estimated_monthly"])
	assert.Len(t, renData["items"].([]any), 1)

	// 5. Terminate ACTIVE → TERMINATED (alasan wajib; tanggal terminasi < start_date ditolak).
	assert.Equal(t, 422, e.post(biz, fmt.Sprintf("/api/v1/contracts/%d/terminate", ctrID), map[string]any{
		"terminated_at":      "2025-12-01",
		"termination_reason": "Tanggal sebelum mulai",
	}).status())

	termRes := e.post(biz, fmt.Sprintf("/api/v1/contracts/%d/terminate", ctrID), map[string]any{
		"terminated_at":      "2026-06-30",
		"termination_reason": "Pelanggan menutup cabang lebih awal",
	})
	require.Equal(t, 200, termRes.status(), termRes.rec.Body.String())
	assert.Equal(t, "TERMINATED", termRes.data()["status"])
	assert.Equal(t, "2026-06-30", termRes.data()["terminated_at"])
	assert.Equal(t, "Pelanggan menutup cabang lebih awal", termRes.data()["termination_reason"])
}
