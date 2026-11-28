//go:build integration

package http_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/config"
	"rental-inventory-api/internal/app"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/testutil/pgtest"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/password"
)

const (
	testPassword = "Passw0rd-Rahasia"
	defaultIP    = "203.0.113.10"
)

var (
	hashOnce     sync.Once
	passwordHash string
)

// bcrypt cost 12 lambat; hash sekali untuk semua user uji.
func testPasswordHash(t *testing.T) string {
	t.Helper()
	hashOnce.Do(func() {
		h, err := password.Hash(testPassword)
		require.NoError(t, err)
		passwordHash = h
	})
	return passwordHash
}

type env struct {
	t      *testing.T
	pool   *pgxpool.Pool
	router *gin.Engine
	clk    *clock.Fake
	cfg    *config.Config
}

func newEnv(t *testing.T, mutate ...func(*config.Config)) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, "../../../migrations", "up")

	cfg := &config.Config{
		AppEnv:             config.EnvLocal,
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		JWTAccessSecret:    "test-secret-0123456789abcdef0123456789",
		JWTAccessTTL:       15 * time.Minute,
		SessionIdleTTL:     30 * time.Minute,
		SessionMaxTTL:      12 * time.Hour,
		AuthRefreshMode:    "cookie",
	}
	for _, m := range mutate {
		m(cfg)
	}
	clk := clock.NewFake(time.Now().UTC().Truncate(time.Second))

	router, err := app.NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), pool, clk)
	require.NoError(t, err)
	return &env{t: t, pool: pool, router: router, clk: clk, cfg: cfg}
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	_, err := e.pool.Exec(testCtx(), sql, args...)
	require.NoError(e.t, err)
}

func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.pool.QueryRow(testCtx(), sql, args...).Scan(&n))
	return n
}

func (e *env) addUser(email string, role domain.Role) int64 {
	e.t.Helper()
	var id int64
	require.NoError(e.t, e.pool.QueryRow(testCtx(), `
		INSERT INTO users (name, email, password_hash, role, must_change_password)
		VALUES ($1, $1, $2, $3, FALSE) RETURNING id`, email, testPasswordHash(e.t), string(role)).Scan(&id))
	return id
}

// --- HTTP helpers ---

type reply struct {
	rec  *httptest.ResponseRecorder
	body map[string]any
}

func (r *reply) status() int          { return r.rec.Code }
func (r *reply) code() string         { s, _ := r.body["code"].(string); return s }
func (r *reply) data() map[string]any { d, _ := r.body["data"].(map[string]any); return d }
func (r *reply) str(key string) string {
	s, _ := r.data()[key].(string)
	return s
}
func (r *reply) cookie(name string) *nethttp.Cookie {
	for _, c := range r.rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

type mod func(*nethttp.Request)

func withIP(ip string) mod { return func(r *nethttp.Request) { r.RemoteAddr = ip + ":4000" } }
func withBearer(tok string) mod {
	return func(r *nethttp.Request) { r.Header.Set("Authorization", "Bearer "+tok) }
}
func withUA(ua string) mod { return func(r *nethttp.Request) { r.Header.Set("User-Agent", ua) } }
func withRefreshCookie(v string) mod {
	return func(r *nethttp.Request) { r.AddCookie(&nethttp.Cookie{Name: "erp_rt", Value: v}) }
}

func (e *env) call(method, path string, body any, mods ...mod) *reply {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(e.t, err)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.RemoteAddr = defaultIP + ":4000"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, m := range mods {
		m(req)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)

	out := &reply{rec: rec}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.body)
	return out
}

func (e *env) login(email, pw string, mods ...mod) *reply {
	return e.call("POST", "/api/v1/auth/login", map[string]string{"email": email, "password": pw}, mods...)
}

func (e *env) refresh(rt string, mods ...mod) *reply {
	return e.call("POST", "/api/v1/auth/refresh", nil, append([]mod{withRefreshCookie(rt)}, mods...)...)
}

