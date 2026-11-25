package usecase

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/pdf"
	"rental-inventory-api/pkg/qrcode"
	"rental-inventory-api/templates"
)

// WithPDF memasang renderer PDF (Gotenberg) dan base URL frontend untuk tautan QR verifikasi.
func (u *LPBUsecase) WithPDF(renderer pdf.Renderer, appBaseURL string) *LPBUsecase {
	u.pdfRenderer = renderer
	u.appBaseURL = strings.TrimRight(strings.TrimSpace(appBaseURL), "/")
	return u
}

// RenderPDF menghasilkan PDF Surat Jalan / LPB untuk dokumen berstatus APPROVED (plan Bagian 7.6 & 8.3).
// Non-APPROVED ditolak 422 (ErrLPBNotApproved). DRIVER hanya boleh mencetak LPB yang ditugaskan kepadanya.
func (u *LPBUsecase) RenderPDF(ctx context.Context, actor domain.Principal, id int64) (string, []byte, error) {
	l, err := u.repo.FindByID(ctx, id)
	if err != nil {
		return "", nil, err
	}
	if !actor.CanAccessWarehouse(l.WarehouseID) {
		return "", nil, domain.ErrForbiddenWarehouse
	}
	if actor.Role == domain.RoleDriver && (l.DriverID == nil || *l.DriverID != actor.UserID) {
		return "", nil, domain.ErrForbidden
	}
	if l.Status != domain.LPBApproved {
		return "", nil, domain.ErrLPBNotApproved
	}
	if u.pdfRenderer == nil {
		return "", nil, domain.ErrPDFServiceUnavailable
	}

	items, err := u.repo.Items(ctx, l.ID)
	if err != nil {
		return "", nil, err
	}

	var qrURI template.URL
	if l.VerifyToken != nil && *l.VerifyToken != "" {
		verifyURL := fmt.Sprintf("%s/verify/lpb/%s", u.appBaseURL, *l.VerifyToken)
		uri, err := qrcode.DataURI(verifyURL, 200)
		if err != nil {
			return "", nil, fmt.Errorf("buat qr verifikasi: %w", err)
		}
		qrURI = template.URL(uri)
	}

	data := buildLPBSuratJalanData(l, items, qrURI, u.clock.Now())
	html, err := templates.RenderLPBSuratJalan(data)
	if err != nil {
		return "", nil, err
	}

	bytes, err := u.pdfRenderer.RenderHTML(ctx, html, pdf.A4Options())
	if err != nil {
		if errors.Is(err, pdf.ErrUnavailable) {
			return "", nil, domain.ErrPDFServiceUnavailable
		}
		return "", nil, err
	}

	filename := strings.NewReplacer("/", "-", " ", "_").Replace(l.Number) + ".pdf"
	return filename, bytes, nil
}

// VerifyByToken mengembalikan ringkasan publik LPB APPROVED untuk halaman verifikasi QR.
func (u *LPBUsecase) VerifyByToken(ctx context.Context, token string) (*domain.LPBVerifySummary, error) {
	token = strings.TrimSpace(token)
	if len(token) < 16 || len(token) > 128 {
		return nil, domain.ErrLPBNotFound
	}
	return u.repo.FindByVerifyToken(ctx, token)
}

