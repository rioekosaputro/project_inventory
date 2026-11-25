package usecase

import (
	"context"
	"errors"
	"fmt"
	"html/template"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/pdf"
	"rental-inventory-api/pkg/qrcode"
	"rental-inventory-api/templates"
)

const maxLabelsBatch = 200

func dedupePositiveIDs(raw []int64, field string) ([]int64, error) {
	if len(raw) == 0 {
		return nil, &domain.ValidationError{
			Message: "Daftar label tidak valid",
			Errors:  []domain.FieldError{{Field: field, Message: "pilih minimal 1 item untuk dicetak"}},
		}
	}
	seen := make(map[int64]bool, len(raw))
	out := make([]int64, 0, len(raw))
	for _, id := range raw {
		if id <= 0 {
			return nil, &domain.ValidationError{
				Message: "Daftar label tidak valid",
				Errors:  []domain.FieldError{{Field: field, Message: "ID harus berupa angka positif"}},
			}
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) > maxLabelsBatch {
		return nil, &domain.ValidationError{
			Message: "Daftar label tidak valid",
			Errors:  []domain.FieldError{{Field: field, Message: fmt.Sprintf("maksimal %d label per sekali cetak", maxLabelsBatch)}},
		}
	}
	return out, nil
}

// RenderLabelsPDF menghasilkan PDF label stiker kode rak (A4, grid 3 kolom) untuk daftar rack_ids.
func (u *RackUsecase) RenderLabelsPDF(ctx context.Context, actor domain.Principal, rackIDs []int64) ([]byte, error) {
	ids, err := dedupePositiveIDs(rackIDs, "rack_ids")
	if err != nil {
		return nil, err
	}
	if u.pdfRenderer == nil {
		return nil, domain.ErrPDFServiceUnavailable
	}

	racks, err := u.repo.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(racks) != len(ids) {
		return nil, fmt.Errorf("%w: sebagian rak tidak ditemukan", domain.ErrNotFound)
	}

	items := make([]templates.LabelItem, 0, len(racks))
	for _, rk := range racks {
		if !actor.CanAccessWarehouse(rk.WarehouseID) {
			return nil, domain.ErrForbiddenWarehouse
		}
		qrURI, err := qrcode.DataURI(rk.Code, 140)
		if err != nil {
			return nil, fmt.Errorf("buat qr rak %s: %w", rk.Code, err)
		}
		var meta, badge string
		if rk.Zone != nil && *rk.Zone != "" {
			meta = "Zona: " + *rk.Zone
		}
		if !rk.IsSellable {
			badge = "NON-SELLABLE"
		}
		items = append(items, templates.LabelItem{
			Kind:      "RAK GUDANG",
			Code:      rk.Code,
			Subtitle:  rk.WarehouseCode,
			Meta:      meta,
			Badge:     badge,
			QRDataURI: template.URL(qrURI),
		})
	}

	return renderLabelSheet(ctx, u.pdfRenderer, "Label Kode Rak", items)
}

// RenderSerialLabelsPDF menghasilkan PDF label stiker asset tag batch (A4, grid 3 kolom) untuk serial_ids.
func (u *InventoryUsecase) RenderSerialLabelsPDF(ctx context.Context, actor domain.Principal, serialIDs []int64) ([]byte, error) {
	ids, err := dedupePositiveIDs(serialIDs, "serial_ids")
	if err != nil {
		return nil, err
	}
	if u.pdfRenderer == nil {
		return nil, domain.ErrPDFServiceUnavailable
	}

	serials, err := u.repo.FindSerialsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(serials) != len(ids) {
		return nil, fmt.Errorf("%w: sebagian unit serial tidak ditemukan", domain.ErrNotFound)
	}

	items := make([]templates.LabelItem, 0, len(serials))
	for _, s := range serials {
		if s.WarehouseID != nil && !actor.CanAccessWarehouse(*s.WarehouseID) {
			return nil, domain.ErrForbiddenWarehouse
		}
		qrURI, err := qrcode.DataURI(s.AssetTag, 140)
		if err != nil {
			return nil, fmt.Errorf("buat qr aset %s: %w", s.AssetTag, err)
		}
		items = append(items, templates.LabelItem{
			Kind:      "ASSET TAG",
			Code:      s.AssetTag,
			Subtitle:  s.SKU + " · " + s.ProductName,
			Meta:      "SN: " + s.SerialNumber,
			QRDataURI: template.URL(qrURI),
		})
	}

	return renderLabelSheet(ctx, u.pdfRenderer, "Label Asset Tag", items)
}

func renderLabelSheet(ctx context.Context, renderer pdf.Renderer, title string, items []templates.LabelItem) ([]byte, error) {
	html, err := templates.RenderLabels(templates.LabelsData{Title: title, Labels: items})
	if err != nil {
		return nil, err
	}
	out, err := renderer.RenderHTML(ctx, html, pdf.A4Options())
	if err != nil {
		if errors.Is(err, pdf.ErrUnavailable) {
			return nil, domain.ErrPDFServiceUnavailable
		}
		return nil, err
	}
	return out, nil
}
