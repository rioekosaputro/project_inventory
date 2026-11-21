// Package usecase berisi aturan bisnis. Tidak mengimpor Gin maupun pgx; akses data lewat
// interface repository di internal/domain dan transaksi lewat domain.TxManager.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/jwt"
	"rental-inventory-api/pkg/password"
	"rental-inventory-api/pkg/reqctx"
)

// AccessTokenIssuer menerbitkan access JWT (diimplementasi *jwt.Manager).
type AccessTokenIssuer interface {
	Issue(c jwt.Claims, now time.Time) (token string, expiresAt time.Time, err error)
}

// AuthConfig mengatur umur token dan rate limit login (plan Bagian 10: 5 gagal / 15 menit).
type AuthConfig struct {
	RefreshTTL         time.Duration
	AttemptWindow      time.Duration
	MaxFailsPerEmailIP int // percobaan gagal per (IP, email) dalam jendela
	MaxFailsPerIP      int // batas gabungan per IP — menahan password spraying lintas email
}

// DefaultAuthConfig memberi nilai bawaan dari plan.
func DefaultAuthConfig(refreshTTL time.Duration) AuthConfig {
	return AuthConfig{
		RefreshTTL:         refreshTTL,
		AttemptWindow:      15 * time.Minute,
		MaxFailsPerEmailIP: 5,
		MaxFailsPerIP:      20,
	}
}

// Session adalah hasil login/refresh/ganti-password.
type Session struct {
	IssuedAt         time.Time // waktu menurut clock usecase; dasar perhitungan umur token
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string // nilai asli; hanya ada di sini, di DB hanya hash-nya
	RefreshExpiresAt time.Time
	Profile
}

// Profile adalah data pengguna untuk FE (GET /auth/me).
type Profile struct {
	User        *domain.User
	Permissions []domain.Permission
	DefaultHome string
}

type AuthUsecase struct {
	tx       domain.TxManager
	users    domain.UserRepository
	tokens   domain.RefreshTokenRepository
	attempts domain.LoginAttemptRepository
	audit    domain.AuditRepository
	issuer   AccessTokenIssuer
	clock    clock.Clock
	cfg      AuthConfig
}

func NewAuthUsecase(
	tx domain.TxManager,
	users domain.UserRepository,
	tokens domain.RefreshTokenRepository,
	attempts domain.LoginAttemptRepository,
	audit domain.AuditRepository,
	issuer AccessTokenIssuer,
	clk clock.Clock,
	cfg AuthConfig,
) *AuthUsecase {
	return &AuthUsecase{tx: tx, users: users, tokens: tokens, attempts: attempts, audit: audit, issuer: issuer, clock: clk, cfg: cfg}
}

// ---------------------------------------------------------------- Login

// Login memverifikasi kredensial. Gagal → ErrInvalidCredentials (sama untuk email tak dikenal
// dan password salah, dan waktu respons disamakan lewat bcrypt dummy agar email tidak bisa
// ditebak lewat timing). Setiap kegagalan dicatat (rate limit + audit LOGIN_FAILED).
func (u *AuthUsecase) Login(ctx context.Context, email, plainPassword string) (*Session, error) {
	email = normalizeEmail(email)
	now := u.clock.Now()
	ip := reqctx.ClientFrom(ctx).IP

	if err := u.checkRateLimit(ctx, email, ip, now); err != nil {
		return nil, err
	}

	user, err := u.users.FindByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		user = nil
	}

	hash := dummyHash()
	if user != nil {
		hash = user.PasswordHash
	}
	passwordOK := password.Verify(hash, plainPassword) // selalu dijalankan (timing seragam)

	if user == nil || !passwordOK {
		if err := u.recordFailure(ctx, email, ip, user, domain.AuditLoginFailed, "invalid_credentials", now); err != nil {
			return nil, err
		}
		return nil, domain.ErrInvalidCredentials
	}
	if !user.IsActive {
		if err := u.audit.Insert(ctx, domain.AuditEntry{
			UserID: &user.ID, Action: domain.AuditLoginFailed, EntityType: "user", EntityID: &user.ID,
			After: map[string]string{"email": email, "reason": "account_disabled"},
		}); err != nil {
			return nil, err
		}
		return nil, domain.ErrAccountDisabled
	}

	raw, tokHash, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	refreshExp := now.Add(u.cfg.RefreshTTL)

	err = u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := u.attempts.Clear(ctx, email, ip); err != nil {
			return err
		}
		if err := u.users.TouchLastLogin(ctx, user.ID, now); err != nil {
			return err
		}
		if _, err := u.tokens.Create(ctx, &domain.RefreshToken{UserID: user.ID, TokenHash: tokHash, ExpiresAt: refreshExp}, reqctx.ClientFrom(ctx).UserAgent, ip); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			UserID: &user.ID, Action: domain.AuditLogin, EntityType: "user", EntityID: &user.ID,
		})
	})
	if err != nil {
		return nil, err
	}
	return u.buildSession(user, raw, refreshExp, now)
}