func (e *env) mustLogin(email string) (access, refresh string) {
	e.t.Helper()
	r := e.login(email, testPassword)
	require.Equal(e.t, 200, r.status(), "login %s: %s", email, r.rec.Body.String())
	c := r.cookie("erp_rt")
	require.NotNil(e.t, c)
	return r.str("access_token"), c.Value
}

func sha(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

// ================================================================ login

func TestLogin_Success_ReturnsSessionAndSecureCookie(t *testing.T) {
	e := newEnv(t)
	id := e.addUser("head@x.id", domain.RoleHeadWarehouse)

	r := e.login("head@x.id", testPassword, withUA("TestBrowser/1.0"))

	require.Equal(t, 200, r.status(), r.rec.Body.String())
	assert.NotEmpty(t, r.str("access_token"))
	assert.Equal(t, "Bearer", r.str("token_type"))
	assert.EqualValues(t, 900, r.data()["expires_in"])
	assert.Equal(t, "/lpb/approvals", r.str("default_home"))
	assert.Nil(t, r.data()["refresh_token"], "mode cookie: refresh token tidak boleh ada di body")

	user := r.data()["user"].(map[string]any)
	assert.EqualValues(t, id, user["id"])
	assert.Equal(t, "HEAD_WAREHOUSE", user["role"])
	assert.NotContains(t, r.rec.Body.String(), "password_hash", "hash tidak boleh bocor")
	assert.NotContains(t, r.rec.Body.String(), "$2a$", "hash bcrypt tidak boleh bocor")
	assert.NotContains(t, r.rec.Body.String(), testPassword)

	perms := r.data()["permissions"].([]any)
	assert.Contains(t, perms, "lpb.approve")
	assert.NotContains(t, perms, "user.manage")

	c := r.cookie("erp_rt")
	require.NotNil(t, c)
	assert.True(t, c.HttpOnly)
	assert.Equal(t, "/api/v1/auth", c.Path)
	assert.Equal(t, nethttp.SameSiteLaxMode, c.SameSite)
	assert.Greater(t, c.MaxAge, 0)

	// Di DB hanya hash SHA-256 yang disimpan, bukan token asli.
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`, sha(c.Value)))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`, c.Value))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM users WHERE id = $1 AND last_login_at IS NOT NULL`, id))
}

func TestLogin_CookieSecureFlagFollowsConfig(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.CookieSecure = true })
	e.addUser("a@x.id", domain.RoleAdmin)

	assert.True(t, e.login("a@x.id", testPassword).cookie("erp_rt").Secure)
}

func TestLogin_EmailIsCaseAndWhitespaceInsensitive(t *testing.T) {
	e := newEnv(t)
	e.addUser("admin@x.id", domain.RoleAdmin)

	assert.Equal(t, 200, e.login("  ADMIN@X.ID ", testPassword).status())
}

func TestLogin_WrongPasswordAndUnknownEmail_AreIndistinguishable(t *testing.T) {
	e := newEnv(t)
	e.addUser("real@x.id", domain.RoleAdmin)

	wrongPw := e.login("real@x.id", "salah-banget", withIP("198.51.100.1"))
	unknown := e.login("tidak-ada@x.id", testPassword, withIP("198.51.100.2"))

	assert.Equal(t, 401, wrongPw.status())
	assert.Equal(t, 401, unknown.status())
	assert.Equal(t, "INVALID_CREDENTIALS", wrongPw.code())
	assert.Equal(t, wrongPw.body["message"], unknown.body["message"], "pesan harus sama agar email tidak bisa ditebak")
	assert.Equal(t, wrongPw.code(), unknown.code())
	assert.Nil(t, wrongPw.cookie("erp_rt"))
}

func TestLogin_ValidationErrors(t *testing.T) {
	e := newEnv(t)

	r := e.call("POST", "/api/v1/auth/login", map[string]string{"email": "", "password": ""})
	assert.Equal(t, 400, r.status())
	assert.Equal(t, "VALIDATION_ERROR", r.code())

	r = e.call("POST", "/api/v1/auth/login", map[string]string{"email": string(bytes.Repeat([]byte("a"), 200)), "password": "x"})
	assert.Equal(t, 400, r.status(), "email > 120 karakter ditolak sebelum menyentuh DB (kolom VARCHAR(120))")

	// Format email salah bukan 400: diperlakukan sebagai kredensial salah.
	assert.Equal(t, 401, e.login("bukan-email", "x").status())
}

func TestLogin_DisabledAccount_403_OnlyAfterCorrectPassword(t *testing.T) {
	e := newEnv(t)
	id := e.addUser("off@x.id", domain.RoleDriver)
	e.exec(`UPDATE users SET is_active = FALSE WHERE id = $1`, id)

	assert.Equal(t, 401, e.login("off@x.id", "salah").status(), "password salah tidak boleh membocorkan status akun")

	r := e.login("off@x.id", testPassword)
	assert.Equal(t, 403, r.status())
	assert.Equal(t, "ACCOUNT_DISABLED", r.code())
	assert.Nil(t, r.cookie("erp_rt"))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM refresh_tokens`))
}

