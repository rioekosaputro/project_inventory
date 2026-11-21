package domain

import (
	"context"
	"time"
)

// RefreshToken adalah satu token refresh (hanya hash SHA-256 yang disimpan).
type RefreshToken struct {
	ID         int64
	UserID     int64
	TokenHash  string // sha256 hex
	FamilyID   string // UUID; satu keluarga = satu rantai rotasi (satu sesi login)
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *int64
}

// AuditEntry adalah satu baris audit_logs. IP, User-Agent, dan request id diisi
// repository dari context (lihat pkg/reqctx).
type AuditEntry struct {
	UserID     *int64
	Action     string
	EntityType string
	EntityID   *int64
	Before     any
	After      any
}

// Aksi audit untuk autentikasi.
const (
	AuditLogin                = "LOGIN"
	AuditLoginFailed          = "LOGIN_FAILED"
	AuditLogout               = "LOGOUT"
	AuditRefreshReuseDetected = "REFRESH_REUSE_DETECTED"
	AuditPasswordChanged      = "PASSWORD_CHANGED"
	AuditPasswordChangeFailed = "PASSWORD_CHANGE_FAILED"
)

// UserRepository — SQL saja, tanpa aturan bisnis.
type UserRepository interface {
	// FindByEmail / FindByID mengembalikan ErrNotFound bila tidak ada. Email dicocokkan persis (sudah lowercase).
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id int64) (*User, error)
	// UpdatePassword mengganti hash dan mengosongkan must_change_password.
	UpdatePassword(ctx context.Context, userID int64, hash string) error
	TouchLastLogin(ctx context.Context, userID int64, at time.Time) error
}

// RefreshTokenRepository. Create membuat keluarga baru bila familyID == "".
type RefreshTokenRepository interface {
	Create(ctx context.Context, t *RefreshToken, userAgent, ip string) (*RefreshToken, error)
	// LockByHash membaca token dengan SELECT ... FOR UPDATE (wajib di dalam transaksi).
	// ErrNotFound bila hash tidak dikenal.
	LockByHash(ctx context.Context, hash string) (*RefreshToken, error)
	// MarkRotated menandai token lama sudah dipakai: revoked_at = at, replaced_by = newID.
	MarkRotated(ctx context.Context, id, newID int64, at time.Time) error
	RevokeFamily(ctx context.Context, familyID string, at time.Time) (int64, error)
	RevokeAllForUser(ctx context.Context, userID int64, at time.Time) (int64, error)
}

// LoginAttemptRepository menyimpan percobaan gagal untuk rate limit (state di DB, bukan memori).
type LoginAttemptRepository interface {
	Record(ctx context.Context, email, ip string, at time.Time) error
	CountByEmailIP(ctx context.Context, email, ip string, since time.Time) (int, error)
	CountByIP(ctx context.Context, ip string, since time.Time) (int, error)
	// OldestSince mengembalikan waktu percobaan tertua dalam jendela (untuk Retry-After).
	OldestByEmailIP(ctx context.Context, email, ip string, since time.Time) (time.Time, error)
	OldestByIP(ctx context.Context, ip string, since time.Time) (time.Time, error)
	Clear(ctx context.Context, email, ip string) error
	DeleteBefore(ctx context.Context, before time.Time) error
}

// AuditRepository — append-only.
type AuditRepository interface {
	Insert(ctx context.Context, e AuditEntry) error
}
