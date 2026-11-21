package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type LoginAttemptRepo struct{ pool *pgxpool.Pool }

func NewLoginAttemptRepo(pool *pgxpool.Pool) *LoginAttemptRepo { return &LoginAttemptRepo{pool: pool} }

var _ domain.LoginAttemptRepository = (*LoginAttemptRepo)(nil)

func (r *LoginAttemptRepo) Record(ctx context.Context, email, ip string, at time.Time) error {
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx,
		`INSERT INTO login_attempts (email, ip_address, created_at) VALUES ($1, $2::inet, $3)`,
		email, ipOrUnspecified(ip), at)
	return err
}

func (r *LoginAttemptRepo) CountByEmailIP(ctx context.Context, email, ip string, since time.Time) (int, error) {
	var n int
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT count(*) FROM login_attempts WHERE email = $1 AND ip_address = $2::inet AND created_at > $3`,
		email, ipOrUnspecified(ip), since).Scan(&n)
	return n, err
}

func (r *LoginAttemptRepo) CountByIP(ctx context.Context, ip string, since time.Time) (int, error) {
	var n int
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT count(*) FROM login_attempts WHERE ip_address = $1::inet AND created_at > $2`,
		ipOrUnspecified(ip), since).Scan(&n)
	return n, err
}

func (r *LoginAttemptRepo) OldestByEmailIP(ctx context.Context, email, ip string, since time.Time) (time.Time, error) {
	var t *time.Time
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT min(created_at) FROM login_attempts WHERE email = $1 AND ip_address = $2::inet AND created_at > $3`,
		email, ipOrUnspecified(ip), since).Scan(&t)
	return derefTime(t), err
}

func (r *LoginAttemptRepo) OldestByIP(ctx context.Context, ip string, since time.Time) (time.Time, error) {
	var t *time.Time
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT min(created_at) FROM login_attempts WHERE ip_address = $1::inet AND created_at > $2`,
		ipOrUnspecified(ip), since).Scan(&t)
	return derefTime(t), err
}

func (r *LoginAttemptRepo) Clear(ctx context.Context, email, ip string) error {
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx,
		`DELETE FROM login_attempts WHERE email = $1 AND ip_address = $2::inet`, email, ipOrUnspecified(ip))
	return err
}

func (r *LoginAttemptRepo) DeleteBefore(ctx context.Context, before time.Time) error {
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `DELETE FROM login_attempts WHERE created_at < $1`, before)
	return err
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