// ================================================================ rate limit

func TestLogin_RateLimit_PerIPAndEmail(t *testing.T) {
	e := newEnv(t)
	e.addUser("victim@x.id", domain.RoleAdmin)
	e.addUser("other@x.id", domain.RoleAdmin)

	for i := 0; i < 5; i++ {
		assert.Equal(t, 401, e.login("victim@x.id", "salah").status(), "percobaan %d", i+1)
	}

	// Percobaan ke-6 diblokir — walau password BENAR.
	r := e.login("victim@x.id", testPassword)
	assert.Equal(t, 429, r.status())
	assert.Equal(t, "TOO_MANY_REQUESTS", r.code())
	retry, err := strconv.Atoi(r.rec.Header().Get("Retry-After"))
	require.NoError(t, err, "header Retry-After wajib ada")
	assert.InDelta(t, 900, retry, 2)
	assert.Equal(t, 5, e.count(`SELECT count(*) FROM login_attempts`), "saat terblokir tidak ada baris baru (tabel tidak bisa dibanjiri)")

	// Email lain dari IP yang sama tidak terblokir; email yang sama dari IP lain pun tidak.
	assert.Equal(t, 200, e.login("other@x.id", testPassword).status())
	assert.Equal(t, 200, e.login("victim@x.id", testPassword, withIP("198.51.100.77")).status(),
		"penyerang tidak bisa mengunci pemilik akun yang login dari IP berbeda")

	// Setelah jendela 15 menit berlalu, boleh lagi.
	e.clk.Advance(15*time.Minute + time.Second)
	assert.Equal(t, 200, e.login("victim@x.id", testPassword).status())
}

func TestLogin_RateLimit_RetryAfterShrinksAsTimePasses(t *testing.T) {
	e := newEnv(t)
	e.addUser("v@x.id", domain.RoleAdmin)
	for i := 0; i < 5; i++ {
		e.login("v@x.id", "salah")
	}

	e.clk.Advance(10 * time.Minute)
	r := e.login("v@x.id", testPassword)

	require.Equal(t, 429, r.status())
	retry, _ := strconv.Atoi(r.rec.Header().Get("Retry-After"))
	assert.InDelta(t, 300, retry, 2)
}

func TestLogin_SuccessClearsFailedAttempts(t *testing.T) {
	e := newEnv(t)
	e.addUser("v@x.id", domain.RoleAdmin)

	for i := 0; i < 4; i++ {
		e.login("v@x.id", "salah")
	}
	require.Equal(t, 4, e.count(`SELECT count(*) FROM login_attempts`))
	require.Equal(t, 200, e.login("v@x.id", testPassword).status())

	assert.Equal(t, 0, e.count(`SELECT count(*) FROM login_attempts`))
}

