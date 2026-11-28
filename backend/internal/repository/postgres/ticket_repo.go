package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type TicketRepo struct{ pool *pgxpool.Pool }

func NewTicketRepo(pool *pgxpool.Pool) *TicketRepo { return &TicketRepo{pool: pool} }

var _ domain.TicketRepository = (*TicketRepo)(nil)

const ticketSelectCols = `
	t.id, t.ticket_number,
	t.customer_id, c.company_name,
	t.customer_site_id, cs.site_name, cs.address_line, cs.city,
	t.product_serial_id, ps.asset_tag, ps.serial_number, p.name,
	t.reported_by_name, t.reported_via, t.category, t.priority, t.status,
	t.title, t.description,
	t.assigned_to, ua.name,
	t.sla_due_at, t.resolution_notes, t.resolved_at, t.closed_at,
	t.swap_so_id, so.so_number,
	t.created_by, uc.name, t.created_at, t.updated_at
`

const ticketJoins = `
	FROM maintenance_tickets t
	JOIN customers c ON c.id = t.customer_id
	JOIN customer_sites cs ON cs.id = t.customer_site_id
	JOIN product_serials ps ON ps.id = t.product_serial_id
	JOIN products p ON p.id = ps.product_id
	JOIN users uc ON uc.id = t.created_by
	LEFT JOIN users ua ON ua.id = t.assigned_to
	LEFT JOIN sales_orders so ON so.id = t.swap_so_id
`

func scanTicket(row interface{ Scan(dest ...any) error }) (domain.MaintenanceTicket, error) {
	var t domain.MaintenanceTicket
	var via, cat, prio, st string
	err := row.Scan(
		&t.ID, &t.TicketNumber,
		&t.CustomerID, &t.CustomerName,
		&t.CustomerSiteID, &t.SiteName, &t.SiteAddress, &t.SiteCity,
		&t.ProductSerialID, &t.AssetTag, &t.SerialNumber, &t.ProductName,
		&t.ReportedByName, &via, &cat, &prio, &st,
		&t.Title, &t.Description,
		&t.AssignedTo, &t.AssignedToName,
		&t.SLADueAt, &t.ResolutionNotes, &t.ResolvedAt, &t.ClosedAt,
		&t.SwapSOID, &t.SwapSONumber,
		&t.CreatedBy, &t.CreatedByName, &t.CreatedAt, &t.UpdatedAt,
	)
	t.ReportedVia = domain.TicketReportedVia(via)
	t.Category = domain.TicketCategory(cat)
	t.Priority = domain.TicketPriority(prio)
	t.Status = domain.TicketStatus(st)
	return t, err
}