// ---------------------------------------------------------------- Refresh

// Refresh menukar refresh token dengan pasangan token baru (rotasi). Token lama langsung
// tidak berlaku. Bila token yang SUDAH dirotasi dipakai lagi (indikasi token dicuri),
// seluruh keluarga token (satu sesi) dicabut dan pemanggil mendapat ErrInvalidToken.
func (u *AuthUsecase) Refresh(ctx context.Context, rawToken string) (*Session, error) {
	if rawToken == "" {
		return nil, domain.ErrInvalidToken
	}
	now := u.clock.Now()
	oldHash := hashToken(rawToken)

	newRaw, newHash, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	refreshExp := now.Add(u.cfg.RefreshTTL)

	var (
		user     *domain.User
		denyWith error // ditentukan di dalam tx, dikembalikan SETELAH commit (lihat catatan)
	)
	// PENTING: revoke family harus ter-COMMIT. Karena itu kasus reuse/akun nonaktif TIDAK
	// mengembalikan error dari closure (itu akan rollback), melainkan mencatat denyWith dan
	// mengembalikan error-nya setelah transaksi selesai.
	err = u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		old, err := u.tokens.LockByHash(ctx, oldHash)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				denyWith = domain.ErrInvalidToken
				return nil
			}
			return err
		}

		if old.RevokedAt != nil {
			// Reuse = token yang SUDAH DIROTASI (replaced_by terisi) dipakai lagi: indikasi token
			// dicuri → cabut seluruh keluarga + audit. Token yang dicabut lewat logout / pencabutan
			// keluarga (replaced_by kosong) cukup ditolak; keluarganya sudah mati.
			if old.ReplacedBy != nil {
				n, err := u.tokens.RevokeFamily(ctx, old.FamilyID, now)
				if err != nil {
					return err
				}
				if err := u.audit.Insert(ctx, domain.AuditEntry{
					UserID: &old.UserID, Action: domain.AuditRefreshReuseDetected, EntityType: "refresh_token", EntityID: &old.ID,
					After: map[string]any{"family_id": old.FamilyID, "revoked_active_tokens": n},
				}); err != nil {
					return err
				}
			}
			denyWith = domain.ErrInvalidToken
			return nil
		}
		if !now.Before(old.ExpiresAt) {
			denyWith = domain.ErrInvalidToken
			return nil
		}

		user, err = u.users.FindByID(ctx, old.UserID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				denyWith = domain.ErrInvalidToken
				return nil
			}
			return err
		}
		if !user.IsActive {
			if _, err := u.tokens.RevokeFamily(ctx, old.FamilyID, now); err != nil {
				return err
			}
			denyWith = domain.ErrAccountDisabled
			return nil
		}

		client := reqctx.ClientFrom(ctx)
		created, err := u.tokens.Create(ctx, &domain.RefreshToken{
			UserID: user.ID, TokenHash: newHash, FamilyID: old.FamilyID, ExpiresAt: refreshExp,
		}, client.UserAgent, client.IP)
		if err != nil {
			return err
		}
		return u.tokens.MarkRotated(ctx, old.ID, created.ID, now)
	})
	if err != nil {
		return nil, err
	}
	if denyWith != nil {
		return nil, denyWith
	}
	return u.buildSession(user, newRaw, refreshExp, now)
}

