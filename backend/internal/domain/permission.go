package domain

import "sort"

// Permission adalah kunci hak akses (plan Bagian 5.1 & kolom "Perm" Bagian 8).
// FE memakai daftar yang sama dari GET /auth/me; otorisasi sebenarnya tetap di backend.
type Permission string

const (
	// Master & inventory
	PermMasterRead    Permission = "master.read"  // gudang, rak, kategori, supplier
	PermMasterWrite   Permission = "master.write" // C master (HEAD_WAREHOUSE)
	PermRackWrite     Permission = "rack.write"   // STAFF_WAREHOUSE boleh C rak
	PermProductRead   Permission = "product.read" // TECHNICIAN hanya R produk
	PermInventoryRead Permission = "inventory.read"
	PermLedgerRead    Permission = "ledger.read"

	// LPB
	PermLPBRead    Permission = "lpb.read"
	PermLPBCreate  Permission = "lpb.create"
	PermLPBSubmit  Permission = "lpb.submit"
	PermLPBApprove Permission = "lpb.approve"
	PermLPBPrint   Permission = "lpb.print" // DRIVER: hanya tugasnya (cek kepemilikan di usecase)

	// Stock opname
	PermOpnameRead     Permission = "opname.read"
	PermOpnameCreate   Permission = "opname.create"
	PermOpnameFinalize Permission = "opname.finalize"

	// Komersial
	PermCustomerRead    Permission = "customer.read"
	PermCustomerWrite   Permission = "customer.write"
	PermContractRead    Permission = "contract.read"
	PermContractWrite   Permission = "contract.write"
	PermContractApprove Permission = "contract.approve"
	PermSORead          Permission = "so.read"
	PermSOWrite         Permission = "so.write"
	PermSOApprove       Permission = "so.approve"
	PermDORead          Permission = "do.read"
	PermDOCreate        Permission = "do.create"
	PermDOReserve       Permission = "do.reserve"
	PermDeliveryAssign  Permission = "delivery.assign"
	PermInvoiceRead     Permission = "invoice.read"
	PermInvoiceCreate   Permission = "invoice.create"
	PermInvoiceApprove  Permission = "invoice.approve"
	PermInvoicePay      Permission = "invoice.pay"

	// Lapangan
	PermDriver       Permission = "driver" // tugas antar/jemput, checkpoint, POD (milik sendiri)
	PermTrackingRead Permission = "tracking.read"
	PermMapRead      Permission = "map.read"
	PermTicketRead   Permission = "ticket.read"
	PermTicketWrite  Permission = "ticket.write"
	PermTicketWork   Permission = "ticket.work"

	// Umum
	PermDashboardRead Permission = "dashboard.read"
	PermReportRead    Permission = "report.read"
	PermAuditRead     Permission = "audit.read"

	// Hanya ADMIN (ADMIN otomatis punya semua; daftar role kosong)
	PermUserManage     Permission = "user.manage"
	PermSettingsManage Permission = "settings.manage"
	PermReconcileRead  Permission = "reconcile.read"
	PermJobsRead       Permission = "jobs.read"
)

// permissionRoles: permission → role selain ADMIN yang memilikinya.
// Satu-satunya sumber kebenaran matriks 5.1; ubah di sini, bukan di router.
var permissionRoles = buildPermissionRoles()

func buildPermissionRoles() map[Permission][]Role {
	var (
		m  = RoleManagement
		b  = RoleBusinessAdmin
		s  = RoleStaffWarehouse
		h  = RoleHeadWarehouse
		d  = RoleDriver
		tk = RoleTechnician
	)
	return map[Permission][]Role{
		PermMasterRead:    {m, b, s, h},
		PermMasterWrite:   {h},
		PermRackWrite:     {s, h},
		PermProductRead:   {m, b, s, h, tk},
		PermInventoryRead: {m, b, s, h, tk},
		PermLedgerRead:    {m, s, h},

		PermLPBRead:    {m, s, h},
		PermLPBCreate:  {s, h},
		PermLPBSubmit:  {s, h},
		PermLPBApprove: {h},
		PermLPBPrint:   {m, b, s, h, d},

		PermOpnameRead:     {m, s, h},
		PermOpnameCreate:   {s, h},
		PermOpnameFinalize: {h},

		PermCustomerRead:    {m, b, tk},
		PermCustomerWrite:   {b},
		PermContractRead:    {m, b},
		PermContractWrite:   {b},
		PermContractApprove: {b},
		PermSORead:          {m, b},
		PermSOWrite:         {b},
		PermSOApprove:       {b},
		PermDORead:          {m, b, s, h},
		PermDOCreate:        {b},
		PermDOReserve:       {s, h},
		PermDeliveryAssign:  {b, s, h},
		PermInvoiceRead:     {m, b},
		PermInvoiceCreate:   {b},
		PermInvoiceApprove:  {b},
		PermInvoicePay:      {b},

		PermDriver:       {d},
		PermTrackingRead: {m, b, h},
		PermMapRead:      {m, b, h, tk},
		PermTicketRead:   {m, b, tk},
		PermTicketWrite:  {b, tk},
		PermTicketWork:   {tk},

		PermDashboardRead: {m, b, s, h},
		PermReportRead:    {m},
		PermAuditRead:     {m},

		PermUserManage:     {},
		PermSettingsManage: {},
		PermReconcileRead:  {},
		PermJobsRead:       {},
	}
}

// IsKnownPermission true bila p terdaftar di matriks. RequirePermission memakainya
// agar salah ketik kunci gagal saat startup, bukan diam-diam menolak/menerima semua.
func IsKnownPermission(p Permission) bool {
	_, ok := permissionRoles[p]
	return ok
}

// Can true bila role memiliki permission. ADMIN memiliki semua permission yang dikenal.
// Permission/role tidak dikenal → false (fail closed).
func Can(role Role, p Permission) bool {
	roles, ok := permissionRoles[p]
	if !ok || !role.Valid() {
		return false
	}
	if role == RoleAdmin {
		return true
	}
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// PermissionsFor mengembalikan seluruh permission milik role, terurut (untuk GET /auth/me).
func PermissionsFor(role Role) []Permission {
	out := make([]Permission, 0, len(permissionRoles))
	for p := range permissionRoles {
		if Can(role, p) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
