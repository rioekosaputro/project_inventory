package handler

import (
	"fmt"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/dto"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/pagination"
	"rental-inventory-api/pkg/response"
)

var balanceSort = pagination.SortSpec{
	Allowed: map[string]string{
		"sku": "p.sku", "name": "p.name", "warehouse": "w.code", "rack": "r.code",
		"qty_on_hand": "sb.qty_on_hand", "qty_available": "qty_available", "updated_at": "sb.updated_at",
	},
	Default: "sku,warehouse,rack",
}

var stockSummarySort = pagination.SortSpec{
	Allowed: map[string]string{
		"sku": "t.sku", "name": "t.name", "category": "t.category_name",
		"qty_on_hand": "t.qty_on_hand", "qty_available": "t.qty_available",
	},
	Default: "sku",
}

var serialSort = pagination.SortSpec{
	Allowed: map[string]string{
		"asset_tag": "ps.asset_tag", "serial_number": "ps.serial_number", "status": "ps.status",
		"sku": "p.sku", "updated_at": "ps.updated_at",
	},
	Default: "asset_tag",
}

// InventoryHandler: /inventory/* (permission inventory.read, dipasang di router).
type InventoryHandler struct{ uc *usecase.InventoryUsecase }

func NewInventoryHandler(uc *usecase.InventoryUsecase) *InventoryHandler {
	return &InventoryHandler{uc: uc}
}

// Balances — GET /inventory/balances
func (h *InventoryHandler) Balances(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	page, pp, ok := pageOf(c, balanceSort)
	if !ok {
		return
	}
	var q dto.ListBalancesQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	items, total, err := h.uc.Balances(c.Request.Context(), a, domain.BalanceFilter{
		Query: q.Q, ProductID: q.ProductID, RackID: q.RackID, WarehouseID: q.WarehouseID,
		LowStock: q.LowStock, IncludeZero: q.IncludeZero,
	}, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.BalanceResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewBalanceResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// ProductStock — GET /inventory/balances/summary
func (h *InventoryHandler) ProductStock(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	page, pp, ok := pageOf(c, stockSummarySort)
	if !ok {
		return
	}
	var q dto.ListStockSummaryQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	items, total, err := h.uc.ProductStock(c.Request.Context(), a, domain.StockSummaryFilter{
		Query: q.Q, CategoryID: q.CategoryID, ItemType: itemTypePtr(q.ItemType), WarehouseID: q.WarehouseID,
		LowStock: q.LowStock, HasStock: q.HasStock, IsActive: q.IsActive,
	}, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.ProductStockResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewProductStockResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Serials — GET /inventory/serials
func (h *InventoryHandler) Serials(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	page, pp, ok := pageOf(c, serialSort)
	if !ok {
		return
	}
	var q dto.ListSerialsQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	f := domain.SerialFilter{
		Query: q.Q, ProductID: q.ProductID, CustomerID: q.CustomerID, WarehouseID: q.WarehouseID, RackID: q.RackID,
	}
	if q.Status != "" {
		st := domain.SerialStatus(q.Status)
		f.Status = &st
	}
	items, total, err := h.uc.Serials(c.Request.Context(), a, f, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.SerialResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewSerialResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// ScanSerial — GET /inventory/serials/scan/:code (asset tag atau serial number)
func (h *InventoryHandler) ScanSerial(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	scan, err := h.uc.ScanSerial(c.Request.Context(), a, c.Param("code"))
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewSerialScanResponse(scan))
}

type serialLabelsRequest struct {
	SerialIDs []int64 `json:"serial_ids" binding:"required"`
}

// SerialLabelsPDF — POST /inventory/serials/labels/pdf (cetak label asset tag batch)
func (h *InventoryHandler) SerialLabelsPDF(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var req serialLabelsRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	pdfBytes, err := h.uc.RenderSerialLabelsPDF(pdfContext(c), a, req.SerialIDs)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	writePDFOrHTML(c, "label-aset.pdf", pdfBytes)
}

var ledgerSort = pagination.SortSpec{
	Allowed: map[string]string{"occurred_at": "l.occurred_at", "id": "l.id", "qty": "l.qty"},
	Default: "-occurred_at,-id",
}

const xlsxMIME = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// SerialHistory — GET /inventory/serials/:id/history (detail unit + timeline lengkap)
func (h *InventoryHandler) SerialHistory(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.uc.SerialHistory(c.Request.Context(), a, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewSerialScanResponse(res))
}

func buildLedgerFilter(c *gin.Context, q *dto.ListLedgersQuery) (domain.LedgerFilter, bool) {
	f := domain.LedgerFilter{Query: q.Q, ProductID: q.ProductID, RackID: q.RackID, WarehouseID: q.WarehouseID}
	if q.Direction != "" {
		dir := q.Direction
		f.Direction = &dir
	}
	if q.MovementType != "" {
		mt := domain.LPBSubType(q.MovementType)
		if !mt.Valid() {
			httperr.BadQueryField(c, "movement_type", "tidak valid")
			return f, false
		}
		f.MovementType = &mt
	}
	if q.DateFrom != "" {
		df := q.DateFrom
		f.DateFrom = &df
	}
	if q.DateTo != "" {
		dt := q.DateTo
		f.DateTo = &dt
	}
	return f, true
}

// Ledgers — GET /inventory/ledgers
func (h *InventoryHandler) Ledgers(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	page, pp, ok := pageOf(c, ledgerSort)
	if !ok {
		return
	}
	var q dto.ListLedgersQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	f, ok := buildLedgerFilter(c, &q)
	if !ok {
		return
	}
	items, total, err := h.uc.Ledgers(c.Request.Context(), a, f, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.LedgerRowResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewLedgerRowResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// ExportLedgers — GET /inventory/ledgers/export (.xlsx)
func (h *InventoryHandler) ExportLedgers(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var q dto.ListLedgersQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	f, ok := buildLedgerFilter(c, &q)
	if !ok {
		return
	}
	data, err := h.uc.ExportLedgersXLSX(c.Request.Context(), a, f)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="buku-besar-stok.xlsx"`)
	c.Data(nethttp.StatusOK, xlsxMIME, data)
}

// StockCard — GET /inventory/stock-card (?format=xlsx untuk export Excel)
func (h *InventoryHandler) StockCard(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var q dto.StockCardQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	f := domain.StockCardFilter{ProductID: q.ProductID, RackID: q.RackID, WarehouseID: q.WarehouseID}
	if q.DateFrom != "" {
		df := q.DateFrom
		f.DateFrom = &df
	}
	if q.DateTo != "" {
		dt := q.DateTo
		f.DateTo = &dt
	}
	if q.Format == "xlsx" {
		data, err := h.uc.ExportStockCardXLSX(c.Request.Context(), a, f)
		if err != nil {
			httperr.Write(c, err)
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="kartu-stok-%d.xlsx"`, q.ProductID))
		c.Data(nethttp.StatusOK, xlsxMIME, data)
		return
	}
	card, err := h.uc.StockCard(c.Request.Context(), a, f)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewStockCardResponse(card))
}
