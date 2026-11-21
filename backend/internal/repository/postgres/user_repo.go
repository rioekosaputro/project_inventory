package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type UserRepo struct{ pool *pgxpool.Pool }

func NewUserRepo(pool *pgxpool.Pool) *UserRepo { return &UserRepo{pool: pool} }

var _ domain.UserRepository = (*UserRepo)(nil)

const userColumns = `id, name, email, phone, password_hash, role, warehouse_id,
	is_active, must_change_password, last_login_at, created_at, updated_at`

func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	var role string
	if err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.PasswordHash, &role, &u.WarehouseID,
		&u.IsActive, &u.MustChangePassword, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	u.Role = domain.Role(role)
	return &u, nil
}

func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return scanUser(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE email = $1`, email))
}

func (r *UserRepo) FindByID(ctx context.Context, id int64) (*domain.User, error) {
	return scanUser(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

func (r *UserRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.User, error) {
	database.MustInTx(ctx)
	return scanUser(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1 FOR UPDATE`, id))
}

func (r *UserRepo) UpdatePassword(ctx context.Context, userID int64, hash string) error {
	return r.mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE users SET password_hash = $2, must_change_password = FALSE WHERE id = $1`, userID, hash))
}

func (r *UserRepo) SetTemporaryPassword(ctx context.Context, userID int64, hash string) error {
	return r.mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE users SET password_hash = $2, must_change_password = TRUE WHERE id = $1`, userID, hash))
}

func (r *UserRepo) TouchLastLogin(ctx context.Context, userID int64, at time.Time) error {
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE users SET last_login_at = $2 WHERE id = $1`, userID, at)
	return err
}

func (r *UserRepo) Create(ctx context.Context, u *domain.User) (*domain.User, error) {
	created, err := scanUser(database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO users (name, email, phone, password_hash, role, warehouse_id, is_active, must_change_password)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+userColumns,
		u.Name, u.Email, u.Phone, u.PasswordHash, string(u.Role), u.WarehouseID, u.IsActive, u.MustChangePassword))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
			return nil, domain.ErrEmailTaken
		}
		return nil, err
	}
	return created, nil
}

func (r *UserRepo) Update(ctx context.Context, u *domain.User) error {
	return r.mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE users SET name = $2, phone = $3, role = $4, warehouse_id = $5, is_active = $6
		WHERE id = $1`, u.ID, u.Name, u.Phone, string(u.Role), u.WarehouseID, u.IsActive))
}

func (r *UserRepo) LockActiveAdmins(ctx context.Context) (int, error) {
	database.MustInTx(ctx)
	var n int
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT id FROM users WHERE role = 'ADMIN' AND is_active ORDER BY id FOR UPDATE
		) admins`).Scan(&n)
	return n, err
}

// orderByPattern: pertahanan berlapis — OrderBy seharusnya sudah dari whitelist
// (pkg/pagination), tetapi repository tetap menolak apa pun selain "kolom [ASC|DESC], ...".
var orderByPattern = regexp.MustCompile(`^[a-z_]+( (ASC|DESC))?(, [a-z_]+( (ASC|DESC))?)*$`)

func (r *UserRepo) List(ctx context.Context, f domain.UserFilter, p domain.PageParams) ([]domain.User, int64, error) {
	var (
		conds []string
		args  []any
	)
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		add("(name ILIKE $%[1]d OR email ILIKE $%[1]d)", "%"+escapeLike(q)+"%")
	}
	if f.Role != nil {
		add("role = $%d", string(*f.Role))
	}
	if f.IsActive != nil {
		add("is_active = $%d", *f.IsActive)
	}
	if f.WarehouseID != nil {
		add("warehouse_id = $%d", *f.WarehouseID)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	order := "id"
	if p.OrderBy != "" {
		if !orderByPattern.MatchString(p.OrderBy) {
			return nil, 0, fmt.Errorf("postgres: OrderBy tidak valid: %q", p.OrderBy)
		}
		order = p.OrderBy + ", id"
	}

	db := database.GetExecutor(ctx, r.pool)
	var total int64
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), p.Limit, p.Offset)
	rows, err := db.Query(ctx, fmt.Sprintf(
		`SELECT %s FROM users%s ORDER BY %s LIMIT $%d OFFSET $%d`,
		userColumns, where, order, len(args)+1, len(args)+2), listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := make([]domain.User, 0, p.Limit)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, *u)
	}
	return users, total, rows.Err()
}

func (r *UserRepo) mustAffectOne(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// escapeLike menetralkan wildcard LIKE (\ % _) pada input pengguna.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
