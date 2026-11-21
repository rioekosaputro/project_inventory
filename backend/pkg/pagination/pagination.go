// Package pagination mem-parsing ?page, ?limit, ?sort dengan aman (plan Bagian 7.4).
//
// Kolom sort WAJIB lewat whitelist (SortSpec.Allowed) agar tidak terbuka untuk
// SQL injection lewat ORDER BY: nilai peta adalah ekspresi SQL yang ditulis developer,
// kunci peta adalah nama yang boleh dipakai klien.
package pagination

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	DefaultLimit = 20
	MaxLimit     = 100
	maxPage      = 100000
	maxSortKeys  = 3
)

// Error adalah kesalahan parameter query; dipetakan ke 400 oleh httperr.
type Error struct {
	Field   string
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Message) }

// SortField satu kolom pengurutan yang sudah lolos whitelist.
type SortField struct {
	Column string // ekspresi SQL dari whitelist
	Desc   bool
}

// SortSpec mendefinisikan sort yang diizinkan untuk satu endpoint.
type SortSpec struct {
	Allowed map[string]string // nama klien -> ekspresi SQL
	Default string            // mis. "-created_at"; dipakai bila ?sort kosong
}

// Params hasil parsing.
type Params struct {
	Page  int
	Limit int
	Sort  []SortField
}

// Offset untuk SQL OFFSET.
func (p Params) Offset() int { return (p.Page - 1) * p.Limit }

// OrderBy menghasilkan isi klausa ORDER BY ("created_at DESC, id ASC"), kosong bila tidak ada sort.
// Aman dipakai langsung karena kolom hanya berasal dari whitelist.
func (p Params) OrderBy() string {
	parts := make([]string, 0, len(p.Sort))
	for _, s := range p.Sort {
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		parts = append(parts, s.Column+" "+dir)
	}
	return strings.Join(parts, ", ")
}

// FromQuery mem-parsing page, limit, dan sort.
//   - page default 1; limit default 20 dan dipotong ke maksimum 100.
//   - nilai bukan angka / < 1 → *Error.
//   - sort tidak ada di whitelist → *Error.
func FromQuery(q url.Values, spec SortSpec) (Params, error) {
	p := Params{Page: 1, Limit: DefaultLimit}

	if v := strings.TrimSpace(q.Get("page")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Params{}, &Error{"page", "harus berupa angka bulat minimal 1"}
		}
		if n > maxPage {
			return Params{}, &Error{"page", fmt.Sprintf("maksimal %d", maxPage)}
		}
		p.Page = n
	}

	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Params{}, &Error{"limit", "harus berupa angka bulat minimal 1"}
		}
		p.Limit = min(n, MaxLimit)
	}

	raw := strings.TrimSpace(q.Get("sort"))
	if raw == "" {
		raw = spec.Default
	}
	sorts, err := parseSort(raw, spec.Allowed)
	if err != nil {
		return Params{}, err
	}
	p.Sort = sorts
	return p, nil
}

func parseSort(raw string, allowed map[string]string) ([]SortField, error) {
	if raw == "" {
		return nil, nil
	}
	keys := strings.Split(raw, ",")
	if len(keys) > maxSortKeys {
		return nil, &Error{"sort", fmt.Sprintf("maksimal %d kolom", maxSortKeys)}
	}

	out := make([]SortField, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		desc := strings.HasPrefix(k, "-")
		name := strings.TrimPrefix(k, "-")
		col, ok := allowed[name]
		if !ok {
			return nil, &Error{"sort", fmt.Sprintf("kolom '%s' tidak dapat dipakai untuk pengurutan (pilihan: %s)", name, allowedNames(allowed))}
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, SortField{Column: col, Desc: desc})
	}
	return out, nil
}

func allowedNames(allowed map[string]string) string {
	names := make([]string, 0, len(allowed))
	for k := range allowed {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// Meta adalah blok "meta" pada respons daftar.
type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// NewMeta menghitung meta dari parameter dan total baris.
func NewMeta(p Params, total int64) Meta {
	pages := 0
	if total > 0 {
		pages = int((total + int64(p.Limit) - 1) / int64(p.Limit))
	}
	return Meta{Page: p.Page, Limit: p.Limit, Total: total, TotalPages: pages}
}