func TestLogin_RateLimit_PasswordSprayingFromOneIPIsBlocked(t *testing.T) {
	e := newEnv(t)
	e.addUser("target@x.id", domain.RoleAdmin)

	for i := 0; i < 20; i++ { // 20 email berbeda → tiap (IP,email) hanya 1 kegagalan
		assert.Equal(t, 401, e.login(fmt.Sprintf("u%d@x.id", i), "salah").status())
	}

	r := e.login("target@x.id", testPassword)
	assert.Equal(t, 429, r.status(), "batas gabungan per IP harus menahan penyemprotan password")

	assert.Equal(t, 200, e.login("target@x.id", testPassword, withIP("198.51.100.99")).status(), "IP lain tidak terdampak")
}

func TestLogin_SpoofedForwardedForDoesNotBypassRateLimit(t *testing.T) {
	e := newEnv(t) // tanpa TRUSTED_PROXIES: X-Forwarded-For diabaikan
	e.addUser("v@x.id", domain.RoleAdmin)

	for i := 0; i < 5; i++ {
		e.login("v@x.id", "salah", func(r *nethttp.Request) { r.Header.Set("X-Forwarded-For", fmt.Sprintf("10.9.9.%d", i)) })
	}

	r := e.login("v@x.id", testPassword, func(r *nethttp.Request) { r.Header.Set("X-Forwarded-For", "10.7.7.7") })
	assert.Equal(t, 429, r.status())
}

// ================================================================ refresh

func TestRefresh_RotatesToken_OldOneBecomesUnusable(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)
	_, rt1 := e.mustLogin("a@x.id")

	r := e.refresh(rt1)

	require.Equal(t, 200, r.status(), r.rec.Body.String())
	rt2 := r.cookie("erp_rt")
	require.NotNil(t, rt2)
	assert.NotEqual(t, rt1, rt2.Value, "token harus berganti setiap refresh")
	assert.NotEmpty(t, r.str("access_token"))
	assert.NotNil(t, r.data()["user"], "refresh juga mengembalikan data user (untuk bootstrap FE)")

	// Satu keluarga, token lama ditandai sudah dirotasi dan menunjuk ke penggantinya.
	assert.Equal(t, 1, e.count(`SELECT count(DISTINCT family_id) FROM refresh_tokens`))
	assert.Equal(t, 1, e.count(`
		SELECT count(*) FROM refresh_tokens o JOIN refresh_tokens n ON n.id = o.replaced_by
		WHERE o.token_hash = $1 AND o.revoked_at IS NOT NULL AND n.token_hash = $2 AND n.revoked_at IS NULL`,
		sha(rt1), sha(rt2.Value)))
}

// DoD T0.6: reuse refresh token me-revoke seluruh family.
func TestRefresh_ReuseOfRotatedToken_RevokesWholeFamily(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)
	_, rt1 := e.mustLogin("a@x.id")

	r2 := e.refresh(rt1) // sah: rt1 → rt2
	require.Equal(t, 200, r2.status())
	rt2 := r2.cookie("erp_rt").Value
	r3 := e.refresh(rt2) // sah: rt2 → rt3
	require.Equal(t, 200, r3.status())
	rt3 := r3.cookie("erp_rt").Value

	// Penyerang memakai rt1 yang sudah usang.
	reuse := e.refresh(rt1)
	assert.Equal(t, 401, reuse.status())
	assert.Equal(t, "INVALID_TOKEN", reuse.code())
	cleared := reuse.cookie("erp_rt")
	require.NotNil(t, cleared, "cookie harus dihapus")
	assert.Less(t, cleared.MaxAge, 0)

	// Akibatnya token AKTIF milik pengguna sah (rt3) juga mati: seluruh family dicabut.
	assert.Equal(t, 401, e.refresh(rt3).status(), "token aktif dalam family yang sama harus ikut dicabut")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NULL`),
		"tidak boleh ada token aktif tersisa di family")

	// Pencabutan harus ter-COMMIT (bukan ikut rollback) dan tercatat di audit — sekali saja:
	// memakai token yang dicabut karena family-revoke (rt3) bukan "reuse" baru.
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'REFRESH_REUSE_DETECTED'`))
}

