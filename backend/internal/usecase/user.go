package usecase

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/password"
)

const (
	maxNameLen  = 100
	maxEmailLen = 120
	maxPhoneLen = 30

	tempPasswordLen = 16
)

// UserUsecase: manajemen user oleh ADMIN (permission user.manage ditegakkan di router).
// Tidak ada hard delete: akun dinonaktifkan (plan Bagian 4).
type UserUsecase struct {
	tx         domain.TxManager
	users      domain.UserRepository
	tokens     domain.RefreshTokenRepository
	warehouses domain.WarehouseRepository
	audit      domain.AuditRepository
	clock      clock.Clock
}

func NewUserUsecase(
	tx domain.TxManager,
	users domain.UserRepository,
	tokens domain.RefreshTokenRepository,
	warehouses domain.WarehouseRepository,
	audit domain.AuditRepository,
	clk clock.Clock,
) *UserUsecase {
	return &UserUsecase{tx: tx, users: users, tokens: tokens, warehouses: warehouses, audit: audit, clock: clk}
}

type CreateUserInput struct {
	Name        string
	Email       string
	Phone       *string
	Role        domain.Role
	WarehouseID *int64
}

// UpdateUserInput adalah perubahan parsial. Field nil = tidak diubah. Untuk field yang boleh
// dikosongkan (phone, warehouse_id) ada penanda Set* agar "dikosongkan" ≠ "tidak dikirim".
type UpdateUserInput struct {
	Name           *string
	SetPhone       bool
	Phone          *string
	Role           *domain.Role
	SetWarehouseID bool
	WarehouseID    *int64
	IsActive       *bool
}

// CreatedUser memuat password sementara yang HANYA tersedia saat ini (di DB hanya hash).
type CreatedUser struct {
	User              *domain.User
	TemporaryPassword string
}

// ---------------------------------------------------------------- Create

// Create membuat akun dengan password sementara acak dan must_change_password = TRUE.
func (u *UserUsecase) Create(ctx context.Context, in CreateUserInput) (*CreatedUser, error) {
	email := normalizeEmail(in.Email)
	name := strings.TrimSpace(in.Name)
	phone := cleanPhone(in.Phone)

	var errs []domain.FieldError
	errs = append(errs, validateName(name)...)
	errs = append(errs, validateEmail(email)...)
	errs = append(errs, validatePhone(phone)...)
	errs = append(errs, validateRoleWarehouse(in.Role, in.WarehouseID)...)
	if len(errs) > 0 {
		return nil, invalidUser(errs)
	}
	whErrs, err := u.checkWarehouse(ctx, in.WarehouseID)
	if err != nil {
		return nil, err
	}
	if len(whErrs) > 0 {
		return nil, invalidUser(whErrs)
	}

	temp, err := password.Generate(tempPasswordLen)
	if err != nil {
		return nil, err
	}
	hash, err := password.Hash(temp)
	if err != nil {
		return nil, err
	}

	var created *domain.User
	err = u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		var err error
		created, err = u.users.Create(ctx, &domain.User{
			Name: name, Email: email, Phone: phone, PasswordHash: hash, Role: in.Role,
			WarehouseID: in.WarehouseID, IsActive: true, MustChangePassword: true,
		})
		if err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action: domain.AuditUserCreate, EntityType: "user", EntityID: &created.ID, After: auditView(created),
		})
	})
	if err != nil {
		return nil, err
	}
	return &CreatedUser{User: created, TemporaryPassword: temp}, nil
}

// ---------------------------------------------------------------- Read

func (u *UserUsecase) Get(ctx context.Context, id int64) (*domain.User, error) {
	return u.users.FindByID(ctx, id)
}

func (u *UserUsecase) List(ctx context.Context, f domain.UserFilter, p domain.PageParams) ([]domain.User, int64, error) {
	return u.users.List(ctx, f, p)
}

// ---------------------------------------------------------------- Update