// ---------------------------------------------------------------- Logout

// Logout mencabut seluruh keluarga token milik refresh token yang diberikan (sesi ini saja).
// Idempoten: token kosong/tidak dikenal dianggap sudah logout.
func (u *AuthUsecase) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	now := u.clock.Now()
	return u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		t, err := u.tokens.LockByHash(ctx, hashToken(rawToken))
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			return err
		}
		if _, err := u.tokens.RevokeFamily(ctx, t.FamilyID, now); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			UserID: &t.UserID, Action: domain.AuditLogout, EntityType: "user", EntityID: &t.UserID,
		})
	})
}

// ---------------------------------------------------------------- Me

// Me mengembalikan profil terkini dari database (bukan dari klaim token), sehingga
// perubahan role/status langsung terlihat.
func (u *AuthUsecase) Me(ctx context.Context, p domain.Principal) (*Profile, error) {
	user, err := u.users.FindByID(ctx, p.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}
	if !user.IsActive {
		return nil, domain.ErrAccountDisabled
	}
	return newProfile(user), nil
}

// ---------------------------------------------------------------- Change password

// ChangePassword mengganti password pengguna yang sedang login. Salah password lama dihitung
// pada rate limit yang sama dengan login (mencegah brute force lewat access token curian).
// Berhasil → semua sesi lama dicabut dan sesi baru diterbitkan (token lama yang masih
// membawa klaim must-change tidak lagi dipakai klien).
func (u *AuthUsecase) ChangePassword(ctx context.Context, p domain.Principal, current, next string) (*Session, error) {
	now := u.clock.Now()
	ip := reqctx.ClientFrom(ctx).IP

	user, err := u.users.FindByID(ctx, p.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}
	if !user.IsActive {
		return nil, domain.ErrAccountDisabled
	}

	if err := u.checkRateLimit(ctx, user.Email, ip, now); err != nil {
		return nil, err
	}
	if !password.Verify(user.PasswordHash, current) {
		if err := u.recordFailure(ctx, user.Email, ip, user, domain.AuditPasswordChangeFailed, "wrong_current_password", now); err != nil {
			return nil, err
		}
		return nil, &domain.ValidationError{
			Message: "Password saat ini salah",
			Errors:  []domain.FieldError{{Field: "current_password", Message: "salah"}},
		}
	}
	if current == next {
		return nil, &domain.ValidationError{
			Message: "Password baru tidak valid",
			Errors:  []domain.FieldError{{Field: "new_password", Message: "tidak boleh sama dengan password saat ini"}},
		}
	}
	newHash, err := password.Hash(next)
	if err != nil {
		return nil, &domain.ValidationError{
			Message: "Password baru tidak valid",
			Errors:  []domain.FieldError{{Field: "new_password", Message: err.Error()}},
		}
	}

	raw, tokHash, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	refreshExp := now.Add(u.cfg.RefreshTTL)

	err = u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := u.users.UpdatePassword(ctx, user.ID, newHash); err != nil {
			return err
		}
		if _, err := u.tokens.RevokeAllForUser(ctx, user.ID, now); err != nil {
			return err
		}
		if err := u.attempts.Clear(ctx, user.Email, ip); err != nil {
			return err
		}
		if _, err := u.tokens.Create(ctx, &domain.RefreshToken{UserID: user.ID, TokenHash: tokHash, ExpiresAt: refreshExp}, reqctx.ClientFrom(ctx).UserAgent, ip); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			UserID: &user.ID, Action: domain.AuditPasswordChanged, EntityType: "user", EntityID: &user.ID,
		})
	})
	if err != nil {
		return nil, err
	}

	user.MustChangePassword = false
	return u.buildSession(user, raw, refreshExp, now)
}