func TestRefresh_ReuseDoesNotAffectOtherSessionsOfSameUser(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)
	_, laptopRT := e.mustLogin("a@x.id")
	_, phoneRT := e.mustLogin("a@x.id") // sesi/family lain

	r := e.refresh(laptopRT)
	require.Equal(t, 200, r.status())
	require.Equal(t, 401, e.refresh(laptopRT).status()) // reuse di laptop

	assert.Equal(t, 200, e.refresh(phoneRT).status(), "sesi di perangkat lain tidak boleh ikut terkunci")
}

func TestRefresh_InvalidTokens_401(t *testing.T) {
	e := newEnv(t)

	for name, rt := range map[string]string{"tanpa cookie": "", "acak": "token-ngawur-tidak-ada", "sangat panjang": string(bytes.Repeat([]byte("a"), 5000))} {
		t.Run(name, func(t *testing.T) {
			var r *reply
			if rt == "" {
				r = e.call("POST", "/api/v1/auth/refresh", nil)
			} else {
				r = e.refresh(rt)
			}
			assert.Equal(t, 401, r.status())
			assert.Equal(t, "INVALID_TOKEN", r.code())
		})
	}
}

// Batas tidak aktif: 30 menit tanpa rotasi refresh token → sesi berakhir.
func TestRefresh_IdleTimeout_401(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)
	_, rt := e.mustLogin("a@x.id")

	e.clk.Advance(30*time.Minute + time.Second)

	assert.Equal(t, 401, e.refresh(rt).status())
}

func TestRefresh_ActivityExtendsIdleWindow(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)
	_, rt := e.mustLogin("a@x.id")

	e.clk.Advance(25 * time.Minute)
	r := e.refresh(rt)
	require.Equal(t, 200, r.status())

	e.clk.Advance(25 * time.Minute) // 50 menit sejak login, tetapi 25 menit sejak rotasi terakhir
	assert.Equal(t, 200, e.refresh(r.cookie("erp_rt").Value).status())
}

// Batas absolut: terus aktif pun wajib login ulang 12 jam setelah login.
func TestRefresh_AbsoluteSessionLimit(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)

	login := e.login("a@x.id", testPassword)
	require.Equal(t, 200, login.status())
	assert.EqualValues(t, 12*3600, login.data()["session_expires_in"])
	assert.EqualValues(t, 30*60, login.data()["idle_timeout_seconds"])
	assert.Equal(t, 30*60, login.cookie("erp_rt").MaxAge)
	rt := login.cookie("erp_rt").Value

	// 28 × 25 menit = 11 jam 40 menit.
	var last *reply
	for i := 0; i < 28; i++ {
		e.clk.Advance(25 * time.Minute)
		last = e.refresh(rt)
		require.Equal(t, 200, last.status(), "rotasi ke-%d", i+1)
		rt = last.cookie("erp_rt").Value
	}

	// Sisa 20 menit: cookie dan hitung mundur dipotong di batas absolut.
	assert.EqualValues(t, 20*60, last.data()["session_expires_in"])
	assert.Equal(t, 20*60, last.cookie("erp_rt").MaxAge)
	assert.EqualValues(t, 15*60, last.data()["expires_in"])

	e.clk.Advance(10 * time.Minute)
	near := e.refresh(rt)
	require.Equal(t, 200, near.status())
	assert.EqualValues(t, 10*60, near.data()["expires_in"], "access token tidak boleh melewati batas sesi")

	e.clk.Advance(10*time.Minute + time.Second)
	assert.Equal(t, 401, e.refresh(near.cookie("erp_rt").Value).status())

	// Login ulang memulai sesi baru.
	assert.Equal(t, 200, e.login("a@x.id", testPassword).status())
}

