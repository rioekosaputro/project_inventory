//go:build integration

package docnum_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/testutil/pgtest"
	"rental-inventory-api/pkg/database"
	"rental-inventory-api/pkg/docnum"
)

const migrationsDir = "../../migrations"

func setup(t *testing.T) (*pgxpool.Pool, *database.TxManager, *docnum.Generator) {
	t.Helper()
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, migrationsDir, "up")
	return pool, database.NewTxManager(pool), docnum.New(pool)
}

func lastNumber(t *testing.T, pool *pgxpool.Pool, docType, scope, period string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT last_number FROM document_sequences WHERE doc_type=$1 AND scope_key=$2 AND period=$3`,
		docType, scope, period).Scan(&n)
	require.NoError(t, err)
	return n
}

// DoD T1.1: 50 goroutine paralel (masing-masing transaksi sendiri) → nomor unik, berurutan, tanpa celah.
func TestNext_Parallel50_UniqueAndGapFree(t *testing.T) {
	pool, txm, gen := setup(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

	const workers = 50
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		numbers = make([]string, 0, workers)
		errs    []error
	)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // lepas bersamaan agar benar-benar berebut
			var got string
			err := txm.WithinTransaction(ctx, func(ctx context.Context) error {
				var err error
				got, err = gen.Next(ctx, "LPBI", "WH-SMG-01", at)
				return err
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			numbers = append(numbers, got)
		}()
	}
	close(start)
	wg.Wait()

	require.Empty(t, errs)
	require.Len(t, numbers, workers)

	sort.Strings(numbers)
	for i, got := range numbers {
		assert.Equal(t, docnum.Format("LPBI", "WH-SMG-01", "202611", i+1), got,
			"nomor ke-%d harus berurutan tanpa celah", i+1)
	}
	assert.Equal(t, workers, lastNumber(t, pool, "LPBI", "WH-SMG-01", "202611"))
}

// Transaksi yang gagal tidak boleh menghabiskan nomor (tidak ada celah).
func TestNext_RollbackDoesNotConsumeNumber(t *testing.T) {
	pool, txm, gen := setup(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

	next := func() (string, error) {
		var got string
		err := txm.WithinTransaction(ctx, func(ctx context.Context) error {
			var err error
			got, err = gen.Next(ctx, "SO", docnum.ScopeGlobal, at)
			return err
		})
		return got, err
	}

	first, err := next()
	require.NoError(t, err)
	assert.Equal(t, "SO/GLOBAL/2026/11/0001", first)

	boom := errors.New("gagal setelah nomor terbit")
	err = txm.WithinTransaction(ctx, func(ctx context.Context) error {
		got, err := gen.Next(ctx, "SO", docnum.ScopeGlobal, at)
		require.NoError(t, err)
		assert.Equal(t, "SO/GLOBAL/2026/11/0002", got, "di dalam tx nomor 2 sudah terbit")
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.Equal(t, 1, lastNumber(t, pool, "SO", docnum.ScopeGlobal, "202611"), "rollback mengembalikan counter")

	again, err := next()
	require.NoError(t, err)
	assert.Equal(t, "SO/GLOBAL/2026/11/0002", again, "nomor 2 dipakai ulang, bukan terlewat")
}

// Panik di dalam transaksi juga me-rollback counter.
func TestNext_PanicRollsBackCounter(t *testing.T) {
	pool, txm, gen := setup(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

	assert.Panics(t, func() {
		_ = txm.WithinTransaction(ctx, func(ctx context.Context) error {
			_, _ = gen.Next(ctx, "DO", docnum.ScopeGlobal, at)
			panic("boom")
		})
	})
	var rows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM document_sequences`).Scan(&rows))
	assert.Zero(t, rows)
}

func TestNext_IndependentSequencesPerTypeScopeAndPeriod(t *testing.T) {
	_, txm, gen := setup(t)
	ctx := context.Background()
	nov := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	// 30 Nov 17:30 UTC = 1 Des 00:30 WIB → bulan baru menurut zona bisnis.
	desWIB := time.Date(2026, 11, 30, 17, 30, 0, 0, time.UTC)

	var got []string
	err := txm.WithinTransaction(ctx, func(ctx context.Context) error {
		for _, c := range []struct {
			docType, scope string
			at             time.Time
		}{
			{"LPBI", "WH-A", nov},
			{"LPBI", "WH-A", nov},
			{"LPBI", "WH-B", nov},    // scope lain mulai dari 1
			{"LPBO", "WH-A", nov},    // jenis lain mulai dari 1
			{"LPBI", "WH-A", desWIB}, // periode (WIB) berikutnya reset ke 1
			{"LPBI", "WH-A", nov},    // kembali ke periode lama: lanjut ke 3
		} {
			n, err := gen.Next(ctx, c.docType, c.scope, c.at)
			if err != nil {
				return err
			}
			got = append(got, n)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"LPBI/WH-A/2026/11/0001",
		"LPBI/WH-A/2026/11/0002",
		"LPBI/WH-B/2026/11/0001",
		"LPBO/WH-A/2026/11/0001",
		"LPBI/WH-A/2026/12/0001",
		"LPBI/WH-A/2026/11/0003",
	}, got)
}

// Urutan tanpa reset (asset tag): periode 000000 terus naik lintas bulan.
func TestNextNumber_PeriodNoneNeverResets(t *testing.T) {
	_, txm, gen := setup(t)
	ctx := context.Background()

	var got []int
	err := txm.WithinTransaction(ctx, func(ctx context.Context) error {
		for i := 0; i < 3; i++ {
			n, err := gen.NextNumber(ctx, "AST", docnum.ScopeGlobal, docnum.PeriodNone)
			if err != nil {
				return err
			}
			got = append(got, n)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, got)
}

// Memanggil di luar transaksi adalah kesalahan pemrograman: panik, bukan nomor yang bisa hilang.
func TestNext_OutsideTransactionPanics(t *testing.T) {
	_, _, gen := setup(t)
	assert.Panics(t, func() {
		_, _ = gen.Next(context.Background(), "LPBI", "GLOBAL", time.Now())
	})
}

// Asset tag: counter global tidak reset lintas bulan; hanya bagian YYMM yang mengikuti waktu.
func TestNextAssetTag_GlobalCounterAcrossMonths(t *testing.T) {
	_, txm, gen := setup(t)
	ctx := context.Background()

	var got []string
	err := txm.WithinTransaction(ctx, func(ctx context.Context) error {
		for _, at := range []time.Time{
			time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC),
			time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
			time.Date(2026, 12, 2, 10, 0, 0, 0, time.UTC), // bulan baru, counter lanjut
		} {
			tag, err := gen.NextAssetTag(ctx, at)
			if err != nil {
				return err
			}
			got = append(got, tag)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"AST-2611-00001", "AST-2611-00002", "AST-2612-00003"}, got)
}

// Asset tag yang diterbitkan paralel tetap unik dan berurutan (UNIQUE product_serials.asset_tag).
func TestNextAssetTag_Parallel50_Unique(t *testing.T) {
	_, txm, gen := setup(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		tags []string
		errs []error
	)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var tag string
			err := txm.WithinTransaction(ctx, func(ctx context.Context) error {
				var err error
				tag, err = gen.NextAssetTag(ctx, at)
				return err
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			tags = append(tags, tag)
		}()
	}
	wg.Wait()

	require.Empty(t, errs)
	sort.Strings(tags)
	require.Len(t, tags, 50)
	for i, tag := range tags {
		assert.Equal(t, docnum.FormatAssetTag(at, i+1), tag)
	}
}