func (r *TicketRepo) List(ctx context.Context, f domain.TicketFilter, p domain.PageParams) ([]domain.MaintenanceTicket, int64, error) {
	var conds []string
	var args []any
	addCond := func(sql string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(sql, len(args)))
	}
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		n := len(args)
		conds = append(conds, fmt.Sprintf(
			`(t.ticket_number ILIKE $%d OR t.title ILIKE $%d OR c.company_name ILIKE $%d OR ps.asset_tag ILIKE $%d OR ps.serial_number ILIKE $%d)`,
			n, n, n, n, n))
	}
	if f.Status != nil {
		addCond(`t.status = $%d`, string(*f.Status))
	}
	if f.Priority != nil {
		addCond(`t.priority = $%d`, string(*f.Priority))
	}
	if f.AssignedTo != nil {
		addCond(`t.assigned_to = $%d`, *f.AssignedTo)
	}
	if f.CustomerID != nil {
		addCond(`t.customer_id = $%d`, *f.CustomerID)
	}
	whereSQL := ""
	if len(conds) > 0 {
		whereSQL = " WHERE " + strings.Join(conds, " AND ")
	}

	exec := database.GetExecutor(ctx, r.pool)
	var total int64
	if err := exec.QueryRow(ctx, `SELECT COUNT(*) `+ticketJoins+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := p.OrderBy
	if orderBy == "" {
		orderBy = "t.created_at DESC, t.id DESC"
	}
	listArgs := append(append([]any{}, args...), p.Limit, p.Offset)
	q := fmt.Sprintf(`SELECT %s %s %s ORDER BY %s LIMIT $%d OFFSET $%d`,
		ticketSelectCols, ticketJoins, whereSQL, orderBy, len(args)+1, len(args)+2)

	rows, err := exec.Query(ctx, q, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []domain.MaintenanceTicket
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

func (r *TicketRepo) FindByID(ctx context.Context, id int64) (*domain.MaintenanceTicket, error) {
	t, err := scanTicket(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+ticketSelectCols+ticketJoins+` WHERE t.id = $1`, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

func (r *TicketRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.MaintenanceTicket, error) {
	database.MustInTx(ctx)
	// Kunci baris maintenance_tickets dulu, lalu baca dengan join.
	var dummy int64
	if err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT id FROM maintenance_tickets WHERE id = $1 FOR UPDATE`, id).Scan(&dummy); err != nil {
		return nil, notFound(err)
	}
	return r.FindByID(ctx, id)
}

func (r *TicketRepo) Create(ctx context.Context, t *domain.MaintenanceTicket) (*domain.MaintenanceTicket, error) {
	database.MustInTx(ctx)
	var id int64
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO maintenance_tickets (
			ticket_number, customer_id, customer_site_id, product_serial_id,
			reported_by_name, reported_via, category, priority, status,
			title, description, assigned_to, sla_due_at, created_by
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id`,
		t.TicketNumber, t.CustomerID, t.CustomerSiteID, t.ProductSerialID,
		t.ReportedByName, string(t.ReportedVia), string(t.Category), string(t.Priority), string(t.Status),
		t.Title, t.Description, t.AssignedTo, t.SLADueAt, t.CreatedBy,
	).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, id)
}

func (r *TicketRepo) Update(ctx context.Context, t *domain.MaintenanceTicket) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE maintenance_tickets
		SET title = $2, description = $3, category = $4, priority = $5,
		    reported_by_name = $6, reported_via = $7, sla_due_at = $8
		WHERE id = $1`,
		t.ID, t.Title, t.Description, string(t.Category), string(t.Priority),
		t.ReportedByName, string(t.ReportedVia), t.SLADueAt,
	))
}

func (r *TicketRepo) Assign(ctx context.Context, id int64, technicianID int64, newStatus domain.TicketStatus) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE maintenance_tickets SET assigned_to = $2, status = $3 WHERE id = $1`,
		id, technicianID, string(newStatus)))
}