// ---------------------------------------------------------------- helpers

func (u *AuthUsecase) buildSession(user *domain.User, rawRefresh string, refreshExp, now time.Time) (*Session, error) {
	access, accessExp, err := u.issuer.Issue(jwt.Claims{
		UserID:             user.ID,
		Role:               string(user.Role),
		WarehouseID:        user.WarehouseID,
		MustChangePassword: user.MustChangePassword,
	}, now)
	if err != nil {
		return nil, err
	}
	return &Session{
		IssuedAt:         now,
		AccessToken:      access,
		AccessExpiresAt:  accessExp,
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: refreshExp,
		Profile:          *newProfile(user),
	}, nil
}

func newProfile(user *domain.User) *Profile {
	return &Profile{User: user, Permissions: domain.PermissionsFor(user.Role), DefaultHome: user.Role.DefaultHome()}
}

// checkRateLimit menolak bila jumlah kegagalan dalam jendela sudah mencapai batas.
// Saat terblokir TIDAK ada baris baru yang ditulis (tabel tidak bisa dibanjiri penyerang).
func (u *AuthUsecase) checkRateLimit(ctx context.Context, email, ip string, now time.Time) error {
	since := now.Add(-u.cfg.AttemptWindow)

	n, err := u.attempts.CountByEmailIP(ctx, email, ip, since)
	if err != nil {
		return err
	}
	if n >= u.cfg.MaxFailsPerEmailIP {
		oldest, err := u.attempts.OldestByEmailIP(ctx, email, ip, since)
		if err != nil {
			return err
		}
		return u.rateLimited(oldest, now)
	}

	n, err = u.attempts.CountByIP(ctx, ip, since)
	if err != nil {
		return err
	}
	if n >= u.cfg.MaxFailsPerIP {
		oldest, err := u.attempts.OldestByIP(ctx, ip, since)
		if err != nil {
			return err
		}
		return u.rateLimited(oldest, now)
	}
	return nil
}

func (u *AuthUsecase) rateLimited(oldest, now time.Time) error {
	retry := oldest.Add(u.cfg.AttemptWindow).Sub(now)
	if retry < time.Second {
		retry = time.Second
	}
	return &domain.RateLimitError{RetryAfter: retry}
}

// recordFailure mencatat kegagalan dan audit-nya dalam satu transaksi yang DI-COMMIT
// (pemanggil lalu mengembalikan error bisnis di luar transaksi).
func (u *AuthUsecase) recordFailure(ctx context.Context, email, ip string, user *domain.User, action, reason string, now time.Time) error {
	return u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := u.attempts.Record(ctx, email, ip, now); err != nil {
			return err
		}
		entry := domain.AuditEntry{
			Action: action, EntityType: "user",
			After: map[string]string{"email": email, "reason": reason},
		}
		if user != nil {
			entry.UserID, entry.EntityID = &user.ID, &user.ID
		}
		if err := u.audit.Insert(ctx, entry); err != nil {
			return err
		}
		// Pembersihan oportunistik: baris > 24 jam tidak lagi relevan untuk jendela 15 menit.
		return u.attempts.DeleteBefore(ctx, now.Add(-24*time.Hour))
	})
}

func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// newRefreshToken: 32 byte acak (256 bit) → base64url; yang disimpan hanya SHA-256-nya.
func newRefreshToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("auth: gagal membuat refresh token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

var (
	dummyHashOnce  sync.Once
	dummyHashValue string
)

// dummyHash dipakai untuk membandingkan password saat email tidak ditemukan, sehingga
// waktu respons tidak membedakan "email tidak ada" dari "password salah".
func dummyHash() string {
	dummyHashOnce.Do(func() {
		dummyHashValue, _ = password.Hash("kata-sandi-pengecoh-untuk-timing")
	})
	return dummyHashValue
}
