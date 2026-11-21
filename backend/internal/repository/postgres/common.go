// Package postgres berisi implementasi repository (SQL saja, tanpa aturan bisnis).
// Setiap method memakai database.GetExecutor(ctx, pool) sehingga otomatis ikut transaksi
// usecase bila ada.
package postgres

import (
	"errors"
	"net/netip"

	"github.com/jackc/pgx/v5"

	"rental-inventory-api/internal/domain"
)

// notFound menerjemahkan pgx.ErrNoRows ke domain.ErrNotFound; error lain diteruskan apa adanya.
func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

// ipOrNil mengembalikan IP tervalidasi (tanpa zone) untuk kolom INET, atau nil bila tidak valid.
func ipOrNil(s string) *string {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return nil
	}
	v := addr.WithZone("").String()
	return &v
}

// ipOrUnspecified: untuk kolom INET NOT NULL; IP tak valid dipetakan ke 0.0.0.0
// agar percobaan tetap terhitung pada rate limit.
func ipOrUnspecified(s string) string {
	if p := ipOrNil(s); p != nil {
		return *p
	}
	return "0.0.0.0"
}
