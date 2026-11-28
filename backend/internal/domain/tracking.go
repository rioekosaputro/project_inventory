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

// MapRentedUnit adalah ringkasan unit RENTED pada satu titik cabang di peta aset (T3.7).
type MapRentedUnit struct {
	SerialID     int64
	AssetTag     string
	SerialNumber string
	SKU          string
	ProductName  string
	ContractID   *int64
}

// MapSitePin adalah satu pin lokasi cabang pelanggan beserta daftar unit RENTED aktif (T3.7).
type MapSitePin struct {
	SiteID       int64
	SiteName     string
	CustomerID   int64
	CustomerName string
	AddressLine  string
	City         string
	Province     string
	PICName      string
	PICPhone     string
	Latitude     *float64
	Longitude    *float64
	Units        []MapRentedUnit
}

// MapAssetsFilter menyaring pin peta sebaran aset.
type MapAssetsFilter struct {
	Query      string
	CustomerID *int64
}

// TrackingRepository mengelola shipment_tracking_logs, peta aset, dan query tugas driver (T3.3 & T3.7).
type TrackingRepository interface {
	ListMyDeliveries(ctx context.Context, driverID int64) ([]DriverDeliveryTask, error)
	FindByClientEventID(ctx context.Context, clientEventID string) (*ShipmentTrackingLog, error)
	ListByLPB(ctx context.Context, lpbID int64) ([]ShipmentTrackingLog, error)
	Insert(ctx context.Context, log *ShipmentTrackingLog) error
	UpdateLPBDeliveryStatus(ctx context.Context, lpbID int64, status TrackingStatus) error
	SiteCoordinates(ctx context.Context, siteID int64) (*SiteCoords, error)
	ListMapAssets(ctx context.Context, f MapAssetsFilter) ([]MapSitePin, error)
}
