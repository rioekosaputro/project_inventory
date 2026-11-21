package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCan_AdminHasEveryKnownPermission(t *testing.T) {
	for p := range permissionRoles {
		assert.True(t, Can(RoleAdmin, p), string(p))
	}
}

func TestCan_FailsClosedForUnknownPermissionAndRole(t *testing.T) {
	assert.False(t, Can(RoleAdmin, "lpb.aprove"), "typo pada permission tidak boleh lolos, bahkan untuk ADMIN")
	assert.False(t, Can("SUPERUSER", PermLPBApprove))
	assert.False(t, Can("", PermLPBApprove))
}

// Memverifikasi baris-baris kunci matriks plan Bagian 5.1.
func TestMatrix_KeyRulesFromPlan(t *testing.T) {
	tests := []struct {
		role Role
		perm Permission
		want bool
	}{
		// LPB approve hanya HEAD_WAREHOUSE (+ADMIN); staff hanya membuat & submit.
		{RoleHeadWarehouse, PermLPBApprove, true},
		{RoleStaffWarehouse, PermLPBApprove, false},
		{RoleManagement, PermLPBApprove, false},
		{RoleBusinessAdmin, PermLPBApprove, false},
		{RoleStaffWarehouse, PermLPBCreate, true},
		{RoleStaffWarehouse, PermLPBSubmit, true},
		{RoleManagement, PermLPBCreate, false}, // MGMT hanya R

		// Opname finalisasi hanya kepala gudang.
		{RoleHeadWarehouse, PermOpnameFinalize, true},
		{RoleStaffWarehouse, PermOpnameFinalize, false},
		{RoleStaffWarehouse, PermOpnameCreate, true},

		// Komersial hanya BUSINESS_ADMIN (MGMT read-only).
		{RoleBusinessAdmin, PermContractApprove, true},
		{RoleManagement, PermContractApprove, false},
		{RoleManagement, PermContractRead, true},
		{RoleHeadWarehouse, PermContractRead, false},
		{RoleBusinessAdmin, PermInvoicePay, true},
		{RoleManagement, PermInvoicePay, false},
		{RoleStaffWarehouse, PermInvoiceRead, false},

		// Driver: hanya tugasnya + cetak surat jalan; tidak ada akses stok/master.
		{RoleDriver, PermDriver, true},
		{RoleDriver, PermLPBPrint, true},
		{RoleDriver, PermInventoryRead, false},
		{RoleDriver, PermMasterRead, false},
		{RoleDriver, PermDashboardRead, false},
		{RoleStaffWarehouse, PermDriver, false},

		// Teknisi: R produk, bukan master lain; mengerjakan tiket.
		{RoleTechnician, PermProductRead, true},
		{RoleTechnician, PermMasterRead, false},
		{RoleTechnician, PermTicketWork, true},
		{RoleBusinessAdmin, PermTicketWork, false},
		{RoleTechnician, PermLPBRead, false},

		// Staff boleh C rak tetapi bukan master lain.
		{RoleStaffWarehouse, PermRackWrite, true},
		{RoleStaffWarehouse, PermMasterWrite, false},
		{RoleHeadWarehouse, PermMasterWrite, true},

		// "User & role, app settings": tidak ada role selain ADMIN.
		{RoleManagement, PermUserManage, false},
		{RoleHeadWarehouse, PermSettingsManage, false},
		{RoleAdmin, PermUserManage, true},

		// Audit: admin & management.
		{RoleManagement, PermAuditRead, true},
		{RoleBusinessAdmin, PermAuditRead, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, Can(tt.role, tt.perm), "%s / %s", tt.role, tt.perm)
	}
}

func TestPermissionsFor_SortedAndConsistentWithCan(t *testing.T) {
	for _, r := range Roles() {
		perms := PermissionsFor(r)
		for i, p := range perms {
			assert.True(t, Can(r, p))
			if i > 0 {
				assert.Less(t, string(perms[i-1]), string(p), "harus terurut tanpa duplikat")
			}
		}
	}
	assert.Len(t, PermissionsFor(RoleAdmin), len(permissionRoles))
	assert.Empty(t, PermissionsFor("SUPERUSER"))
}

func TestEveryRoleHasAtLeastOnePermissionAndAHome(t *testing.T) {
	for _, r := range Roles() {
		assert.NotEmpty(t, PermissionsFor(r), r)
		assert.NotEmpty(t, r.DefaultHome(), r)
	}
}

func TestPrincipal_CanAccessWarehouse(t *testing.T) {
	wh1, wh2 := int64(1), int64(2)

	assert.True(t, Principal{Role: RoleHeadWarehouse, WarehouseID: nil}.CanAccessWarehouse(9), "tanpa scope = semua gudang")
	assert.True(t, Principal{Role: RoleHeadWarehouse, WarehouseID: &wh1}.CanAccessWarehouse(1))
	assert.False(t, Principal{Role: RoleHeadWarehouse, WarehouseID: &wh1}.CanAccessWarehouse(2))
	assert.True(t, Principal{Role: RoleAdmin, WarehouseID: &wh2}.CanAccessWarehouse(1), "ADMIN tidak dibatasi")
}
