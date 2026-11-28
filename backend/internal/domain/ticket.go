package domain

import (
	"context"
	"time"
)

type TicketStatus string

const (
	TicketOpen        TicketStatus = "OPEN"
	TicketAssigned    TicketStatus = "ASSIGNED"
	TicketInProgress  TicketStatus = "IN_PROGRESS"
	TicketWaitingPart TicketStatus = "WAITING_PART"
	TicketResolved    TicketStatus = "RESOLVED"
	TicketClosed      TicketStatus = "CLOSED"
	TicketCancelled   TicketStatus = "CANCELLED"
)

func (s TicketStatus) Valid() bool {
	switch s {
	case TicketOpen, TicketAssigned, TicketInProgress, TicketWaitingPart,
		TicketResolved, TicketClosed, TicketCancelled:
		return true
	}
	return false
}

type TicketPriority string

const (
	PriorityLow      TicketPriority = "LOW"
	PriorityMedium   TicketPriority = "MEDIUM"
	PriorityHigh     TicketPriority = "HIGH"
	PriorityCritical TicketPriority = "CRITICAL"
)

func (p TicketPriority) Valid() bool {
	switch p {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityCritical:
		return true
	}
	return false
}

type TicketCategory string

const (
	CatPaperJam     TicketCategory = "PAPER_JAM"
	CatPrintQuality TicketCategory = "PRINT_QUALITY"
	CatHardware     TicketCategory = "HARDWARE"
	CatSoftware     TicketCategory = "SOFTWARE"
	CatNetwork      TicketCategory = "NETWORK"
	CatPreventive   TicketCategory = "PREVENTIVE"
	CatOther        TicketCategory = "OTHER"
)

func (c TicketCategory) Valid() bool {
	switch c {
	case CatPaperJam, CatPrintQuality, CatHardware, CatSoftware, CatNetwork, CatPreventive, CatOther:
		return true
	}
	return false
}

type TicketReportedVia string

const (
	ViaPhone    TicketReportedVia = "PHONE"
	ViaWhatsApp TicketReportedVia = "WHATSAPP"
	ViaEmail    TicketReportedVia = "EMAIL"
	ViaPortal   TicketReportedVia = "PORTAL"
	ViaInternal TicketReportedVia = "INTERNAL"
)

func (v TicketReportedVia) Valid() bool {
	switch v {
	case ViaPhone, ViaWhatsApp, ViaEmail, ViaPortal, ViaInternal:
		return true
	}
	return false
}

type TicketActivityType string

const (
	ActivityComment       TicketActivityType = "COMMENT"
	ActivityStatusChange  TicketActivityType = "STATUS_CHANGE"
	ActivityAssign        TicketActivityType = "ASSIGN"
	ActivityVisitCheckin  TicketActivityType = "VISIT_CHECKIN"
	ActivityVisitCheckout TicketActivityType = "VISIT_CHECKOUT"
	ActivityPhoto         TicketActivityType = "PHOTO"
)

func (t TicketActivityType) ValidManual() bool {
	switch t {
	case ActivityComment, ActivityVisitCheckin, ActivityVisitCheckout, ActivityPhoto:
		return true
	}
	return false
}

// MaintenanceTicket adalah entitas tiket maintenance unit sewa (T3.8).
type MaintenanceTicket struct {
	ID              int64
	TicketNumber    string
	CustomerID      int64
	CustomerName    string
	CustomerSiteID  int64
	SiteName        string
	SiteAddress     string
	SiteCity        string
	ProductSerialID int64
	AssetTag        string
	SerialNumber    string
	ProductName     string
	ReportedByName  *string
	ReportedVia     TicketReportedVia
	Category        TicketCategory
	Priority        TicketPriority
	Status          TicketStatus
	Title           string
	Description     *string
	AssignedTo      *int64
	AssignedToName  *string
	SLADueAt        *time.Time
	ResolutionNotes *string
	ResolvedAt      *time.Time
	ClosedAt        *time.Time
	SwapSOID        *int64
	SwapSONumber    *string
	CreatedBy       int64
	CreatedByName   string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type TicketActivity struct {
	ID           int64
	TicketID     int64
	UserID       int64
	UserName     string
	ActivityType TicketActivityType
	Message      *string
	FromStatus   *string
	ToStatus     *string
	Latitude     *float64
	Longitude    *float64
	PhotoPath    *string
	PhotoURL     *string // diisi di usecase dari storage.Uploader.SignedURL
	CreatedAt    time.Time
}

type TicketSparepart struct {
	ID              int64
	TicketID        int64
	ProductID       int64
	SKU             string
	ProductName     string
	Qty             int
	Status          string
	LPBID           *int64
	LPBNumber       *string
	IsBillable      bool
	RequestedBy     int64
	RequestedByName string
	RequestedAt     time.Time
	Notes           *string
}

type TicketDetail struct {
	MaintenanceTicket
	Activities []TicketActivity
	Spareparts []TicketSparepart
}

type TicketFilter struct {
	Query      string
	Status     *TicketStatus
	Priority   *TicketPriority
	AssignedTo *int64
	CustomerID *int64
}

type TicketRepository interface {
	List(ctx context.Context, f TicketFilter, p PageParams) ([]MaintenanceTicket, int64, error)
	FindByID(ctx context.Context, id int64) (*MaintenanceTicket, error)
	FindByIDForUpdate(ctx context.Context, id int64) (*MaintenanceTicket, error)
	Create(ctx context.Context, t *MaintenanceTicket) (*MaintenanceTicket, error)
	Update(ctx context.Context, t *MaintenanceTicket) error
	Assign(ctx context.Context, id int64, technicianID int64, newStatus TicketStatus) error
	UpdateStatus(ctx context.Context, id int64, status TicketStatus, resolutionNotes *string, resolvedAt, closedAt *time.Time) error
	ListActivities(ctx context.Context, ticketID int64) ([]TicketActivity, error)
	InsertActivity(ctx context.Context, a *TicketActivity) (*TicketActivity, error)
	ListSpareparts(ctx context.Context, ticketID int64) ([]TicketSparepart, error)
	InsertSparepart(ctx context.Context, sp *TicketSparepart) (*TicketSparepart, error)
	FindSparepartByIDForUpdate(ctx context.Context, id int64) (*TicketSparepart, error)
	MarkSparepartLPBCreated(ctx context.Context, sparepartID, lpbID int64) error
	MarkSparepartIssuedByLPB(ctx context.Context, lpbID int64) ([]TicketSparepart, error)
}
