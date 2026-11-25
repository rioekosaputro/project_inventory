package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// DOContext adalah metadata SO/kontrak/pelanggan milik sebuah Delivery Order (untuk POD/Pickup).
type DOContext struct {
	DOID       int64
	SOID       int64
	OrderType  OrderType
	ContractID *int64
	CustomerID int64
	SiteID     int64
}

// DeliveryPODRepository mengelola POD pengiriman (DELIVERED + rental_placements) dan pickup penarikan (T2.7).
type DeliveryPODRepository interface {
	MarkDelivered(ctx context.Context, lpbID int64, deliveredAt time.Time, receiverName string, podPhotoPath *string) error
	MarkPickedUp(ctx context.Context, lpbID int64) error
	LookupDOContext(ctx context.Context, doID int64) (*DOContext, error)
	FindContractItemRate(ctx context.Context, contractID, productID, siteID int64) (*int64, decimal.Decimal, error)
	MarkSerialRented(ctx context.Context, serialID, customerID, siteID int64, contractID *int64) error
	InsertRentalPlacement(ctx context.Context, contractID int64, contractItemID *int64, serialID, siteID int64, monthlyRate decimal.Decimal, startDate string, startLPBID int64) error
	MarkSerialPickupInTransit(ctx context.Context, serialID int64) error
	CloseActivePlacement(ctx context.Context, serialID int64, endDate, endReason string, endLPBID int64) error
	CompleteDOAndMaybeSO(ctx context.Context, doID int64) error
}
