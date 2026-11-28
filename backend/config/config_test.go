package config

import (
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clearEnv mengosongkan semua variabel yang dibaca config agar test deterministik.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"APP_ENV", "PORT", "APP_BASE_URL", "CORS_ALLOWED_ORIGINS", "TZ_APP",
		"DATABASE_URL", "MIGRATE_DATABASE_URL", "DB_POOLER_MODE", "DB_MAX_CONNS",
		"JWT_ACCESS_SECRET", "JWT_ACCESS_TTL", "SESSION_IDLE_TTL", "SESSION_MAX_TTL", "AUTH_REFRESH_MODE", "COOKIE_SECURE",
		"STORAGE_DRIVER", "S3_FORCE_PATH_STYLE", "TRUSTED_PROXIES",
	} {
		t.Setenv(k, "")
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)

	cfg, err := loadFromEnv()
	require.NoError(t, err)

	assert.Equal(t, EnvLocal, cfg.AppEnv)
	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, PoolerSession, cfg.DBPoolerMode)
	assert.Equal(t, 10, cfg.DBMaxConns)
	assert.Equal(t, 15*time.Minute, cfg.JWTAccessTTL)
	assert.Equal(t, 30*time.Minute, cfg.SessionIdleTTL)
	assert.Equal(t, 12*time.Hour, cfg.SessionMaxTTL)
	assert.Equal(t, []string{"http://localhost:5173"}, cfg.CORSAllowedOrigins)
	assert.Equal(t, "Asia/Jakarta", cfg.Location.String())
	assert.True(t, cfg.IsLocal())
}

func TestLoad_ReadsPort(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", "9090")

	cfg, err := loadFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "9090", cfg.Port)
}

func TestLoad_InvalidValues(t *testing.T) {
	tests := []struct {
		name, key, value string
	}{
		{"app env", "APP_ENV", "dev"},
		{"port bukan angka", "PORT", "abc"},
		{"port di luar rentang", "PORT", "70000"},
		{"pooler mode", "DB_POOLER_MODE", "pgbouncer"},
		{"max conns", "DB_MAX_CONNS", "banyak"},
		{"durasi", "JWT_ACCESS_TTL", "15menit"},
		{"zona waktu", "TZ_APP", "Mars/Olympus"},
		{"storage driver", "STORAGE_DRIVER", "ftp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(tt.key, tt.value)

			_, err := loadFromEnv()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.key)
		})
	}
}

func TestLoad_RejectsWildcardCORSBecauseOfCredentials(t *testing.T) {
	clearEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,*")

	_, err := loadFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CORS_ALLOWED_ORIGINS")
}

func TestLoad_TrustedProxies(t *testing.T) {
	clearEnv(t)

	cfg, err := loadFromEnv()
	require.NoError(t, err)
	assert.Empty(t, cfg.TrustedProxies, "default: tidak ada proxy yang dipercaya")

	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8, 127.0.0.1")
	cfg, err = loadFromEnv()
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.0/8", "127.0.0.1"}, cfg.TrustedProxies)

	t.Setenv("TRUSTED_PROXIES", "bukan-ip")
	_, err = loadFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TRUSTED_PROXIES")
}

func TestLoad_NonLocalRequiresStrongSecretAndSecureCookie(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", EnvProduction)

	_, err := loadFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JWT_ACCESS_SECRET")
	assert.Contains(t, err.Error(), "COOKIE_SECURE")

	t.Setenv("JWT_ACCESS_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("COOKIE_SECURE", "true")
	cfg, err := loadFromEnv()
	require.NoError(t, err)
	assert.Equal(t, EnvProduction, cfg.AppEnv)
}

// Regresi: komentar di belakang nilai kosong (KEY=   # ...) dibaca godotenv sebagai NILAI.
func TestEnvExample_HasNoCommentLeakingIntoValues(t *testing.T) {
	vars, err := godotenv.Read("../.env.example")
	require.NoError(t, err)
	require.NotEmpty(t, vars)

	for k, v := range vars {
		assert.NotContains(t, v, "#", "%s: nilai mengandung komentar", k)
	}
	assert.Empty(t, vars["MIGRATE_DATABASE_URL"])
	assert.Empty(t, vars["S3_ENDPOINT"])
}

func TestMigrateURL_FallsBackToDatabaseURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://a")
	cfg, err := loadFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "postgres://a", cfg.MigrateURL())

	t.Setenv("MIGRATE_DATABASE_URL", "postgres://b")
	cfg, err = loadFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "postgres://b", cfg.MigrateURL())
}
