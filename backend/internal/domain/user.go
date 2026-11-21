package domain

import "time"

// Role adalah peran pengguna (CHECK constraint di tabel users).
type Role string

const (
	RoleAdmin          Role = "ADMIN"
	RoleManagement     Role = "MANAGEMENT"
	RoleBusinessAdmin  Role = "BUSINESS_ADMIN"
	RoleStaffWarehouse Role = "STAFF_WAREHOUSE"
	RoleHeadWarehouse  Role = "HEAD_WAREHOUSE"
	RoleDriver         Role = "DRIVER"
	RoleTechnician     Role = "TECHNICIAN"
)

var allRoles = []Role{
	RoleAdmin, RoleManagement, RoleBusinessAdmin, RoleStaffWarehouse,
	RoleHeadWarehouse, RoleDriver, RoleTechnician,
}

// Roles mengembalikan salinan daftar seluruh role.
func Roles() []Role { return append([]Role(nil), allRoles...) }

// Valid true bila r adalah role yang dikenal.
func (r Role) Valid() bool {
	for _, x := range allRoles {
		if r == x {
			return true
		}
	}
	return false
}

// DefaultHome adalah rute FE tujuan setelah login, diturunkan dari "Role utama"
// pada plan Bagian 9.4. FE boleh fallback ke /dashboard untuk halaman yang belum ada.
func (r Role) DefaultHome() string {
	switch r {
	case RoleStaffWarehouse:
		return "/lpb"
	case RoleHeadWarehouse:
		return "/lpb/approvals"
	case RoleBusinessAdmin:
		return "/pelanggan"
	case RoleDriver:
		return "/driver/deliveries"
	case RoleTechnician:
		return "/tickets"
	default: // ADMIN, MANAGEMENT
		return "/dashboard"
	}
}

// User adalah akun pengguna.
type User struct {
	ID                 int64
	Name               string
	Email              string
	Phone              *string
	PasswordHash       string
	Role               Role
	WarehouseID        *int64 // scope gudang; nil = semua gudang
	IsActive           bool
	MustChangePassword bool
	LastLoginAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Principal adalah identitas pemanggil yang sudah terautentikasi (dari access token).
type Principal struct {
	UserID             int64
	Role               Role
	WarehouseID        *int64
	MustChangePassword bool
}

// CanAccessWarehouse menerapkan warehouse scoping (plan Bagian 5.1): ADMIN dan pengguna
// dengan WarehouseID nil boleh semua gudang; selainnya hanya gudangnya sendiri.
func (p Principal) CanAccessWarehouse(warehouseID int64) bool {
	return p.Role == RoleAdmin || p.WarehouseID == nil || *p.WarehouseID == warehouseID
}
