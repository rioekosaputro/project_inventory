package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
	"rental-inventory-api/pkg/reqctx"
)

// AuditRepo menulis audit_logs (append-only di level DB). IP, User-Agent, dan request id
// diambil dari context sehingga pemanggil cukup mengisi siapa/apa.
type AuditRepo struct{ pool *pgxpool.Pool }

func NewAuditRepo(pool *pgxpool.Pool) *AuditRepo { return &AuditRepo{pool: pool} }

var _ domain.AuditRepository = (*AuditRepo)(nil)

func (r *AuditRepo) Insert(ctx context.Context, e domain.AuditEntry) error {
	before, err := toJSON(e.Before)
	if err != nil {
		return fmt.Errorf("audit: before_data: %w", err)
	}
	after, err := toJSON(e.After)
	if err != nil {
		return fmt.Errorf("audit: after_data: %w", err)
	}
	client := reqctx.ClientFrom(ctx)

	_, err = database.GetExecutor(ctx, r.pool).Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, entity_type, entity_id, before_data, after_data,
		                        ip_address, user_agent, request_id)
		VALUES ($1, $2, NULLIF($3::text, ''), $4, $5::jsonb, $6::jsonb, $7::inet, NULLIF($8::text, ''), NULLIF($9::text, ''))`,
		e.UserID, e.Action, e.EntityType, e.EntityID, before, after,
		ipOrNil(client.IP), client.UserAgent, reqctx.RequestID(ctx))
	return err
}

// toJSON: nil → NULL (pointer nil), selain itu JSON teks.
func toJSON(v any) (*string, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}
