// Package jwt menerbitkan dan memverifikasi access token (HS256).
// Claims (plan Bagian 10): sub, role, wid, exp — ditambah mcp (must change password).
package jwt

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

const (
	issuer    = "rental-erp-api"
	leeway    = 5 * time.Second
	minSecret = 32
)

// Error hasil verifikasi. Detail alasan tidak dibocorkan ke klien.
var (
	ErrInvalid = errors.New("token tidak valid")
	ErrExpired = errors.New("token kedaluwarsa")
)

// Claims adalah data identitas di dalam access token.
type Claims struct {
	UserID             int64
	Role               string
	WarehouseID        *int64
	MustChangePassword bool
}

type wireClaims struct {
	Role               string `json:"role"`
	WarehouseID        *int64 `json:"wid,omitempty"`
	MustChangePassword bool   `json:"mcp,omitempty"`
	jwtlib.RegisteredClaims
}

// Manager menerbitkan & memverifikasi token dengan satu secret.
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager menolak secret yang lemah/placeholder agar tidak ada token yang
// ditandatangani dengan kunci yang bisa ditebak.
func NewManager(secret string, ttl time.Duration) (*Manager, error) {
	if len(secret) < minSecret {
		return nil, fmt.Errorf("JWT_ACCESS_SECRET minimal %d karakter acak", minSecret)
	}
	if strings.HasPrefix(secret, "ganti-") {
		return nil, errors.New("JWT_ACCESS_SECRET masih nilai contoh; ganti dengan string acak (mis. `openssl rand -hex 32`)")
	}
	if ttl <= 0 {
		return nil, errors.New("JWT_ACCESS_TTL harus > 0")
	}
	return &Manager{secret: []byte(secret), ttl: ttl}, nil
}

// TTL umur access token.
func (m *Manager) TTL() time.Duration { return m.ttl }

// Issue menerbitkan token yang berlaku dari now selama TTL.
func (m *Manager) Issue(c Claims, now time.Time) (string, time.Time, error) {
	exp := now.Add(m.ttl)
	jti := make([]byte, 8)
	_, _ = rand.Read(jti)

	wc := wireClaims{
		Role:               c.Role,
		WarehouseID:        c.WarehouseID,
		MustChangePassword: c.MustChangePassword,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer:    issuer,
			Subject:   strconv.FormatInt(c.UserID, 10),
			IssuedAt:  jwtlib.NewNumericDate(now),
			ExpiresAt: jwtlib.NewNumericDate(exp),
			ID:        hex.EncodeToString(jti),
		},
	}
	signed, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, wc).SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("jwt: gagal menandatangani: %w", err)
	}
	return signed, exp, nil
}

// Parse memverifikasi tanda tangan, algoritma (hanya HS256 — menolak "none"/RS256 confusion),
// issuer, dan kedaluwarsa terhadap now.
func (m *Manager) Parse(token string, now time.Time) (Claims, error) {
	var wc wireClaims
	_, err := jwtlib.ParseWithClaims(token, &wc,
		func(*jwtlib.Token) (any, error) { return m.secret, nil },
		jwtlib.WithValidMethods([]string{jwtlib.SigningMethodHS256.Alg()}),
		jwtlib.WithIssuer(issuer),
		jwtlib.WithExpirationRequired(),
		jwtlib.WithTimeFunc(func() time.Time { return now }),
		jwtlib.WithLeeway(leeway),
	)
	if err != nil {
		if errors.Is(err, jwtlib.ErrTokenExpired) {
			return Claims{}, ErrExpired
		}
		return Claims{}, ErrInvalid
	}

	uid, err := strconv.ParseInt(wc.Subject, 10, 64)
	if err != nil || uid <= 0 || wc.Role == "" {
		return Claims{}, ErrInvalid
	}
	return Claims{UserID: uid, Role: wc.Role, WarehouseID: wc.WarehouseID, MustChangePassword: wc.MustChangePassword}, nil
}
