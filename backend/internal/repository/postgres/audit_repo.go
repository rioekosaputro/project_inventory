package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
	"rental-inventory-api/pkg/reqctx"
)

// AuditRepo menulis audit_logs (append-only di level DB). IP, User-Agent, dan request id
// diambil dari context sehingga pemanggil cukup mengisi siapa/apa.
type AuditRepo struct{ pool *pgxpool.Pool }

func NewAuditRepo(pool *pgxpool.Pool) *AuditRepo { return &AuditRepo{pool: pool} }

var (
	_ domain.AuditRepository = (*AuditRepo)(nil)
	_ domain.AuditReader     = (*AuditRepo)(nil)
)

func (r *AuditRepo) Insert(ctx context.Context, e domain.AuditEntry) error {
	before, err := toJSON(e.Before)
	if err != nil {
		return fmt.Errorf("audit: before_data: %w", err)
	}
	after, err := toJSON(e.After)
	if err != nil {
		return fmt.Errorf("audit: after_data: %w", err)
	}
	actor := e.UserID
	if actor == nil { // pelaku dari context (pengguna terautentikasi) bila usecase tidak mengisinya
		if id, ok := reqctx.UserIDFrom(ctx); ok {
			actor = &id
		}
	}
	client := reqctx.ClientFrom(ctx)

	_, err = database.GetExecutor(ctx, r.pool).Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, entity_type, entity_id, before_data, after_data,
		                        ip_address, user_agent, request_id)
		VALUES ($1, $2, NULLIF($3::text, ''), $4, $5::jsonb, $6::jsonb, $7::inet, NULLIF($8::text, ''), NULLIF($9::text, ''))`,
		actor, e.Action, e.EntityType, e.EntityID, before, after,
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

// List membaca log audit terpaginasi dengan filter teks, aksi, entitas, pelaku, dan rentang tanggal (T4.3).
func (r *AuditRepo) List(ctx context.Context, f domain.AuditFilter, p domain.PageParams) ([]domain.AuditLogRow, int64, error) {
	db := database.GetExecutor(ctx, r.pool)

	var conds []string
	var args []any
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if q := strings.TrimSpace(f.Query); q != "" {
		pat := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
		ph := addArg(pat)
		conds = append(conds, fmt.Sprintf(
			`(a.action ILIKE %s ESCAPE '\' OR COALESCE(a.entity_type, '') ILIKE %s ESCAPE '\' OR COALESCE(u.name, '') ILIKE %s ESCAPE '\' OR COALESCE(u.email, '') ILIKE %s ESCAPE '\' OR COALESCE(a.request_id, '') ILIKE %s ESCAPE '\')`,
			ph, ph, ph, ph, ph,
		))
	}
	if act := strings.TrimSpace(f.Action); act != "" {
		conds = append(conds, fmt.Sprintf("a.action = %s", addArg(strings.ToUpper(act))))
	}
	if et := strings.TrimSpace(f.EntityType); et != "" {
		conds = append(conds, fmt.Sprintf("a.entity_type = %s", addArg(et)))
	}
	if f.UserID != nil {
		conds = append(conds, fmt.Sprintf("a.user_id = %s", addArg(*f.UserID)))
	}
	if f.DateFrom != nil {
		conds = append(conds, fmt.Sprintf("a.created_at >= %s", addArg(*f.DateFrom)))
	}
	if f.DateTo != nil {
		conds = append(conds, fmt.Sprintf("a.created_at < %s", addArg(*f.DateTo)))
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int64
	countSQL := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.user_id
		%s`, where)
	if err := db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit count: %w", err)
	}

	orderBy := "a.created_at DESC, a.id DESC"
	if p.OrderBy != "" {
		orderBy = p.OrderBy + ", a.id DESC"
	}
	limitPh := addArg(p.Limit)
	offsetPh := addArg(p.Offset)

	listSQL := fmt.Sprintf(`
		SELECT a.id, a.user_id, u.name, u.email, a.action, a.entity_type, a.entity_id,
		       a.before_data, a.after_data, HOST(a.ip_address), a.user_agent, a.request_id, a.created_at
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.user_id
		%s
		ORDER BY %s
		LIMIT %s OFFSET %s`, where, orderBy, limitPh, offsetPh)

	rows, err := db.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit list: %w", err)
	}
	defer rows.Close()

	out := make([]domain.AuditLogRow, 0)
	for rows.Next() {
		var row domain.AuditLogRow
		var rawBefore, rawAfter []byte
		if err := rows.Scan(
			&row.ID, &row.UserID, &row.UserName, &row.UserEmail, &row.Action,
			&row.EntityType, &row.EntityID, &rawBefore, &rawAfter,
			&row.IPAddress, &row.UserAgent, &row.RequestID, &row.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("audit scan: %w", err)
		}
		if len(rawBefore) > 0 {
			var v any
			if err := json.Unmarshal(rawBefore, &v); err == nil {
				row.BeforeData = v
			}
		}
		if len(rawAfter) > 0 {
			var v any
			if err := json.Unmarshal(rawAfter, &v); err == nil {
				row.AfterData = v
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("audit rows: %w", err)
	}
	return out, total, nil
}
