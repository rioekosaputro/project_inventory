package jwt

import (
	"strings"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "0123456789abcdef0123456789abcdef"

var now = time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

func newManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(secret, 15*time.Minute)
	require.NoError(t, err)
	return m
}

func TestNewManager_RejectsWeakSecrets(t *testing.T) {
	for _, s := range []string{"", "pendek", "ganti-dengan-random-64-char-ganti-dengan-random-64", strings.Repeat("a", 31)} {
		_, err := NewManager(s, time.Minute)
		assert.Error(t, err, "secret %q harus ditolak", s)
	}
	_, err := NewManager(secret, 0)
	assert.Error(t, err)
}

func TestIssueAndParse_RoundTrip(t *testing.T) {
	m := newManager(t)
	wid := int64(7)

	tok, exp, err := m.Issue(Claims{UserID: 42, Role: "HEAD_WAREHOUSE", WarehouseID: &wid, MustChangePassword: true}, now)
	require.NoError(t, err)
	assert.Equal(t, now.Add(15*time.Minute), exp)

	got, err := m.Parse(tok, now.Add(time.Minute))
	require.NoError(t, err)
	assert.EqualValues(t, 42, got.UserID)
	assert.Equal(t, "HEAD_WAREHOUSE", got.Role)
	require.NotNil(t, got.WarehouseID)
	assert.EqualValues(t, 7, *got.WarehouseID)
	assert.True(t, got.MustChangePassword)
}

func TestParse_NilWarehouseStaysNil(t *testing.T) {
	m := newManager(t)
	tok, _, _ := m.Issue(Claims{UserID: 1, Role: "ADMIN"}, now)

	got, err := m.Parse(tok, now)
	require.NoError(t, err)
	assert.Nil(t, got.WarehouseID)
	assert.False(t, got.MustChangePassword)
}

func TestParse_Expired(t *testing.T) {
	m := newManager(t)
	tok, _, _ := m.Issue(Claims{UserID: 1, Role: "ADMIN"}, now)

	_, err := m.Parse(tok, now.Add(15*time.Minute+leeway+time.Second))
	assert.ErrorIs(t, err, ErrExpired)

	_, err = m.Parse(tok, now.Add(15*time.Minute-time.Second))
	assert.NoError(t, err, "masih berlaku tepat sebelum kedaluwarsa")
}

func TestParse_RejectsTamperedAndForeignTokens(t *testing.T) {
	m := newManager(t)
	good, _, _ := m.Issue(Claims{UserID: 1, Role: "STAFF_WAREHOUSE"}, now)

	t.Run("tanda tangan diubah", func(t *testing.T) {
		parts := strings.Split(good, ".")
		_, err := m.Parse(parts[0]+"."+parts[1]+"."+parts[2][:len(parts[2])-2]+"xx", now)
		assert.ErrorIs(t, err, ErrInvalid)
	})

	t.Run("payload diubah (naik jadi ADMIN)", func(t *testing.T) {
		other, _, _ := m.Issue(Claims{UserID: 1, Role: "ADMIN"}, now)
		forged := strings.Split(good, ".")[0] + "." + strings.Split(other, ".")[1] + "." + strings.Split(good, ".")[2]
		_, err := m.Parse(forged, now)
		assert.ErrorIs(t, err, ErrInvalid)
	})

	t.Run("secret berbeda", func(t *testing.T) {
		other, _ := NewManager(strings.Repeat("z", 40), time.Minute)
		tok, _, _ := other.Issue(Claims{UserID: 1, Role: "ADMIN"}, now)
		_, err := m.Parse(tok, now)
		assert.ErrorIs(t, err, ErrInvalid)
	})

	t.Run("alg none", func(t *testing.T) {
		tok, err := jwtlib.NewWithClaims(jwtlib.SigningMethodNone, wireClaims{
			Role: "ADMIN",
			RegisteredClaims: jwtlib.RegisteredClaims{
				Issuer: issuer, Subject: "1", ExpiresAt: jwtlib.NewNumericDate(now.Add(time.Hour)),
			},
		}).SignedString(jwtlib.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)
		_, err = m.Parse(tok, now)
		assert.ErrorIs(t, err, ErrInvalid)
	})

	t.Run("alg HS512 ditolak (hanya HS256)", func(t *testing.T) {
		tok, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS512, wireClaims{
			Role: "ADMIN",
			RegisteredClaims: jwtlib.RegisteredClaims{
				Issuer: issuer, Subject: "1", ExpiresAt: jwtlib.NewNumericDate(now.Add(time.Hour)),
			},
		}).SignedString([]byte(secret))
		require.NoError(t, err)
		_, err = m.Parse(tok, now)
		assert.ErrorIs(t, err, ErrInvalid)
	})

	t.Run("issuer salah", func(t *testing.T) {
		tok, _ := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, wireClaims{
			Role: "ADMIN",
			RegisteredClaims: jwtlib.RegisteredClaims{
				Issuer: "orang-lain", Subject: "1", ExpiresAt: jwtlib.NewNumericDate(now.Add(time.Hour)),
			},
		}).SignedString([]byte(secret))
		_, err := m.Parse(tok, now)
		assert.ErrorIs(t, err, ErrInvalid)
	})

	t.Run("tanpa exp ditolak", func(t *testing.T) {
		tok, _ := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, wireClaims{
			Role:             "ADMIN",
			RegisteredClaims: jwtlib.RegisteredClaims{Issuer: issuer, Subject: "1"},
		}).SignedString([]byte(secret))
		_, err := m.Parse(tok, now)
		assert.ErrorIs(t, err, ErrInvalid)
	})

	t.Run("subject bukan angka / role kosong", func(t *testing.T) {
		for _, c := range []wireClaims{
			{Role: "ADMIN", RegisteredClaims: jwtlib.RegisteredClaims{Issuer: issuer, Subject: "abc", ExpiresAt: jwtlib.NewNumericDate(now.Add(time.Hour))}},
			{Role: "", RegisteredClaims: jwtlib.RegisteredClaims{Issuer: issuer, Subject: "1", ExpiresAt: jwtlib.NewNumericDate(now.Add(time.Hour))}},
		} {
			tok, _ := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, c).SignedString([]byte(secret))
			_, err := m.Parse(tok, now)
			assert.ErrorIs(t, err, ErrInvalid)
		}
	})

	t.Run("sampah", func(t *testing.T) {
		for _, s := range []string{"", "abc", "a.b.c", "Bearer xyz"} {
			_, err := m.Parse(s, now)
			assert.ErrorIs(t, err, ErrInvalid, s)
		}
	})
}