func (r *TicketRepo) UpdateStatus(ctx context.Context, id int64, status domain.TicketStatus, resolutionNotes *string, resolvedAt, closedAt *time.Time) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE maintenance_tickets
		SET status = $2,
		    resolution_notes = COALESCE($3, resolution_notes),
		    resolved_at = COALESCE($4, resolved_at),
		    closed_at = COALESCE($5, closed_at)
		WHERE id = $1`,
		id, string(status), resolutionNotes, resolvedAt, closedAt))
}

func (r *TicketRepo) ListActivities(ctx context.Context, ticketID int64) ([]domain.TicketActivity, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT a.id, a.ticket_id, a.user_id, u.name, a.activity_type,
		       a.message, a.from_status, a.to_status,
		       a.latitude::float8, a.longitude::float8, a.photo_path, a.created_at
		FROM ticket_activities a
		JOIN users u ON u.id = a.user_id
		WHERE a.ticket_id = $1
		ORDER BY a.created_at ASC, a.id ASC`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.TicketActivity
	for rows.Next() {
		var a domain.TicketActivity
		var at string
		if err := rows.Scan(
			&a.ID, &a.TicketID, &a.UserID, &a.UserName, &at,
			&a.Message, &a.FromStatus, &a.ToStatus,
			&a.Latitude, &a.Longitude, &a.PhotoPath, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		a.ActivityType = domain.TicketActivityType(at)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *TicketRepo) InsertActivity(ctx context.Context, a *domain.TicketActivity) (*domain.TicketActivity, error) {
	database.MustInTx(ctx)
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO ticket_activities (
			ticket_id, user_id, activity_type, message, from_status, to_status,
			latitude, longitude, photo_path
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, created_at`,
		a.TicketID, a.UserID, string(a.ActivityType), a.Message, a.FromStatus, a.ToStatus,
		a.Latitude, a.Longitude, a.PhotoPath,
	).Scan(&a.ID, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *TicketRepo) ListSpareparts(ctx context.Context, ticketID int64) ([]domain.TicketSparepart, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT ts.id, ts.ticket_id, ts.product_id, p.sku, p.name, ts.qty, ts.status,
		       ts.lpb_id, l.lpb_number, ts.is_billable,
		       ts.requested_by, u.name, ts.requested_at, ts.notes
		FROM ticket_spareparts ts
		JOIN products p ON p.id = ts.product_id
		JOIN users u ON u.id = ts.requested_by
		LEFT JOIN lpb_documents l ON l.id = ts.lpb_id
		WHERE ts.ticket_id = $1
		ORDER BY ts.requested_at ASC, ts.id ASC`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.TicketSparepart
	for rows.Next() {
		var sp domain.TicketSparepart
		if err := rows.Scan(
			&sp.ID, &sp.TicketID, &sp.ProductID, &sp.SKU, &sp.ProductName, &sp.Qty, &sp.Status,
			&sp.LPBID, &sp.LPBNumber, &sp.IsBillable,
			&sp.RequestedBy, &sp.RequestedByName, &sp.RequestedAt, &sp.Notes,
		); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (r *TicketRepo) InsertSparepart(ctx context.Context, sp *domain.TicketSparepart) (*domain.TicketSparepart, error) {
	database.MustInTx(ctx)
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO ticket_spareparts (ticket_id, product_id, qty, status, is_billable, requested_by, notes)
		VALUES ($1, $2, $3, 'REQUESTED', $4, $5, $6)
		RETURNING id, status, requested_at`,
		sp.TicketID, sp.ProductID, sp.Qty, sp.IsBillable, sp.RequestedBy, sp.Notes,
	).Scan(&sp.ID, &sp.Status, &sp.RequestedAt)
	if err != nil {
		return nil, err
	}
	return sp, nil
}

func (r *TicketRepo) FindSparepartByIDForUpdate(ctx context.Context, id int64) (*domain.TicketSparepart, error) {
	database.MustInTx(ctx)
	var sp domain.TicketSparepart
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT ts.id, ts.ticket_id, ts.product_id, p.sku, p.name, ts.qty, ts.status,
		       ts.lpb_id, l.lpb_number, ts.is_billable,
		       ts.requested_by, u.name, ts.requested_at, ts.notes
		FROM ticket_spareparts ts
		JOIN products p ON p.id = ts.product_id
		JOIN users u ON u.id = ts.requested_by
		LEFT JOIN lpb_documents l ON l.id = ts.lpb_id
		WHERE ts.id = $1
		FOR UPDATE OF ts`, id).Scan(
		&sp.ID, &sp.TicketID, &sp.ProductID, &sp.SKU, &sp.ProductName, &sp.Qty, &sp.Status,
		&sp.LPBID, &sp.LPBNumber, &sp.IsBillable,
		&sp.RequestedBy, &sp.RequestedByName, &sp.RequestedAt, &sp.Notes,
	)
	if err != nil {
		return nil, notFound(err)
	}
	return &sp, nil
}

func (r *TicketRepo) MarkSparepartLPBCreated(ctx context.Context, sparepartID, lpbID int64) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE ticket_spareparts
		SET status = 'LPB_CREATED', lpb_id = $2
		WHERE id = $1 AND status = 'REQUESTED'`, sparepartID, lpbID))
}

func (r *TicketRepo) MarkSparepartIssuedByLPB(ctx context.Context, lpbID int64) ([]domain.TicketSparepart, error) {
	database.MustInTx(ctx)
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		UPDATE ticket_spareparts ts
		SET status = 'ISSUED'
		FROM products p
		WHERE ts.lpb_id = $1 AND ts.status = 'LPB_CREATED' AND p.id = ts.product_id
		RETURNING ts.id, ts.ticket_id, ts.product_id, p.sku, p.name, ts.qty, ts.status,
		          ts.lpb_id, ts.is_billable, ts.requested_by, ts.requested_at, ts.notes`, lpbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.TicketSparepart
	for rows.Next() {
		var sp domain.TicketSparepart
		if err := rows.Scan(
			&sp.ID, &sp.TicketID, &sp.ProductID, &sp.SKU, &sp.ProductName, &sp.Qty, &sp.Status,
			&sp.LPBID, &sp.IsBillable, &sp.RequestedBy, &sp.RequestedAt, &sp.Notes,
		); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (r *TicketRepo) SetSwapSO(ctx context.Context, ticketID, soID int64) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE maintenance_tickets SET swap_so_id = $2 WHERE id = $1 AND swap_so_id IS NULL`,
		ticketID, soID))
}