// Token 14 hari yang terbit sebelum aturan ini ikut tunduk pada batas baru.
func TestRefresh_LegacyLongLivedTokenFollowsNewLimits(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)
	_, rt := e.mustLogin("a@x.id")
	e.exec(`UPDATE refresh_tokens SET expires_at = created_at + INTERVAL '14 days'`)

	e.clk.Advance(31 * time.Minute)

	assert.Equal(t, 401, e.refresh(rt).status())
}

func TestRefresh_ParallelUseOfSameToken_OnlyOneWins(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)
	_, rt := e.mustLogin("a@x.id")

	const n = 6
	var wg sync.WaitGroup
	statuses := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/api/v1/auth/refresh", nil)
			req.RemoteAddr = defaultIP + ":4000"
			req.AddCookie(&nethttp.Cookie{Name: "erp_rt", Value: rt})
			rec := httptest.NewRecorder()
			e.router.ServeHTTP(rec, req)
			statuses[i] = rec.Code
		}(i)
	}
	wg.Wait()

	ok := 0
	for _, s := range statuses {
		if s == 200 {
			ok++
		} else {
			assert.Equal(t, 401, s)
		}
	}
	assert.Equal(t, 1, ok, "FOR UPDATE harus menyerialkan: tepat satu refresh berhasil, statuses=%v", statuses)
}

