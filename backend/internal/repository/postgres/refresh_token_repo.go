package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type RefreshTokenRepo struct{ pool *pgxpool.Pool }

func NewRefreshTokenRepo(pool *pgxpool.Pool) *RefreshTokenRepo { return &RefreshTokenRepo{pool: pool} }

var _ domain.RefreshTokenRepository = (*RefreshTokenRepo)(nil)

// Create menyimpan token. FamilyID kosong → keluarga baru (UUID dibuat database).
func (r *RefreshTokenRepo) Create(ctx context.Context, t *domain.RefreshToken, userAgent, ip string) (*domain.RefreshToken, error) {
	out := *t
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at, user_agent, ip_address, created_at)
		VALUES ($1, $2, COALESCE(NULLIF($3::text, '')::uuid, gen_random_uuid()), $4, NULLIF($5::text, ''), $6::inet, $7)
		RETURNING id, family_id::text`,
		t.UserID, t.TokenHash, t.FamilyID, t.ExpiresAt, userAgent, ipOrNil(ip), t.CreatedAt,
	).Scan(&out.ID, &out.FamilyID)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// LockByHash: SELECT ... FOR UPDATE. Wajib di dalam transaksi supaya dua refresh
// bersamaan dengan token yang sama diserialkan (yang kedua melihat token sudah dirotasi).
func (r *RefreshTokenRepo) LockByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	database.MustInTx(ctx)
	var t domain.RefreshToken
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT id, user_id, token_hash, family_id::text, expires_at, created_at, revoked_at, replaced_by
		FROM refresh_tokens WHERE token_hash = $1 FOR UPDATE`, hash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.FamilyID, &t.ExpiresAt, &t.CreatedAt, &t.RevokedAt, &t.ReplacedBy)
	if err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

func (r *RefreshTokenRepo) MarkRotated(ctx context.Context, id, newID int64, at time.Time) error {
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $3, replaced_by = $2 WHERE id = $1 AND revoked_at IS NULL`,
		id, newID, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *RefreshTokenRepo) FamilyStartedAt(ctx context.Context, familyID string) (time.Time, error) {
	var at time.Time
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT created_at FROM refresh_tokens WHERE family_id = $1::uuid ORDER BY id LIMIT 1`, familyID).Scan(&at)
	if err != nil {
		return time.Time{}, notFound(err)
	}
	return at, nil
}

func (r *RefreshTokenRepo) RevokeFamily(ctx context.Context, familyID string, at time.Time) (int64, error) {
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2 WHERE family_id = $1::uuid AND revoked_at IS NULL`, familyID, at)
	return tag.RowsAffected(), err
}

func (r *RefreshTokenRepo) RevokeAllForUser(ctx context.Context, userID int64, at time.Time) (int64, error) {
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`, userID, at)
	return tag.RowsAffected(), err
}
