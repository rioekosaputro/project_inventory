package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type IdempotencyRepo struct{ pool *pgxpool.Pool }

func NewIdempotencyRepo(pool *pgxpool.Pool) *IdempotencyRepo { return &IdempotencyRepo{pool: pool} }

var _ domain.IdempotencyRepository = (*IdempotencyRepo)(nil)

func (r *IdempotencyRepo) Begin(ctx context.Context, key string, userID int64, endpoint string, ttl time.Duration) (bool, *domain.IdempotencyRecord, error) {
	db := database.GetExecutor(ctx, r.pool)
	// Catatan kedaluwarsa dibuang agar kunci lama dapat dipakai lagi.
	if _, err := db.Exec(ctx, `DELETE FROM idempotency_keys WHERE idem_key = $1 AND user_id = $2 AND created_at < $3`,
		key, userID, time.Now().Add(-ttl)); err != nil {
		return false, nil, err
	}
	tag, err := db.Exec(ctx, `
		INSERT INTO idempotency_keys (idem_key, user_id, endpoint) VALUES ($1, $2, $3)
		ON CONFLICT (idem_key, user_id) DO NOTHING`, key, userID, endpoint)
	if err != nil {
		return false, nil, err
	}
	if tag.RowsAffected() == 1 {
		return true, nil, nil
	}

	var rec domain.IdempotencyRecord
	var body []byte
	if err := db.QueryRow(ctx, `
		SELECT endpoint, status_code, response_body, created_at FROM idempotency_keys
		WHERE idem_key = $1 AND user_id = $2`, key, userID).Scan(&rec.Endpoint, &rec.StatusCode, &body, &rec.CreatedAt); err != nil {
		return false, nil, err // catatan dibatalkan bersamaan (Abort): klien cukup mengulang
	}
	rec.Body = body
	return false, &rec, nil
}

func (r *IdempotencyRepo) Complete(ctx context.Context, key string, userID int64, status int, body []byte) error {
	if !json.Valid(body) {
		b, _ := json.Marshal(string(body))
		body = b
	}
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE idempotency_keys SET status_code = $3, response_body = $4::jsonb
		WHERE idem_key = $1 AND user_id = $2`, key, userID, status, string(body))
	return err
}

func (r *IdempotencyRepo) Abort(ctx context.Context, key string, userID int64) error {
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx,
		`DELETE FROM idempotency_keys WHERE idem_key = $1 AND user_id = $2`, key, userID)
	return err
}