// Update mengubah profil/role/gudang/status. Aturan pengaman:
//   - Tidak boleh menonaktifkan atau mengubah role akun SENDIRI (mencegah admin mengunci dirinya).
//   - ADMIN aktif terakhir tidak boleh dinonaktifkan/diturunkan (dijaga dengan lock agar
//     dua admin yang saling menonaktifkan bersamaan tidak lolos dua-duanya).
//   - Menonaktifkan akun mencabut seluruh refresh token-nya di transaksi yang sama.
func (u *UserUsecase) Update(ctx context.Context, actor domain.Principal, id int64, in UpdateUserInput) (*domain.User, error) {
	if errs := validateUpdateShape(in); len(errs) > 0 {
		return nil, invalidUser(errs)
	}
	now := u.clock.Now()

	var result *domain.User
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		// Urutan lock tetap: semua ADMIN aktif (urut id) → baru baris target. Mengunci target
		// lebih dulu bisa deadlock bila dua admin saling menonaktifkan bersamaan.
		peek, err := u.users.FindByID(ctx, id)
		if err != nil {
			return err
		}
		adminCount := 0
		if peek.Role == domain.RoleAdmin {
			if adminCount, err = u.users.LockActiveAdmins(ctx); err != nil {
				return err
			}
		}
		cur, err := u.users.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if cur.Role != peek.Role { // berubah di antara baca dan kunci: minta ulang
			return domain.ErrConflict
		}

		next := *cur
		if in.Name != nil {
			next.Name = strings.TrimSpace(*in.Name)
		}
		if in.SetPhone {
			next.Phone = cleanPhone(in.Phone)
		}
		if in.Role != nil {
			next.Role = *in.Role
		}
		if in.SetWarehouseID {
			next.WarehouseID = in.WarehouseID
		}
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
		}

		var errs []domain.FieldError
		errs = append(errs, validateName(next.Name)...)
		errs = append(errs, validatePhone(next.Phone)...)
		errs = append(errs, validateRoleWarehouse(next.Role, next.WarehouseID)...)
		if in.SetWarehouseID || in.Role != nil { // validasi gudang hanya bila relevan berubah
			if next.WarehouseID != nil && (cur.WarehouseID == nil || *cur.WarehouseID != *next.WarehouseID) {
				whErrs, err := u.checkWarehouse(ctx, next.WarehouseID)
				if err != nil {
					return err
				}
				errs = append(errs, whErrs...)
			}
		}
		if id == actor.UserID {
			if next.Role != cur.Role {
				errs = append(errs, domain.FieldError{Field: "role", Message: "tidak dapat mengubah role akun Anda sendiri"})
			}
			if cur.IsActive && !next.IsActive {
				errs = append(errs, domain.FieldError{Field: "is_active", Message: "tidak dapat menonaktifkan akun Anda sendiri"})
			}
		}
		if len(errs) > 0 {
			return invalidUser(errs)
		}

		losesAdmin := cur.Role == domain.RoleAdmin && cur.IsActive && (next.Role != domain.RoleAdmin || !next.IsActive)
		if losesAdmin && adminCount <= 1 {
			return domain.ErrLastAdmin
		}

		before, after := diffView(cur, &next)
		if len(after) == 0 {
			result = cur // tidak ada perubahan: tidak ada tulisan & audit
			return nil
		}

		if err := u.users.Update(ctx, &next); err != nil {
			return err
		}
		if err := u.audit.Insert(ctx, domain.AuditEntry{
			Action: domain.AuditUserUpdate, EntityType: "user", EntityID: &id, Before: before, After: after,
		}); err != nil {
			return err
		}

		switch {
		case cur.IsActive && !next.IsActive:
			revoked, err := u.tokens.RevokeAllForUser(ctx, id, now)
			if err != nil {
				return err
			}
			if err := u.audit.Insert(ctx, domain.AuditEntry{
				Action: domain.AuditUserDeactivate, EntityType: "user", EntityID: &id,
				After: map[string]any{"revoked_sessions": revoked},
			}); err != nil {
				return err
			}
		case !cur.IsActive && next.IsActive:
			if err := u.audit.Insert(ctx, domain.AuditEntry{
				Action: domain.AuditUserActivate, EntityType: "user", EntityID: &id,
			}); err != nil {
				return err
			}
		}
		result = &next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ---------------------------------------------------------------- Reset password

// ResetPassword menetapkan password sementara baru (acak, tampil sekali), mewajibkan
// penggantian saat login, dan mencabut semua sesi user. Akun sendiri harus memakai
// change-password (reset sendiri akan memutus sesi admin dan menampilkan password di layar).
func (u *UserUsecase) ResetPassword(ctx context.Context, actor domain.Principal, id int64) (string, error) {
	if id == actor.UserID {
		return "", invalidUser([]domain.FieldError{{Field: "id", Message: "gunakan fitur ganti password untuk akun Anda sendiri"}})
	}
	temp, err := password.Generate(tempPasswordLen)
	if err != nil {
		return "", err
	}
	hash, err := password.Hash(temp)
	if err != nil {
		return "", err
	}
	now := u.clock.Now()

	err = u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if _, err := u.users.FindByIDForUpdate(ctx, id); err != nil {
			return err
		}
		if err := u.users.SetTemporaryPassword(ctx, id, hash); err != nil {
			return err
		}
		revoked, err := u.tokens.RevokeAllForUser(ctx, id, now)
		if err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action: domain.AuditUserPasswordReset, EntityType: "user", EntityID: &id,
			After: map[string]any{"revoked_sessions": revoked}, // password/hash TIDAK pernah masuk audit
		})
	})
	if err != nil {
		return "", err
	}
	return temp, nil
}

