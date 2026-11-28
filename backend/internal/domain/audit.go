package domain

import (
	"context"
	"time"
)

// AuditLogRow adalah satu baris log audit untuk halaman Audit Log Viewer (T4.3).
type AuditLogRow struct {
	ID         int64     `json:"id"`
	UserID     *int64    `json:"user_id,omitempty"`
	UserName   *string   `json:"user_name,omitempty"`
	UserEmail  *string   `json:"user_email,omitempty"`
	Action     string    `json:"action"`
	EntityType *string   `json:"entity_type,omitempty"`
	EntityID   *int64    `json:"entity_id,omitempty"`
	BeforeData any       `json:"before_data,omitempty"`
	AfterData  any       `json:"after_data,omitempty"`
	IPAddress  *string   `json:"ip_address,omitempty"`
	UserAgent  *string   `json:"user_agent,omitempty"`
	RequestID  *string   `json:"request_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// AuditFilter adalah parameter filter daftar audit_logs.
type AuditFilter struct {
	Query      string
	Action     string
	EntityType string
	UserID     *int64
	DateFrom   *time.Time
	DateTo     *time.Time
}

// AuditReader membaca log audit secara paginasi untuk Admin & Manajemen (PermAuditRead).
type AuditReader interface {
	List(ctx context.Context, f AuditFilter, p PageParams) ([]AuditLogRow, int64, error)
}