func TestRefresh_DisabledUser_403AndFamilyRevoked(t *testing.T) {
	e := newEnv(t)
	id := e.addUser("a@x.id", domain.RoleAdmin)
	_, rt := e.mustLogin("a@x.id")

	e.exec(`UPDATE users SET is_active = FALSE WHERE id = $1`, id)
	r := e.refresh(rt)

	assert.Equal(t, 403, r.status())
	assert.Equal(t, "ACCOUNT_DISABLED", r.code())
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NULL`), "pencabutan harus ter-commit")
}

func TestRefresh_ReflectsRoleChangeInNewAccessToken(t *testing.T) {
	e := newEnv(t)
	id := e.addUser("a@x.id", domain.RoleStaffWarehouse)
	_, rt := e.mustLogin("a@x.id")

	e.exec(`UPDATE users SET role = 'HEAD_WAREHOUSE' WHERE id = $1`, id)
	r := e.refresh(rt)

	assert.Equal(t, "HEAD_WAREHOUSE", r.data()["user"].(map[string]any)["role"])
	assert.Contains(t, r.data()["permissions"], "lpb.approve")
}

// ================================================================ logout

func TestLogout_RevokesSessionClearsCookieAndIsIdempotent(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)
	_, rt := e.mustLogin("a@x.id")

	r := e.call("POST", "/api/v1/auth/logout", nil, withRefreshCookie(rt))
	assert.Equal(t, 200, r.status())
	require.NotNil(t, r.cookie("erp_rt"))
	assert.Less(t, r.cookie("erp_rt").MaxAge, 0)

	assert.Equal(t, 401, e.refresh(rt).status(), "refresh setelah logout harus gagal")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'LOGOUT'`))

	assert.Equal(t, 200, e.call("POST", "/api/v1/auth/logout", nil, withRefreshCookie(rt)).status(), "logout ulang tetap 200")
	assert.Equal(t, 200, e.call("POST", "/api/v1/auth/logout", nil).status(), "tanpa cookie pun 200")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'REFRESH_REUSE_DETECTED'`),
		"token yang dicabut lewat logout bukan indikasi pencurian")
}

// ================================================================ me

func TestMe_RequiresValidAccessToken(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleDriver)
	access, _ := e.mustLogin("a@x.id")

	assert.Equal(t, 401, e.call("GET", "/api/v1/auth/me", nil).status())
	assert.Equal(t, 401, e.call("GET", "/api/v1/auth/me", nil, withBearer("sampah")).status())

	r := e.call("GET", "/api/v1/auth/me", nil, withBearer(access))
	require.Equal(t, 200, r.status())
	assert.Equal(t, "/driver/deliveries", r.str("default_home"))
	assert.Equal(t, []any{"driver", "lpb.print"}, r.data()["permissions"], "DRIVER hanya punya tugasnya + cetak surat jalan")

	e.clk.Advance(16 * time.Minute)
	assert.Equal(t, 401, e.call("GET", "/api/v1/auth/me", nil, withBearer(access)).status(), "access token kedaluwarsa setelah 15 menit")
}

func TestMe_DisabledUserWithStillValidTokenIsRejected(t *testing.T) {
	e := newEnv(t)
	id := e.addUser("a@x.id", domain.RoleAdmin)
	access, _ := e.mustLogin("a@x.id")

	e.exec(`UPDATE users SET is_active = FALSE WHERE id = $1`, id)

	r := e.call("GET", "/api/v1/auth/me", nil, withBearer(access))
	assert.Equal(t, 403, r.status())
	assert.Equal(t, "ACCOUNT_DISABLED", r.code())
}

// ================================================================ change password

func TestChangePassword_FullFlow(t *testing.T) {
	e := newEnv(t)
	id := e.addUser("new@x.id", domain.RoleStaffWarehouse)
	e.exec(`UPDATE users SET must_change_password = TRUE WHERE id = $1`, id)

	r := e.login("new@x.id", testPassword)
	require.Equal(t, 200, r.status())
	access, oldRT := r.str("access_token"), r.cookie("erp_rt").Value
	assert.Equal(t, true, r.data()["user"].(map[string]any)["must_change_password"])

	// Masih boleh melihat profil.
	assert.Equal(t, 200, e.call("GET", "/api/v1/auth/me", nil, withBearer(access)).status())

	// Ganti password.
	cp := e.call("POST", "/api/v1/auth/change-password", map[string]string{
		"current_password": testPassword, "new_password": "Sandi-Baru-123",
	}, withBearer(access))
	require.Equal(t, 200, cp.status(), cp.rec.Body.String())
	assert.Equal(t, false, cp.data()["user"].(map[string]any)["must_change_password"])
	newAccess, newRT := cp.str("access_token"), cp.cookie("erp_rt")
	require.NotNil(t, newRT)

	// Sesi lama dicabut; sesi baru berfungsi.
	assert.Equal(t, 401, e.refresh(oldRT).status(), "refresh token lama harus dicabut setelah ganti password")
	assert.Equal(t, 200, e.refresh(newRT.Value).status())
	assert.Equal(t, 200, e.call("GET", "/api/v1/auth/me", nil, withBearer(newAccess)).status())

	// Password lama tidak lagi berlaku, yang baru berlaku, dan flag sudah turun di DB.
	assert.Equal(t, 401, e.login("new@x.id", testPassword).status())
	assert.Equal(t, 200, e.login("new@x.id", "Sandi-Baru-123").status())
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM users WHERE id = $1 AND must_change_password = FALSE`, id))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'PASSWORD_CHANGED' AND user_id = $1`, id))
}

func TestChangePassword_Validation(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)
	access, _ := e.mustLogin("a@x.id")
	post := func(cur, next string) *reply {
		return e.call("POST", "/api/v1/auth/change-password",
			map[string]string{"current_password": cur, "new_password": next}, withBearer(access))
	}

	r := post("password-lama-salah", "Sandi-Baru-123")
	assert.Equal(t, 422, r.status())
	assert.Equal(t, "INVALID_DATA", r.code())

	r = post(testPassword, testPassword)
	assert.Equal(t, 422, r.status(), "password baru tidak boleh sama dengan yang lama")

	r = post(testPassword, "pendek")
	assert.Equal(t, 400, r.status())
	assert.Equal(t, "VALIDATION_ERROR", r.code())

	assert.Equal(t, 401, e.call("POST", "/api/v1/auth/change-password",
		map[string]string{"current_password": testPassword, "new_password": "Sandi-Baru-123"}).status(), "wajib login")

	assert.Equal(t, 0, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'PASSWORD_CHANGED'`), "semua percobaan di atas gagal")
}

