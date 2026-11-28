// Package config memuat konfigurasi aplikasi dari environment variable.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	EnvLocal      = "local"
	EnvStaging    = "staging"
	EnvProduction = "production"

	PoolerSession     = "session"
	PoolerTransaction = "transaction"

	minSecretLen = 32
)

// Config menampung seluruh konfigurasi aplikasi (lihat plan Bagian 7.3).
type Config struct {
	AppEnv             string
	Port               string
	AppBaseURL         string
	CORSAllowedOrigins []string
	// TrustedProxies: IP/CIDR proxy yang boleh menentukan IP klien lewat X-Forwarded-For.
	// Kosong = tidak ada yang dipercaya (IP klien = alamat koneksi langsung).
	TrustedProxies []string
	TZApp          string
	Location       *time.Location

	DatabaseURL        string
	MigrateDatabaseURL string
	DBPoolerMode       string
	DBMaxConns         int

	JWTAccessSecret string
	JWTAccessTTL    time.Duration
	AuthRefreshMode string
	CookieSecure    bool

	// SessionIdleTTL: sesi berakhir bila tidak ada rotasi refresh token selama ini (tidak aktif).
	SessionIdleTTL time.Duration
	// SessionMaxTTL: batas absolut sejak login; setelahnya wajib login ulang walau terus aktif.
	SessionMaxTTL time.Duration

	GotenbergURL      string
	GotenbergUsername string
	GotenbergPassword string

	StorageDriver    string
	StorageLocalDir  string
	S3Endpoint       string
	S3Region         string
	S3Bucket         string
	S3AccessKey      string
	S3SecretKey      string
	S3ForcePathStyle bool

	CronSecret        string
	SeedAdminEmail    string
	SeedAdminPassword string
}

// IsLocal true jika berjalan di mesin developer.
func (c *Config) IsLocal() bool { return c.AppEnv == EnvLocal }

// MigrateURL mengembalikan URL untuk migrasi (fallback ke DatabaseURL).
func (c *Config) MigrateURL() string {
	if c.MigrateDatabaseURL != "" {
		return c.MigrateDatabaseURL
	}
	return c.DatabaseURL
}

// Load membaca file .env (jika ada, hanya untuk pengembangan lokal) lalu
// environment variable. Variabel yang sudah diset di OS tidak ditimpa .env.
func Load() (*Config, error) {
	_ = godotenv.Load() // abaikan error: .env opsional (staging/production memakai env host)
	return loadFromEnv()
}

