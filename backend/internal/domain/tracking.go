package domain

import (
	"context"
	"time"
)

// TrackingStatus adalah status checkpoint pengiriman driver (CHECK di shipment_tracking_logs).
type TrackingStatus string

const (
	TrackingPickedUp  TrackingStatus = "PICKED_UP"
	TrackingInTransit TrackingStatus = "IN_TRANSIT"
	TrackingArrived   TrackingStatus = "ARRIVED"
	TrackingDelivered TrackingStatus = "DELIVERED"
	TrackingFailed    TrackingStatus = "FAILED"
)

func (s TrackingStatus) ValidCheckpoint() bool {
	return s == TrackingPickedUp || s == TrackingInTransit || s == TrackingArrived
}

// ShipmentTrackingLog adalah satu entri checkpoint GPS / POD / gagal kirim milik driver.
type ShipmentTrackingLog struct {
	ID            int64
	LPBID         int64
	DriverID      int64
	DriverName    string
	Status        TrackingStatus
	Latitude      float64
	Longitude     float64
	AccuracyM     *int
	RecordedAt    time.Time
	ClientTime    *time.Time
	ClientEventID *string
	ReceiverName  *string
	PhotoPath     *string
	FailureReason *string
	Notes         *string
}

// DriverDeliveryTask adalah satu kartu tugas aktif (kirim atau jemput) untuk driver mobile.
type DriverDeliveryTask struct {
	LPBID          int64
	LPBNumber      string
	Type           LPBType
	SubType        LPBSubType
	Status         LPBStatus
	DeliveryStatus *string
	DocDate        string
	WarehouseID    int64
	WarehouseCode  string
	WarehouseName  string
	CustomerID     *int64
	CustomerName   *string
	CustomerSiteID *int64
	SiteName       *string
	SiteAddress    *string
	SiteCity       *string
	SitePICName    *string
	SitePICPhone   *string
	SiteLatitude   *float64
	SiteLongitude  *float64
	ItemCount      int
	TotalQty       int
	Notes          *string
}

// SiteCoords adalah koordinat cabang pelanggan untuk peringatan jarak GPS POD (> 500 m).
type SiteCoords struct {
	Latitude  *float64
	Longitude *float64
}

// TrackingRepository mengelola shipment_tracking_logs dan query tugas driver (T3.3).
type TrackingRepository interface {
	ListMyDeliveries(ctx context.Context, driverID int64) ([]DriverDeliveryTask, error)
	FindByClientEventID(ctx context.Context, clientEventID string) (*ShipmentTrackingLog, error)
	ListByLPB(ctx context.Context, lpbID int64) ([]ShipmentTrackingLog, error)
	Insert(ctx context.Context, log *ShipmentTrackingLog) error
	UpdateLPBDeliveryStatus(ctx context.Context, lpbID int64, status TrackingStatus) error
	SiteCoordinates(ctx context.Context, siteID int64) (*SiteCoords, error)
}