func buildLPBSuratJalanData(l *domain.LPB, items []domain.LPBItem, qrURI template.URL, now time.Time) templates.LPBSuratJalanData {
	title := "LEMBAR PENERIMAAN BARANG (LPB MASUK)"
	typeLabel := "Masuk"
	if l.Type == domain.LPBOutbound {
		title = "SURAT JALAN / LPB KELUAR"
		typeLabel = "Keluar"
	}

	totalQty := 0
	tItems := make([]templates.LPBSuratJalanItem, 0, len(items))
	for i, it := range items {
		totalQty += it.Qty
		var destLabel string
		if it.DestRackCode != nil {
			if it.DestWarehouseCode != nil && *it.DestWarehouseCode != l.WarehouseCode {
				destLabel = *it.DestWarehouseCode + " / " + *it.DestRackCode
			} else {
				destLabel = *it.DestRackCode
			}
		}
		serials := make([]string, 0, len(it.Serials))
		for _, s := range it.Serials {
			if s.AssetTag != nil && *s.AssetTag != "" {
				serials = append(serials, fmt.Sprintf("%s (%s)", *s.AssetTag, s.SerialNumber))
			} else {
				serials = append(serials, s.SerialNumber)
			}
		}
		tItems = append(tItems, templates.LPBSuratJalanItem{
			No:          i + 1,
			SKU:         it.SKU,
			ProductName: it.ProductName,
			Unit:        it.Unit,
			RackCode:    it.RackCode,
			DestLabel:   destLabel,
			Qty:         it.Qty,
			Serials:     serials,
			Notes:       derefStr(it.Notes),
		})
	}

	approvedAt := "—"
	if l.ApprovedAt != nil {
		approvedAt = formatWIBDateTime(*l.ApprovedAt)
	}
	approvedBy := derefStr(l.ApprovedByName)
	if approvedBy == "" {
		approvedBy = "Kepala Gudang"
	}

	return templates.LPBSuratJalanData{
		Title:             title,
		LPBNumber:         l.Number,
		TypeLabel:         typeLabel,
		SubTypeLabel:      subTypeLabelID(l.SubType),
		DocDate:           formatIndoDate(l.DocDate),
		WarehouseCode:     l.WarehouseCode,
		WarehouseName:     l.WarehouseName,
		WarehouseAddress:  derefStr(l.WarehouseAddress),
		SupplierName:      derefStr(l.SupplierName),
		ReferenceNumber:   derefStr(l.ReferenceNumber),
		CounterpartNumber: derefStr(l.CounterpartNumber),
		Notes:             derefStr(l.Notes),
		ItemCount:         len(items),
		TotalQty:          totalQty,
		Items:             tItems,
		CreatedByName:     l.CreatedByName,
		CreatedAt:         formatWIBDateTime(l.CreatedAt),
		ApprovedByName:    approvedBy,
		ApprovedAt:        approvedAt,
		PrintedAt:         formatWIBDateTime(now),
		QRDataURI:         qrURI,
	}
}

var indoMonths = [...]string{
	"", "Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des",
}

func formatIndoDate(ymd string) string {
	t, err := time.Parse("2006-01-02", ymd)
	if err != nil {
		return ymd
	}
	return fmt.Sprintf("%02d %s %d", t.Day(), indoMonths[t.Month()], t.Year())
}

func formatWIBDateTime(t time.Time) string {
	w := t.In(clock.WIB)
	return fmt.Sprintf("%02d %s %d, %02d:%02d WIB", w.Day(), indoMonths[w.Month()], w.Year(), w.Hour(), w.Minute())
}

func subTypeLabelID(s domain.LPBSubType) string {
	switch s {
	case domain.LPBPurchase:
		return "Pembelian"
	case domain.LPBInitialStock:
		return "Stok Awal"
	case domain.LPBRentalReturn:
		return "Retur Sewa"
	case domain.LPBFailedDeliveryRet:
		return "Retur Gagal Kirim"
	case domain.LPBTransferIn:
		return "Transfer Masuk"
	case domain.LPBOpnameAdjustmentIn:
		return "Penyesuaian Opname Masuk"
	case domain.LPBRentalDelivery:
		return "Pengiriman Sewa"
	case domain.LPBConsumableDelivery:
		return "Pengiriman Consumable"
	case domain.LPBSparePartIssue:
		return "Pengeluaran Sparepart"
	case domain.LPBTransferOut:
		return "Transfer Keluar"
	case domain.LPBOpnameAdjustmentOut:
		return "Penyesuaian Opname Keluar"
	case domain.LPBDisposal:
		return "Penghapusan Unit Rusak"
	default:
		return string(s)
	}
}