func TestChangePassword_WrongCurrentPasswordIsRateLimited(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)
	access, _ := e.mustLogin("a@x.id")
	attempt := func(cur string) *reply {
		return e.call("POST", "/api/v1/auth/change-password",
			map[string]string{"current_password": cur, "new_password": "Sandi-Baru-123"}, withBearer(access))
	}

	for i := 0; i < 5; i++ {
		assert.Equal(t, 422, attempt("salah").status())
	}

	// Brute force password lama lewat access token curian harus mentok.
	r := attempt(testPassword)
	assert.Equal(t, 429, r.status())
	assert.NotEmpty(t, r.rec.Header().Get("Retry-After"))
}

// ================================================================ mode body

func TestRefreshModeBody_ReturnsTokenInBodyAndAcceptsItBack(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.AuthRefreshMode = "body" })
	e.addUser("a@x.id", domain.RoleAdmin)

	r := e.login("a@x.id", testPassword)
	require.Equal(t, 200, r.status())
	rt := r.str("refresh_token")
	assert.NotEmpty(t, rt)
	assert.Nil(t, r.cookie("erp_rt"), "mode body: tidak ada cookie")

	r2 := e.call("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": rt})
	require.Equal(t, 200, r2.status())
	assert.NotEqual(t, rt, r2.str("refresh_token"))

	assert.Equal(t, 401, e.call("POST", "/api/v1/auth/refresh", map[string]string{"refresh_token": rt}).status(), "reuse tetap terdeteksi di mode body")
}

// ================================================================ audit

func TestAudit_LoginEventsCarryClientMetadata(t *testing.T) {
	e := newEnv(t)
	id := e.addUser("a@x.id", domain.RoleAdmin)

	failed := e.login("ghost@x.id", "salah", withUA("Mozilla/TestUA"), withIP("198.51.100.5"))
	ok := e.login("a@x.id", testPassword, withUA("Mozilla/TestUA"), withIP("198.51.100.5"))

	var (
		userID    *int64
		ip, ua    string
		requestID string
		email     string
	)
	require.NoError(t, e.pool.QueryRow(testCtx(), `
		SELECT user_id, host(ip_address), user_agent, request_id, after_data->>'email'
		FROM audit_logs WHERE action = 'LOGIN_FAILED'`).Scan(&userID, &ip, &ua, &requestID, &email))
	assert.Nil(t, userID, "email tak dikenal: user_id kosong")
	assert.Equal(t, "198.51.100.5", ip)
	assert.Equal(t, "Mozilla/TestUA", ua)
	assert.Equal(t, failed.rec.Header().Get("X-Request-ID"), requestID, "audit harus bisa ditautkan ke request lewat request_id")
	assert.Equal(t, "ghost@x.id", email)

	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'LOGIN' AND user_id = $1 AND request_id = $2`,
		id, ok.rec.Header().Get("X-Request-ID")))
}

func TestAudit_FailedLoginForKnownUserRecordsUserID(t *testing.T) {
	e := newEnv(t)
	id := e.addUser("a@x.id", domain.RoleAdmin)

	e.login("a@x.id", "salah")

	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_logs WHERE action = 'LOGIN_FAILED' AND user_id = $1`, id))
}

func TestAudit_NoPasswordOrTokenEverWrittenToAuditLog(t *testing.T) {
	e := newEnv(t)
	e.addUser("a@x.id", domain.RoleAdmin)
	_, rt := e.mustLogin("a@x.id")
	e.login("a@x.id", "rahasia-yang-salah-123")
	e.refresh(rt)

	assert.Equal(t, 0, e.count(`SELECT count(*) FROM audit_logs WHERE after_data::text LIKE '%rahasia-yang-salah-123%' OR after_data::text LIKE '%' || $1 || '%'`, rt))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM audit_logs WHERE after_data::text ILIKE '%password%' AND after_data::text NOT ILIKE '%reason%'`))
}
