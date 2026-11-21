package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

// filterBuilder merakit klausa WHERE parameterized. Nilai pengguna SELALU lewat argumen ($n),
// tidak pernah disambung ke string SQL.
type filterBuilder struct {
	conds []string
	args  []any
}

// add menambah kondisi; cond memakai %d (atau %[1]d bila dipakai berulang) untuk nomor argumen.
func (b *filterBuilder) add(cond string, v any) {
	b.args = append(b.args, v)
	b.conds = append(b.conds, fmt.Sprintf(cond, len(b.args)))
}

// like menambah pencarian ILIKE '%q%' pada beberapa kolom (wildcard pada q dinetralkan).
func (b *filterBuilder) like(q string, columns ...string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return
	}
	parts := make([]string, len(columns))
	for i, col := range columns {
		parts[i] = col + " ILIKE $%[1]d"
	}
	b.add("("+strings.Join(parts, " OR ")+")", "%"+escapeLike(q)+"%")
}

func (b *filterBuilder) where() string {
	if len(b.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(b.conds, " AND ")
}

// orderPattern: pertahanan berlapis — OrderBy seharusnya sudah dari whitelist (pkg/pagination),
// tetapi repository tetap menolak apa pun selain "kolom[.kolom] [ASC|DESC], ...".
var orderPattern = regexp.MustCompile(`^[a-z_]+(\.[a-z_]+)?( (ASC|DESC))?(, [a-z_]+(\.[a-z_]+)?( (ASC|DESC))?)*$`)

// queryPage menjalankan COUNT dan SELECT berhalaman. from berisi tabel + JOIN, columns daftar kolom
// SELECT, tiebreak kolom pengurut tambahan agar urutan stabil antar halaman.
func queryPage[T any](
	ctx context.Context, db database.DBTX,
	from, columns string, b *filterBuilder, p domain.PageParams, tiebreak string,
	scan func(pgx.Row) (T, error),
) ([]T, int64, error) {
	order := tiebreak
	if p.OrderBy != "" {
		if !orderPattern.MatchString(p.OrderBy) {
			return nil, 0, fmt.Errorf("postgres: OrderBy tidak valid: %q", p.OrderBy)
		}
		order = p.OrderBy + ", " + tiebreak
	}

	var total int64
	if err := db.QueryRow(ctx, `SELECT count(*) FROM `+from+b.where(), b.args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args := append(append([]any{}, b.args...), p.Limit, p.Offset)
	rows, err := db.Query(ctx, fmt.Sprintf(`SELECT %s FROM %s%s ORDER BY %s LIMIT $%d OFFSET $%d`,
		columns, from, b.where(), order, len(b.args)+1, len(b.args)+2), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]T, 0, p.Limit)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

// duplicateOf memetakan pelanggaran UNIQUE (23505) pada constraint yang dikenal ke *DuplicateError.
// Error lain dikembalikan apa adanya.
func duplicateOf(err error, byConstraint map[string]domain.DuplicateError) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if d, ok := byConstraint[pgErr.ConstraintName]; ok {
			return &d
		}
	}
	return err
}

func mustAffectOne(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// moneyArg: decimal → teks NUMERIC(15,2) untuk parameter ($n::numeric); nil → NULL.
func moneyArg(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.StringFixed(2)
	return &s
}

// moneyFrom: hasil kolom NUMERIC yang di-cast ::text → decimal; nil → nil.
func moneyFrom(s *string) (*decimal.Decimal, error) {
	if s == nil {
		return nil, nil
	}
	d, err := decimal.NewFromString(*s)
	if err != nil {
		return nil, fmt.Errorf("postgres: nilai uang tidak valid %q: %w", *s, err)
	}
	return &d, nil
}
