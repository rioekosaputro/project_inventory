package pagination

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var spec = SortSpec{
	Allowed: map[string]string{"created_at": "l.created_at", "lpb_number": "l.lpb_number", "status": "l.status"},
	Default: "-created_at",
}

func q(s string) url.Values {
	v, _ := url.ParseQuery(s)
	return v
}

func TestFromQuery_Defaults(t *testing.T) {
	p, err := FromQuery(q(""), spec)
	require.NoError(t, err)

	assert.Equal(t, 1, p.Page)
	assert.Equal(t, DefaultLimit, p.Limit)
	assert.Equal(t, 0, p.Offset())
	assert.Equal(t, "l.created_at DESC", p.OrderBy())
}

func TestFromQuery_PageLimitOffset(t *testing.T) {
	p, err := FromQuery(q("page=3&limit=25"), spec)
	require.NoError(t, err)

	assert.Equal(t, 3, p.Page)
	assert.Equal(t, 25, p.Limit)
	assert.Equal(t, 50, p.Offset())
}

func TestFromQuery_LimitIsCappedAtMax(t *testing.T) {
	p, err := FromQuery(q("limit=5000"), spec)
	require.NoError(t, err)
	assert.Equal(t, MaxLimit, p.Limit)
}

func TestFromQuery_InvalidPageAndLimit(t *testing.T) {
	for _, raw := range []string{"page=0", "page=-1", "page=abc", "page=1.5", "page=100001", "limit=0", "limit=-5", "limit=abc"} {
		t.Run(raw, func(t *testing.T) {
			_, err := FromQuery(q(raw), spec)
			var pErr *Error
			require.ErrorAs(t, err, &pErr)
			assert.Contains(t, []string{"page", "limit"}, pErr.Field)
		})
	}
}

func TestFromQuery_SortMultipleColumnsAndDirection(t *testing.T) {
	p, err := FromQuery(q("sort=-created_at,lpb_number"), spec)
	require.NoError(t, err)

	assert.Equal(t, []SortField{{"l.created_at", true}, {"l.lpb_number", false}}, p.Sort)
	assert.Equal(t, "l.created_at DESC, l.lpb_number ASC", p.OrderBy())
}

func TestFromQuery_SortDuplicateIgnored(t *testing.T) {
	p, err := FromQuery(q("sort=status,-status"), spec)
	require.NoError(t, err)
	assert.Equal(t, "l.status ASC", p.OrderBy())
}

// Kolom sort berasal dari whitelist: input berbahaya harus ditolak, bukan disalin ke SQL.
func TestFromQuery_SortRejectsNonWhitelisted(t *testing.T) {
	for _, raw := range []string{
		"sort=password_hash",
		"sort=created_at;DROP TABLE users",
		"sort=created_at,(SELECT 1)",
		"sort=l.created_at",
		"sort=-",
		"sort=created_at,status,lpb_number,created_at",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := FromQuery(q(url.Values{"sort": {raw[5:]}}.Encode()), spec)
			var pErr *Error
			require.ErrorAs(t, err, &pErr)
			assert.Equal(t, "sort", pErr.Field)
		})
	}
}

func TestFromQuery_NoDefaultSortMeansNoOrderBy(t *testing.T) {
	p, err := FromQuery(q(""), SortSpec{Allowed: spec.Allowed})
	require.NoError(t, err)
	assert.Empty(t, p.OrderBy())
}

func TestNewMeta(t *testing.T) {
	tests := []struct {
		total int64
		limit int
		pages int
	}{{0, 20, 0}, {1, 20, 1}, {20, 20, 1}, {21, 20, 2}, {101, 10, 11}}
	for _, tt := range tests {
		m := NewMeta(Params{Page: 1, Limit: tt.limit}, tt.total)
		assert.Equal(t, tt.pages, m.TotalPages, "total=%d limit=%d", tt.total, tt.limit)
		assert.Equal(t, tt.total, m.Total)
	}
}