// ---------------------------------------------------------------- validation helpers

func invalidUser(errs []domain.FieldError) error {
	return &domain.ValidationError{Message: "Data user tidak valid", Errors: errs}
}

func validateName(name string) []domain.FieldError {
	if n := utf8.RuneCountInString(name); n < 1 || n > maxNameLen {
		return []domain.FieldError{{Field: "name", Message: "wajib diisi, maksimal 100 karakter"}}
	}
	return nil
}

func validateEmail(email string) []domain.FieldError {
	bad := domain.FieldError{Field: "email", Message: "format email tidak valid (maksimal 120 karakter)"}
	if email == "" || len(email) > maxEmailLen {
		return []domain.FieldError{bad}
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !strings.Contains(email, "@") {
		return []domain.FieldError{bad}
	}
	return nil
}

func validatePhone(phone *string) []domain.FieldError {
	if phone != nil && utf8.RuneCountInString(*phone) > maxPhoneLen {
		return []domain.FieldError{{Field: "phone", Message: "maksimal 30 karakter"}}
	}
	return nil
}

// cleanPhone: trim; kosong → nil (NULL di DB).
func cleanPhone(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}

func isWarehouseRole(r domain.Role) bool {
	return r == domain.RoleStaffWarehouse || r == domain.RoleHeadWarehouse
}

// validateRoleWarehouse: role harus dikenal; warehouse_id (scope gudang) hanya untuk role gudang
// (users.warehouse_id: "scope gudang utk role warehouse"). NULL = semua gudang.
func validateRoleWarehouse(role domain.Role, warehouseID *int64) []domain.FieldError {
	if !role.Valid() {
		return []domain.FieldError{{Field: "role", Message: "tidak valid"}}
	}
	if warehouseID != nil && !isWarehouseRole(role) {
		return []domain.FieldError{{Field: "warehouse_id", Message: "hanya untuk role STAFF_WAREHOUSE atau HEAD_WAREHOUSE; kosongkan untuk role ini"}}
	}
	return nil
}

func validateUpdateShape(in UpdateUserInput) []domain.FieldError {
	var errs []domain.FieldError
	if in.Role != nil && !in.Role.Valid() {
		errs = append(errs, domain.FieldError{Field: "role", Message: "tidak valid"})
	}
	return errs
}

// checkWarehouse: (kesalahan input, kegagalan infrastruktur). Error DB dikembalikan apa adanya
// (→ 500), tidak disamarkan sebagai kesalahan validasi.
func (u *UserUsecase) checkWarehouse(ctx context.Context, id *int64) ([]domain.FieldError, error) {
	if id == nil {
		return nil, nil
	}
	active, err := u.warehouses.IsActive(ctx, *id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return []domain.FieldError{{Field: "warehouse_id", Message: "gudang tidak ditemukan"}}, nil
	case err != nil:
		return nil, err
	case !active:
		return []domain.FieldError{{Field: "warehouse_id", Message: "gudang tidak aktif"}}, nil
	}
	return nil, nil
}

// auditView: representasi user untuk audit_logs — tidak pernah memuat password_hash.
func auditView(u *domain.User) map[string]any {
	return map[string]any{
		"name": u.Name, "email": u.Email, "phone": u.Phone, "role": u.Role,
		"warehouse_id": u.WarehouseID, "is_active": u.IsActive,
	}
}

// diffView mengembalikan hanya field yang berubah (sebelum/sesudah).
func diffView(a, b *domain.User) (before, after map[string]any) {
	before, after = map[string]any{}, map[string]any{}
	av, bv := auditView(a), auditView(b)
	for k, x := range av {
		if !equalAny(x, bv[k]) {
			before[k], after[k] = x, bv[k]
		}
	}
	return before, after
}

func equalAny(x, y any) bool {
	switch xv := x.(type) {
	case *string:
		yv := y.(*string)
		return (xv == nil && yv == nil) || (xv != nil && yv != nil && *xv == *yv)
	case *int64:
		yv := y.(*int64)
		return (xv == nil && yv == nil) || (xv != nil && yv != nil && *xv == *yv)
	default:
		return x == y
	}
}