func loadFromEnv() (*Config, error) {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	c := &Config{
		AppEnv:             getEnv("APP_ENV", EnvLocal),
		Port:               getEnv("PORT", "8080"),
		AppBaseURL:         getEnv("APP_BASE_URL", "http://localhost:5173"),
		CORSAllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
		TrustedProxies:     splitCSV(os.Getenv("TRUSTED_PROXIES")),
		TZApp:              getEnv("TZ_APP", "Asia/Jakarta"),

		DatabaseURL:        os.Getenv("DATABASE_URL"),
		MigrateDatabaseURL: os.Getenv("MIGRATE_DATABASE_URL"),
		DBPoolerMode:       getEnv("DB_POOLER_MODE", PoolerSession),

		JWTAccessSecret: os.Getenv("JWT_ACCESS_SECRET"),
		AuthRefreshMode: getEnv("AUTH_REFRESH_MODE", "cookie"),

		GotenbergURL:      getEnv("GOTENBERG_URL", "http://localhost:3000"),
		GotenbergUsername: os.Getenv("GOTENBERG_USERNAME"),
		GotenbergPassword: os.Getenv("GOTENBERG_PASSWORD"),

		StorageDriver:   getEnv("STORAGE_DRIVER", "local"),
		StorageLocalDir: getEnv("STORAGE_LOCAL_DIR", "./storage"),
		S3Endpoint:      os.Getenv("S3_ENDPOINT"),
		S3Region:        os.Getenv("S3_REGION"),
		S3Bucket:        getEnv("S3_BUCKET", "erp-files"),
		S3AccessKey:     os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:     os.Getenv("S3_SECRET_KEY"),

		CronSecret:        os.Getenv("CRON_SECRET"),
		SeedAdminEmail:    os.Getenv("SEED_ADMIN_EMAIL"),
		SeedAdminPassword: os.Getenv("SEED_ADMIN_PASSWORD"),
	}

	var err error
	if c.DBMaxConns, err = getInt("DB_MAX_CONNS", 10); err != nil {
		errs = append(errs, err)
	}
	if c.JWTAccessTTL, err = getDuration("JWT_ACCESS_TTL", 15*time.Minute); err != nil {
		errs = append(errs, err)
	}
	if c.SessionIdleTTL, err = getDuration("SESSION_IDLE_TTL", 30*time.Minute); err != nil {
		errs = append(errs, err)
	}
	if c.SessionMaxTTL, err = getDuration("SESSION_MAX_TTL", 12*time.Hour); err != nil {
		errs = append(errs, err)
	}
	if c.CookieSecure, err = getBool("COOKIE_SECURE", false); err != nil {
		errs = append(errs, err)
	}
	if c.S3ForcePathStyle, err = getBool("S3_FORCE_PATH_STYLE", true); err != nil {
		errs = append(errs, err)
	}

	// Validasi nilai.
	switch c.AppEnv {
	case EnvLocal, EnvStaging, EnvProduction:
	default:
		fail("APP_ENV tidak valid: %q (local | staging | production)", c.AppEnv)
	}
	if p, perr := strconv.Atoi(c.Port); perr != nil || p < 1 || p > 65535 {
		fail("PORT tidak valid: %q", c.Port)
	}
	switch c.DBPoolerMode {
	case PoolerSession, PoolerTransaction:
	default:
		fail("DB_POOLER_MODE tidak valid: %q (session | transaction)", c.DBPoolerMode)
	}
	switch c.AuthRefreshMode {
	case "cookie", "body":
	default:
		fail("AUTH_REFRESH_MODE tidak valid: %q (cookie | body)", c.AuthRefreshMode)
	}
	switch c.StorageDriver {
	case "local", "s3":
	default:
		fail("STORAGE_DRIVER tidak valid: %q (local | s3)", c.StorageDriver)
	}
	if c.SessionIdleTTL <= 0 || c.SessionMaxTTL < c.SessionIdleTTL {
		fail("SESSION_IDLE_TTL harus > 0 dan SESSION_MAX_TTL tidak boleh lebih kecil darinya")
	}
	if c.DBMaxConns < 1 {
		fail("DB_MAX_CONNS harus >= 1")
	}
	for _, o := range c.CORSAllowedOrigins {
		if o == "*" {
			fail("CORS_ALLOWED_ORIGINS tidak boleh '*' (cookie refresh memakai credentials); sebutkan origin eksplisit")
		}
	}
	for _, p := range c.TrustedProxies {
		if _, _, err := net.ParseCIDR(p); err != nil && net.ParseIP(p) == nil {
			fail("TRUSTED_PROXIES berisi IP/CIDR tidak valid: %q", p)
		}
	}

	c.Location, err = time.LoadLocation(c.TZApp)
	if err != nil {
		fail("TZ_APP tidak valid: %q (%v)", c.TZApp, err)
	}

	// Di luar lokal, secret wajib kuat dan cookie wajib secure.
	if c.AppEnv != EnvLocal {
		if len(c.JWTAccessSecret) < minSecretLen {
			fail("JWT_ACCESS_SECRET wajib diisi minimal %d karakter di %s", minSecretLen, c.AppEnv)
		}
		if !c.CookieSecure {
			fail("COOKIE_SECURE wajib true di %s", c.AppEnv)
		}
	}

	return c, errors.Join(errs...)
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func getInt(key string, def int) (int, error) {
	v := getEnv(key, "")
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s harus berupa angka: %q", key, v)
	}
	return n, nil
}

func getBool(key string, def bool) (bool, error) {
	v := getEnv(key, "")
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s harus true/false: %q", key, v)
	}
	return b, nil
}

func getDuration(key string, def time.Duration) (time.Duration, error) {
	v := getEnv(key, "")
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s harus berupa durasi (mis. 15m): %q", key, v)
	}
	return d, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
