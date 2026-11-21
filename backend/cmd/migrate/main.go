// Command migrate adalah wrapper golang-migrate.
//
// Pemakaian (dari folder backend/):
//
//	go run ./cmd/migrate up
//	go run ./cmd/migrate down [N|all]   # default 1 langkah
//	go run ./cmd/migrate goto VERSION
//	go run ./cmd/migrate version
//	go run ./cmd/migrate force VERSION  # hanya untuk memperbaiki state "dirty"
//	go run ./cmd/migrate new NAMA
//
// Opsi: -path DIR (default "migrations"). URL database: MIGRATE_DATABASE_URL,
// fallback DATABASE_URL. Di Supabase pakai session pooler (5432) / direct, bukan 6543.
package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"rental-inventory-api/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	path := fs.String("path", "migrations", "folder file migrasi")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New("perintah wajib diisi: up | down [N|all] | goto V | version | force V | new NAMA")
	}
	cmd, params := rest[0], rest[1:]

	// `new` tidak butuh koneksi database.
	if cmd == "new" {
		if len(params) != 1 {
			return errors.New("pemakaian: new NAMA")
		}
		return newMigration(*path, params[0])
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	dbURL := cfg.MigrateURL()
	if dbURL == "" {
		return errors.New("MIGRATE_DATABASE_URL / DATABASE_URL belum diisi")
	}
	src, err := sourceURL(*path)
	if err != nil {
		return err
	}
	driverURL, err := toPgx5URL(dbURL)
	if err != nil {
		return err
	}

	m, err := migrate.New(src, driverURL)
	if err != nil {
		return fmt.Errorf("gagal menyiapkan migrasi: %w", err)
	}
	// Error penutupan tidak bisa ditindaklanjuti saat proses CLI selesai.
	defer func() { _, _ = m.Close() }()

	switch cmd {
	case "up":
		return report(m.Up())
	case "down":
		return runDown(m, params)
	case "goto":
		v, err := parseVersion(params)
		if err != nil {
			return err
		}
		return report(m.Migrate(v))
	case "force":
		v, err := parseVersion(params)
		if err != nil {
			return err
		}
		return m.Force(int(v))
	case "version":
		v, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			fmt.Println("versi: belum ada migrasi yang diterapkan")
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Printf("versi: %d (dirty: %t)\n", v, dirty)
		return nil
	default:
		return fmt.Errorf("perintah tidak dikenal: %q", cmd)
	}
}

func runDown(m *migrate.Migrate, params []string) error {
	if len(params) == 1 && params[0] == "all" {
		return report(m.Down())
	}
	n := 1
	if len(params) == 1 {
		v, err := strconv.Atoi(params[0])
		if err != nil || v < 1 {
			return fmt.Errorf("jumlah langkah down harus angka >= 1 atau 'all': %q", params[0])
		}
		n = v
	} else if len(params) > 1 {
		return errors.New("pemakaian: down [N|all]")
	}
	return report(m.Steps(-n))
}

func report(err error) error {
	switch {
	case err == nil:
		fmt.Println("migrasi selesai")
		return nil
	case errors.Is(err, migrate.ErrNoChange):
		fmt.Println("tidak ada perubahan")
		return nil
	default:
		return err
	}
}

func parseVersion(params []string) (uint, error) {
	if len(params) != 1 {
		return 0, errors.New("pemakaian: <perintah> VERSION")
	}
	v, err := strconv.ParseUint(params[0], 10, 32)
	if err != nil {
		return 0, fmt.Errorf("versi harus angka: %q", params[0])
	}
	return uint(v), nil
}

// sourceURL membangun URL sumber file yang aman di Windows maupun Linux.
func sourceURL(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return "", fmt.Errorf("folder migrasi tidak ditemukan: %s", abs)
	}
	// Driver file golang-migrate memakai host+path dari URL. Linux: file:///abs/path;
	// Windows: file://C:/abs/path (tanpa slash ekstra di depan drive letter).
	return "file://" + filepath.ToSlash(abs), nil
}

// toPgx5URL mengubah postgres(ql):// menjadi pgx5:// (nama driver golang-migrate untuk pgx v5).
func toPgx5URL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("URL database tidak valid")
	}
	switch u.Scheme {
	case "postgres", "postgresql", "pgx5":
		u.Scheme = "pgx5"
	default:
		return "", fmt.Errorf("skema URL database tidak didukung: %q", u.Scheme)
	}
	return u.String(), nil
}

var (
	nameRe    = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)
	versionRe = regexp.MustCompile(`^(\d{6})_.+\.(up|down)\.sql$`)
)

// newMigration membuat pasangan file NNNNNN_nama.up.sql / .down.sql dengan nomor berikutnya.
func newMigration(dir, name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("nama migrasi harus snake_case huruf kecil/angka: %q", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	next := 1
	for _, e := range entries {
		if m := versionRe.FindStringSubmatch(e.Name()); m != nil {
			if v, _ := strconv.Atoi(m[1]); v >= next {
				next = v + 1
			}
		}
	}

	base := fmt.Sprintf("%06d_%s", next, name)
	for _, kind := range []string{"up", "down"} {
		p := filepath.Join(dir, fmt.Sprintf("%s.%s.sql", base, kind))
		body := fmt.Sprintf("-- %s: %s\n", kind, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return err
		}
		fmt.Println("dibuat:", p)
	}
	return nil
}
