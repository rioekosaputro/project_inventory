package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type UserRepo struct{ pool *pgxpool.Pool }

func NewUserRepo(pool *pgxpool.Pool) *UserRepo { return &UserRepo{pool: pool} }

var _ domain.UserRepository = (*UserRepo)(nil)

const userColumns = `id, name, email, phone, password_hash, role, warehouse_id,
	is_active, must_change_password, last_login_at, created_at, updated_at`

func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.one(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email)
}

func (r *UserRepo) FindByID(ctx context.Context, id int64) (*domain.User, error) {
	return r.one(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id)
}

func (r *UserRepo) one(ctx context.Context, query string, arg any) (*domain.User, error) {
	var u domain.User
	var role string
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, query, arg).Scan(
		&u.ID, &u.Name, &u.Email, &u.Phone, &u.PasswordHash, &role, &u.WarehouseID,
		&u.IsActive, &u.MustChangePassword, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	u.Role = domain.Role(role)
	return &u, nil
}

func (r *UserRepo) UpdatePassword(ctx context.Context, userID int64, hash string) error {
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE users SET password_hash = $2, must_change_password = FALSE WHERE id = $1`, userID, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *UserRepo) TouchLastLogin(ctx context.Context, userID int64, at time.Time) error {
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE users SET last_login_at = $2 WHERE id = $1`, userID, at)
	return err
}
