package middleware_test

import (
	"encoding/json"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/jwt"
)

const secret = "0123456789abcdef0123456789abcdef"

var start = time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

type fixture struct {
	engine *gin.Engine
	mgr    *jwt.Manager
	clk    *clock.Fake
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	httperr.Setup()

	mgr, err := jwt.NewManager(secret, 15*time.Minute)
	require.NoError(t, err)
	clk := clock.NewFake(start)

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Recover(slog.New(slog.NewTextHandler(io.Discard, nil))))
	ok := func(c *gin.Context) {
		p, _ := middleware.PrincipalFrom(c)
		c.JSON(200, gin.H{"user_id": p.UserID, "role": p.Role})
	}

	r.GET("/me", middleware.AuthenticateAllowPasswordChange(mgr, clk), ok)
	r.GET("/open", middleware.Authenticate(mgr, clk), ok)
	r.POST("/lpb/:id/approve", middleware.Authenticate(mgr, clk), middleware.RequirePermission(domain.PermLPBApprove), ok)
	r.GET("/users", middleware.Authenticate(mgr, clk), middleware.RequirePermission(domain.PermUserManage), ok)
	r.GET("/no-auth-but-rbac", middleware.RequirePermission(domain.PermLPBRead), ok)
	return &fixture{engine: r, mgr: mgr, clk: clk}
}

func (f *fixture) token(t *testing.T, role domain.Role, mustChange bool) string {
	t.Helper()
	tok, _, err := f.mgr.Issue(jwt.Claims{UserID: 7, Role: string(role), MustChangePassword: mustChange}, f.clk.Now())
	require.NoError(t, err)
	return tok
}

func (f *fixture) do(method, path, authHeader string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	f.engine.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestAuthenticate_MissingOrMalformedHeader_401(t *testing.T) {
	f := newFixture(t)
	good := f.token(t, domain.RoleAdmin, false)

	for name, header := range map[string]string{
		"tanpa header":      "",
		"skema salah":       "Basic " + good,
		"tanpa token":       "Bearer",
		"token kosong":      "Bearer   ",
		"token tanpa skema": good,
		"sampah":            "Bearer bukan.jwt.valid",
	} {
		t.Run(name, func(t *testing.T) {
			rec, body := f.do("GET", "/open", header)
			assert.Equal(t, 401, rec.Code)
			assert.Equal(t, false, body["success"])
			assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "Bearer")
		})
	}
}

func TestAuthenticate_AcceptsValidTokenAndSchemeCaseInsensitive(t *testing.T) {
	f := newFixture(t)
	tok := f.token(t, domain.RoleStaffWarehouse, false)

	for _, scheme := range []string{"Bearer", "bearer", "BEARER"} {
		rec, body := f.do("GET", "/open", scheme+" "+tok)
		assert.Equal(t, 200, rec.Code, scheme)
		assert.EqualValues(t, 7, body["user_id"])
		assert.Equal(t, "STAFF_WAREHOUSE", body["role"])
	}
}

func TestAuthenticate_ExpiredTokenIs401(t *testing.T) {
	f := newFixture(t)
	tok := f.token(t, domain.RoleAdmin, false)

	f.clk.Advance(16 * time.Minute)
	rec, body := f.do("GET", "/open", "Bearer "+tok)

	assert.Equal(t, 401, rec.Code)
	assert.Equal(t, "INVALID_TOKEN", body["code"])
}

func TestAuthenticate_UnknownRoleInTokenIs401(t *testing.T) {
	f := newFixture(t)
	tok, _, err := f.mgr.Issue(jwt.Claims{UserID: 1, Role: "SUPERUSER"}, f.clk.Now())
	require.NoError(t, err)

	rec, _ := f.do("GET", "/open", "Bearer "+tok)
	assert.Equal(t, 401, rec.Code)
}

func TestAuthenticate_MustChangePassword_BlocksEverythingExceptAllowedRoutes(t *testing.T) {
	f := newFixture(t)
	tok := f.token(t, domain.RoleAdmin, true)

	rec, body := f.do("GET", "/open", "Bearer "+tok)
	assert.Equal(t, 403, rec.Code)
	assert.Equal(t, "PASSWORD_CHANGE_REQUIRED", body["code"])

	rec, _ = f.do("POST", "/lpb/1/approve", "Bearer "+tok)
	assert.Equal(t, 403, rec.Code, "bahkan ADMIN tidak boleh melewati kewajiban ganti password")

	rec, _ = f.do("GET", "/me", "Bearer "+tok)
	assert.Equal(t, 200, rec.Code, "/auth/me tetap boleh agar FE tahu harus ke halaman ganti password")
}

// --- DoD T0.6: role tanpa permission → 403 ---

func TestRequirePermission_RoleWithoutPermissionGets403(t *testing.T) {
	f := newFixture(t)

	for _, role := range []domain.Role{
		domain.RoleStaffWarehouse, domain.RoleManagement, domain.RoleBusinessAdmin,
		domain.RoleDriver, domain.RoleTechnician,
	} {
		rec, body := f.do("POST", "/lpb/1/approve", "Bearer "+f.token(t, role, false))
		assert.Equal(t, 403, rec.Code, "%s tidak boleh approve LPB", role)
		assert.Equal(t, "FORBIDDEN", body["code"])
		assert.NotEmpty(t, body["request_id"])
	}
}

func TestRequirePermission_RoleWithPermissionPasses(t *testing.T) {
	f := newFixture(t)

	for _, role := range []domain.Role{domain.RoleHeadWarehouse, domain.RoleAdmin} {
		rec, _ := f.do("POST", "/lpb/1/approve", "Bearer "+f.token(t, role, false))
		assert.Equal(t, 200, rec.Code, role)
	}
}

func TestRequirePermission_AdminOnlyRoutesRejectEveryoneElse(t *testing.T) {
	f := newFixture(t)

	rec, _ := f.do("GET", "/users", "Bearer "+f.token(t, domain.RoleAdmin, false))
	assert.Equal(t, 200, rec.Code)

	for _, role := range domain.Roles() {
		if role == domain.RoleAdmin {
			continue
		}
		rec, _ := f.do("GET", "/users", "Bearer "+f.token(t, role, false))
		assert.Equal(t, 403, rec.Code, role)
	}
}

func TestRequirePermission_WithoutAuthenticateFailsClosed(t *testing.T) {
	f := newFixture(t)
	rec, _ := f.do("GET", "/no-auth-but-rbac", "")
	assert.Equal(t, nethttp.StatusUnauthorized, rec.Code)
}

func TestRequirePermission_UnknownPermissionPanicsAtRegistration(t *testing.T) {
	assert.Panics(t, func() { middleware.RequirePermission("lpb.aprove") })
	assert.NotPanics(t, func() { middleware.RequirePermission(domain.PermLPBApprove) })
}
