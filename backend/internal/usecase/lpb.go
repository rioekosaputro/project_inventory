package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/pdf"
)

const (
	maxLPBItems     = 200
	maxItemQty      = 1_000_000
	maxItemNotesLen = 500
	maxReferenceLen = 60
	docDateLayout   = "2006-01-02"
)

// LPBUsecase: dokumen penerimaan/pengeluaran barang (plan Bagian 5.2–5.4). Tahap ini hanya
// DRAFT → PENDING_APPROVAL / CANCELLED (dan REJECTED → DRAFT). Tidak ada satu pun method di sini
// yang mengubah stok atau status unit: itu hanya terjadi saat approve (CLAUDE.md invarian #1).
type LPBUsecase struct {
	tx         domain.TxManager
	repo       domain.LPBRepository
	warehouses domain.WarehouseRepository
	racks      domain.RackRepository
	products   domain.ProductRepository
	suppliers  domain.SupplierRepository
	inventory  domain.InventoryRepository
	stock      domain.StockRepository
	settings   domain.SettingsRepository
	audit       domain.AuditRepository
	docs        domain.DocNumberGenerator
	clock       clock.Clock
	pdfRenderer pdf.Renderer
	appBaseURL  string
}

func NewLPBUsecase(
	tx domain.TxManager,
	repo domain.LPBRepository,
	warehouses domain.WarehouseRepository,
	racks domain.RackRepository,
	products domain.ProductRepository,
	suppliers domain.SupplierRepository,
	inventory domain.InventoryRepository,
	stock domain.StockRepository,
	settings domain.SettingsRepository,
	audit domain.AuditRepository,
	docs domain.DocNumberGenerator,
	clk clock.Clock,
) *LPBUsecase {
	return &LPBUsecase{
		tx: tx, repo: repo, warehouses: warehouses, racks: racks, products: products,
		suppliers: suppliers, inventory: inventory, stock: stock, settings: settings, audit: audit, docs: docs, clock: clk,
	}
}

// LPBItemInput: satu baris item pada draft. UnitCost berupa teks desimal (tanpa float).
type LPBItemInput struct {
	ProductID int64
	RackID    int64
	// DestRackID: rak tujuan, wajib untuk TRANSFER_OUT (boleh di gudang lain), terlarang untuk jenis lain.
	DestRackID *int64
	Qty        int
	UnitCost   *string
	Notes      *string
}

type CreateLPBInput struct {
	SubType         domain.LPBSubType
	WarehouseID     int64
	DocDate         *string // YYYY-MM-DD; kosong = hari ini (WIB)
	SupplierID      *int64
	ReferenceNumber *string
	Notes           *string
	Items           []LPBItemInput
}

// UpdateLPBInput: perubahan parsial (hanya saat DRAFT). Jenis dan gudang tidak dapat diubah.
// Items (bila dikirim) menggantikan seluruh daftar item; item dengan (produk, rak) sama dipertahankan
// beserta serial yang sudah di-scan.
type UpdateLPBInput struct {
	DocDate         *string
	SupplierID      Patch[int64]
	ReferenceNumber Patch[string]
	Notes           Patch[string]
	Items           *[]LPBItemInput
}

// LPBDetail: header + item (+ serial) + timeline.
type LPBDetail struct {
	LPB      domain.LPB
	Items    []domain.LPBItem
	Timeline []domain.LPBEvent
}

func lpbInvalid(errs []domain.FieldError) error {
	return &domain.ValidationError{Message: "Data LPB tidak valid", Errors: errs}
}

// canModifyLPB: draft bersifat milik pembuatnya; Kepala Gudang dan ADMIN boleh membantu.
func canModifyLPB(actor domain.Principal, l *domain.LPB) bool {
	return actor.UserID == l.CreatedBy || actor.Role == domain.RoleAdmin || actor.Role == domain.RoleHeadWarehouse
}

// ---------------------------------------------------------------- baca

func (u *LPBUsecase) List(ctx context.Context, actor domain.Principal, f domain.LPBFilter, p domain.PageParams) ([]domain.LPBListItem, int64, error) {
	if f.WarehouseID != nil && !actor.CanAccessWarehouse(*f.WarehouseID) {
		return nil, 0, domain.ErrForbiddenWarehouse
	}
	f.ScopeWarehouseID = scopeOf(actor)
	return u.repo.List(ctx, f, p)
}

func (u *LPBUsecase) Get(ctx context.Context, actor domain.Principal, id int64) (*LPBDetail, error) {
	l, err := u.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !actor.CanAccessWarehouse(l.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	return u.detail(ctx, l)
}

func (u *LPBUsecase) detail(ctx context.Context, l *domain.LPB) (*LPBDetail, error) {
	items, err := u.repo.Items(ctx, l.ID)
	if err != nil {
		return nil, err
	}
	timeline, err := u.repo.Timeline(ctx, l.ID)
	if err != nil {
		return nil, err
	}
	return &LPBDetail{LPB: *l, Items: items, Timeline: timeline}, nil
}

// ---------------------------------------------------------------- buat

func (u *LPBUsecase) Create(ctx context.Context, actor domain.Principal, in CreateLPBInput) (*LPBDetail, error) {
	if !actor.CanAccessWarehouse(in.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	now := u.clock.Now()

	var errs []domain.FieldError
	switch {
	case !in.SubType.Valid():
		errs = append(errs, domain.FieldError{Field: "sub_type", Message: "jenis LPB tidak dikenal"})
	case !in.SubType.ManuallyCreatable():
		errs = append(errs, domain.FieldError{Field: "sub_type", Message: "jenis LPB ini dibuat otomatis oleh proses lain dan tidak dapat dibuat manual"})
	}
	docDate, dateErrs := u.resolveDocDate(in.DocDate, now)
	errs = append(errs, dateErrs...)
	ref := cleanOpt(in.ReferenceNumber)
	errs = append(errs, optionalText("reference_number", ref, maxReferenceLen)...)
	notes := cleanOpt(in.Notes)
	errs = append(errs, optionalText("notes", notes, maxDescriptionLen)...)
	if in.SubType == domain.LPBPurchase && in.SupplierID == nil {
		errs = append(errs, domain.FieldError{Field: "supplier_id", Message: "wajib diisi untuk pembelian"})
	}
	if in.SubType != domain.LPBPurchase && in.SupplierID != nil {
		errs = append(errs, domain.FieldError{Field: "supplier_id", Message: "hanya untuk LPB pembelian"})
	}
	items, itemErrs := shapeItems(in.SubType, in.Items)
	errs = append(errs, itemErrs...)
	if len(errs) > 0 {
		return nil, lpbInvalid(errs)
	}

	var created *domain.LPB
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		wh, err := u.warehouses.FindByID(ctx, in.WarehouseID)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			return lpbInvalid([]domain.FieldError{{Field: "warehouse_id", Message: "gudang tidak ditemukan"}})
		case err != nil:
			return err
		case !wh.IsActive:
			return lpbInvalid([]domain.FieldError{{Field: "warehouse_id", Message: "gudang tidak aktif"}})
		}
		if errs, err := u.checkSupplier(ctx, in.SupplierID); err != nil || len(errs) > 0 {
			if err != nil {
				return err
			}
			return lpbInvalid(errs)
		}
		if errs, err := u.checkItems(ctx, in.WarehouseID, items); err != nil || len(errs) > 0 {
			if err != nil {
				return err
			}
			return lpbInvalid(errs)
		}

		prefix := "LPBI"
		if in.SubType.Type() == domain.LPBOutbound {
			prefix = "LPBO"
		}
		number, err := u.docs.Next(ctx, prefix, wh.Code, now)
		if err != nil {
			return err
		}

		l := &domain.LPB{
			Number: number, Type: in.SubType.Type(), SubType: in.SubType, WarehouseID: in.WarehouseID,
			DocDate: docDate, ReferenceType: referenceTypeFor(in.SubType, ref), ReferenceNumber: ref,
			SupplierID: in.SupplierID, Notes: notes, CreatedBy: actor.UserID,
		}
		if created, err = u.repo.Create(ctx, l); err != nil {
			return err
		}
		for i := range items {
			if _, err := u.repo.InsertItem(ctx, created.ID, &items[i]); err != nil {
				return err
			}
		}
		stored, err := u.repo.Items(ctx, created.ID)
		if err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action: domain.AuditLPBCreate, EntityType: domain.EntityLPB, EntityID: &created.ID,
			After: lpbView(created, stored),
		})
	})
	if err != nil {
		return nil, err
	}
	return u.detail(ctx, created)
}

// ---------------------------------------------------------------- ubah

func (u *LPBUsecase) Update(ctx context.Context, actor domain.Principal, id int64, in UpdateLPBInput) (*LPBDetail, error) {
	now := u.clock.Now()
	var result *domain.LPB

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		l, err := u.lockEditable(ctx, actor, id)
		if err != nil {
			return err
		}
		existing, err := u.repo.Items(ctx, l.ID)
		if err != nil {
			return err
		}

		next := *l
		var errs []domain.FieldError
		if in.DocDate != nil {
			d, e := u.resolveDocDate(in.DocDate, now)
			errs = append(errs, e...)
			next.DocDate = d
		}
		if in.SupplierID.Set {
			next.SupplierID = in.SupplierID.Value
		}
		if in.ReferenceNumber.Set {
			next.ReferenceNumber = applyPatch(l.ReferenceNumber, in.ReferenceNumber)
			next.ReferenceType = referenceTypeFor(l.SubType, next.ReferenceNumber)
		}
		next.Notes = applyPatch(l.Notes, in.Notes)
		errs = append(errs, optionalText("reference_number", next.ReferenceNumber, maxReferenceLen)...)
		errs = append(errs, optionalText("notes", next.Notes, maxDescriptionLen)...)
		if l.SubType == domain.LPBPurchase && next.SupplierID == nil {
			errs = append(errs, domain.FieldError{Field: "supplier_id", Message: "wajib diisi untuk pembelian"})
		}
		if l.SubType != domain.LPBPurchase && next.SupplierID != nil {
			errs = append(errs, domain.FieldError{Field: "supplier_id", Message: "hanya untuk LPB pembelian"})
		}
		var newItems []domain.LPBItem
		if in.Items != nil {
			var itemErrs []domain.FieldError
			newItems, itemErrs = shapeItems(l.SubType, *in.Items)
			errs = append(errs, itemErrs...)
		}
		if len(errs) > 0 {
			return lpbInvalid(errs)
		}
		if next.SupplierID != nil && (l.SupplierID == nil || *l.SupplierID != *next.SupplierID) {
			if e, err := u.checkSupplier(ctx, next.SupplierID); err != nil || len(e) > 0 {
				if err != nil {
					return err
				}
				return lpbInvalid(e)
			}
		}
		if in.Items != nil {
			if e, err := u.checkItems(ctx, l.WarehouseID, newItems); err != nil || len(e) > 0 {
				if err != nil {
					return err
				}
				return lpbInvalid(e)
			}
		}

		before, after := lpbHeaderView(l), lpbHeaderView(&next)
		headerChanged := !equalViews(before, after)
		if headerChanged {
			if err := u.repo.UpdateHeader(ctx, &next); err != nil {
				return err
			}
		}
		if in.Items != nil {
			changed, err := u.syncItems(ctx, l.ID, existing, newItems)
			if err != nil {
				return err
			}
			if changed {
				stored, err := u.repo.Items(ctx, l.ID)
				if err != nil {
					return err
				}
				before["items"], after["items"] = itemsView(existing), itemsView(stored)
			}
		}
		if b, a := diffViews(before, after); len(a) > 0 {
			if err := u.audit.Insert(ctx, domain.AuditEntry{
				Action: domain.AuditLPBUpdate, EntityType: domain.EntityLPB, EntityID: &l.ID, Before: b, After: a,
			}); err != nil {
				return err
			}
		}
		result = l
		return nil
	})
	if err != nil {
		return nil, err
	}
	fresh, err := u.repo.FindByID(ctx, result.ID)
	if err != nil {
		return nil, err
	}
	return u.detail(ctx, fresh)
}

// syncItems menyelaraskan item tersimpan dengan daftar baru. Mengembalikan true bila ada perubahan.
func (u *LPBUsecase) syncItems(ctx context.Context, lpbID int64, existing []domain.LPBItem, next []domain.LPBItem) (bool, error) {
	type key struct{ product, rack int64 }
	byKey := make(map[key]*domain.LPBItem, len(existing))
	for i := range existing {
		byKey[key{existing[i].ProductID, existing[i].RackID}] = &existing[i]
	}

	changed := false
	var errs []domain.FieldError
	kept := map[int64]bool{}
	for i := range next {
		it := next[i]
		cur, ok := byKey[key{it.ProductID, it.RackID}]
		if !ok {
			if _, err := u.repo.InsertItem(ctx, lpbID, &it); err != nil {
				return false, err
			}
			changed = true
			continue
		}
		kept[cur.ID] = true
		if len(cur.Serials) > it.Qty {
			errs = append(errs, domain.FieldError{
				Field:   fmt.Sprintf("items[%d].qty", i),
				Message: fmt.Sprintf("sudah ada %d serial number ter-scan; hapus scan yang berlebih terlebih dahulu", len(cur.Serials)),
			})
			continue
		}
		if cur.Qty != it.Qty || !sameDecimal(cur.UnitCost, it.UnitCost) || !samePtr(cur.Notes, it.Notes) || !sameID(cur.DestRackID, it.DestRackID) {
			it.ID = cur.ID
			if err := u.repo.UpdateItem(ctx, &it); err != nil {
				return false, err
			}
			changed = true
		}
	}
	if len(errs) > 0 {
		return false, lpbInvalid(errs)
	}
	for i := range existing {
		if !kept[existing[i].ID] {
			if err := u.repo.DeleteItem(ctx, existing[i].ID); err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, nil
}

// ---------------------------------------------------------------- status

// Submit: DRAFT → PENDING_APPROVAL. Validasi lengkap draft dilakukan di sini (plan Bagian 5.4);
// validasi final dengan row lock tetap saat approve.
func (u *LPBUsecase) Submit(ctx context.Context, actor domain.Principal, id int64) (*LPBDetail, error) {
	now := u.clock.Now()
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		l, err := u.lockEditable(ctx, actor, id)
		if err != nil {
			return err
		}
		items, err := u.repo.Items(ctx, l.ID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return domain.ErrLPBEmptyItems
		}
		if err := u.validateForSubmit(ctx, l, items); err != nil {
			return err
		}
		if err := u.repo.MarkSubmitted(ctx, l.ID, actor.UserID, now); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action: domain.AuditLPBSubmit, EntityType: domain.EntityLPB, EntityID: &l.ID,
			Before: map[string]any{"status": string(domain.LPBDraft)},
			After:  map[string]any{"status": string(domain.LPBPendingApproval)},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.reload(ctx, id)
}

// Cancel: DRAFT → CANCELLED. Dokumen yang sudah diajukan tidak dapat dibatalkan pembuatnya
// (ditolak/di-approve Kepala Gudang); nomor dokumen tetap terpakai.
func (u *LPBUsecase) Cancel(ctx context.Context, actor domain.Principal, id int64) (*LPBDetail, error) {
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		l, err := u.lockEditable(ctx, actor, id)
		if err != nil {
			return err
		}
		if err := u.repo.MarkCancelled(ctx, l.ID); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action: domain.AuditLPBCancel, EntityType: domain.EntityLPB, EntityID: &l.ID,
			Before: map[string]any{"status": string(domain.LPBDraft)},
			After:  map[string]any{"status": string(domain.LPBCancelled)},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.reload(ctx, id)
}

// Revise: REJECTED → DRAFT (dokumen sama; alasan penolakan tetap terlihat sampai diajukan ulang).
func (u *LPBUsecase) Revise(ctx context.Context, actor domain.Principal, id int64) (*LPBDetail, error) {
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		l, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if err := u.authorize(actor, l); err != nil {
			return err
		}
		if l.Status != domain.LPBRejected {
			return fmt.Errorf("%w: hanya LPB berstatus REJECTED yang dapat direvisi", domain.ErrInvalidStateTransition)
		}
		if err := u.repo.MarkRevised(ctx, l.ID); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action: domain.AuditLPBRevise, EntityType: domain.EntityLPB, EntityID: &l.ID,
			Before: map[string]any{"status": string(domain.LPBRejected)},
			After:  map[string]any{"status": string(domain.LPBDraft)},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.reload(ctx, id)
}

func (u *LPBUsecase) reload(ctx context.Context, id int64) (*LPBDetail, error) {
	l, err := u.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return u.detail(ctx, l)
}

// authorize: akses gudang + kepemilikan draft.
func (u *LPBUsecase) authorize(actor domain.Principal, l *domain.LPB) error {
	if !actor.CanAccessWarehouse(l.WarehouseID) {
		return domain.ErrForbiddenWarehouse
	}
	if !canModifyLPB(actor, l) {
		return fmt.Errorf("%w: draft ini milik %s", domain.ErrForbidden, l.CreatedByName)
	}
	return nil
}

// lockEditable mengunci header (FOR UPDATE), memeriksa akses, kepemilikan, dan status DRAFT.
func (u *LPBUsecase) lockEditable(ctx context.Context, actor domain.Principal, id int64) (*domain.LPB, error) {
	l, err := u.repo.FindByIDForUpdate(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := u.authorize(actor, l); err != nil {
		return nil, err
	}
	if l.Status != domain.LPBDraft {
		return nil, fmt.Errorf("%w: LPB berstatus %s, hanya DRAFT yang dapat diubah", domain.ErrInvalidStateTransition, l.Status)
	}
	return l, nil
}

// ---------------------------------------------------------------- validasi item

// shapeItems memeriksa bentuk item (tanpa database) dan menyusunnya menjadi domain.LPBItem.
func shapeItems(sub domain.LPBSubType, in []LPBItemInput) ([]domain.LPBItem, []domain.FieldError) {
	var errs []domain.FieldError
	if len(in) > maxLPBItems {
		errs = append(errs, domain.FieldError{Field: "items", Message: fmt.Sprintf("maksimal %d item per LPB", maxLPBItems)})
		return nil, errs
	}
	type key struct{ p, r int64 }
	seen := map[key]bool{}
	out := make([]domain.LPBItem, 0, len(in))
	costAllowed := sub == domain.LPBPurchase || sub == domain.LPBInitialStock

	for i, it := range in {
		f := func(field string) string { return fmt.Sprintf("items[%d].%s", i, field) }
		if it.ProductID < 1 {
			errs = append(errs, domain.FieldError{Field: f("product_id"), Message: "wajib diisi"})
		}
		if it.RackID < 1 {
			errs = append(errs, domain.FieldError{Field: f("rack_id"), Message: "wajib diisi"})
		}
		if it.Qty < 1 || it.Qty > maxItemQty {
			errs = append(errs, domain.FieldError{Field: f("qty"), Message: fmt.Sprintf("harus antara 1 dan %d", maxItemQty)})
		}
		switch {
		case sub == domain.LPBTransferOut && (it.DestRackID == nil || *it.DestRackID < 1):
			errs = append(errs, domain.FieldError{Field: f("dest_rack_id"), Message: "rak tujuan wajib diisi untuk transfer keluar"})
		case sub == domain.LPBTransferOut && *it.DestRackID == it.RackID:
			errs = append(errs, domain.FieldError{Field: f("dest_rack_id"), Message: "rak tujuan harus berbeda dari rak asal"})
		case sub != domain.LPBTransferOut && it.DestRackID != nil:
			errs = append(errs, domain.FieldError{Field: f("dest_rack_id"), Message: "hanya untuk transfer keluar"})
		}
		cost, moneyErrs := parseMoney(f("unit_cost"), it.UnitCost)
		errs = append(errs, moneyErrs...)
		if cost != nil && !costAllowed {
			errs = append(errs, domain.FieldError{Field: f("unit_cost"), Message: "hanya untuk penerimaan pembelian/stok awal"})
		}
		notes := cleanOpt(it.Notes)
		errs = append(errs, optionalText(f("notes"), notes, maxItemNotesLen)...)
		k := key{it.ProductID, it.RackID}
		if seen[k] {
			errs = append(errs, domain.FieldError{Field: fmt.Sprintf("items[%d]", i), Message: "produk dan rak yang sama muncul lebih dari sekali"})
		}
		seen[k] = true
		out = append(out, domain.LPBItem{ProductID: it.ProductID, RackID: it.RackID, DestRackID: it.DestRackID, Qty: it.Qty, UnitCost: cost, Notes: notes})
	}
	return out, errs
}

// checkItems memeriksa item terhadap data master: produk aktif, rak aktif, rak milik gudang LPB, dan
// rak tidak sedang dikunci stock opname (ErrRackLocked).
func (u *LPBUsecase) checkItems(ctx context.Context, warehouseID int64, items []domain.LPBItem) ([]domain.FieldError, error) {
	var errs []domain.FieldError
	products := map[int64]*domain.Product{}
	racks := map[int64]*domain.Rack{}

	for i, it := range items {
		f := func(field string) string { return fmt.Sprintf("items[%d].%s", i, field) }

		p, ok := products[it.ProductID]
		if !ok {
			var err error
			if p, err = u.products.FindByID(ctx, it.ProductID); err != nil && !errors.Is(err, domain.ErrNotFound) {
				return nil, err
			}
			products[it.ProductID] = p
		}
		switch {
		case p == nil:
			errs = append(errs, domain.FieldError{Field: f("product_id"), Message: "produk tidak ditemukan"})
		case !p.IsActive:
			errs = append(errs, domain.FieldError{Field: f("product_id"), Message: "produk tidak aktif"})
		}

		r, ok := racks[it.RackID]
		if !ok {
			var err error
			if r, err = u.racks.FindByID(ctx, it.RackID); err != nil && !errors.Is(err, domain.ErrNotFound) {
				return nil, err
			}
			racks[it.RackID] = r
		}
		switch {
		case r == nil:
			errs = append(errs, domain.FieldError{Field: f("rack_id"), Message: "rak tidak ditemukan"})
		case !r.IsActive:
			errs = append(errs, domain.FieldError{Field: f("rack_id"), Message: "rak tidak aktif"})
		case r.WarehouseID != warehouseID:
			errs = append(errs, domain.FieldError{Field: f("rack_id"), Message: "rak bukan milik gudang LPB ini"})
		case r.LockedByOpnameID != nil:
			return nil, fmt.Errorf("%w: rak %s", domain.ErrRackLocked, r.Code)
		}

		if it.DestRackID != nil {
			destErrs, err := u.checkDestRack(ctx, f("dest_rack_id"), *it.DestRackID, p, racks)
			if err != nil {
				return nil, err
			}
			errs = append(errs, destErrs...)
		}
	}
	// Satu dokumen transfer = satu gudang tujuan (pasangan TRANSFER_IN-nya satu dokumen).
	var destWh int64
	for i, it := range items {
		if it.DestRackID == nil {
			continue
		}
		r := racks[*it.DestRackID]
		if r == nil {
			continue
		}
		if destWh != 0 && destWh != r.WarehouseID {
			errs = append(errs, domain.FieldError{Field: fmt.Sprintf("items[%d].dest_rack_id", i), Message: "semua item transfer harus menuju satu gudang tujuan"})
		}
		destWh = r.WarehouseID
	}
	return errs, nil
}

// checkDestRack memeriksa rak tujuan transfer: ada, aktif, di gudang aktif, tidak dikunci opname, dan —
// untuk produk berserial — dapat dijual (unit tiba berstatus AVAILABLE; unit AVAILABLE di rak yang tidak
// dapat dijual akan merusak hitungan stok tersedia).
func (u *LPBUsecase) checkDestRack(ctx context.Context, field string, rackID int64, p *domain.Product, cache map[int64]*domain.Rack) ([]domain.FieldError, error) {
	r, ok := cache[rackID]
	if !ok {
		var err error
		if r, err = u.racks.FindByID(ctx, rackID); err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		cache[rackID] = r
	}
	switch {
	case r == nil:
		return []domain.FieldError{{Field: field, Message: "rak tujuan tidak ditemukan"}}, nil
	case !r.IsActive:
		return []domain.FieldError{{Field: field, Message: "rak tujuan tidak aktif"}}, nil
	case r.LockedByOpnameID != nil:
		return nil, fmt.Errorf("%w: rak tujuan %s", domain.ErrRackLocked, r.Code)
	}
	wh, err := u.warehouses.FindByID(ctx, r.WarehouseID)
	if err != nil {
		return nil, err
	}
	if !wh.IsActive {
		return []domain.FieldError{{Field: field, Message: "gudang tujuan tidak aktif"}}, nil
	}
	if p != nil && p.HasSerialNumber && !r.IsSellable {
		return []domain.FieldError{{Field: field, Message: fmt.Sprintf("produk %s berserial number hanya dapat dipindah ke rak yang dapat dijual", p.SKU)}}, nil
	}
	return nil, nil
}

func (u *LPBUsecase) checkSupplier(ctx context.Context, id *int64) ([]domain.FieldError, error) {
	if id == nil {
		return nil, nil
	}
	s, err := u.suppliers.FindByID(ctx, *id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return []domain.FieldError{{Field: "supplier_id", Message: "supplier tidak ditemukan"}}, nil
	case err != nil:
		return nil, err
	case !s.IsActive:
		return []domain.FieldError{{Field: "supplier_id", Message: "supplier tidak aktif"}}, nil
	}
	return nil, nil
}

// resolveDocDate: kosong = hari ini (WIB). Tanggal dokumen tidak boleh di masa depan.
func (u *LPBUsecase) resolveDocDate(raw *string, now time.Time) (string, []domain.FieldError) {
	today := clock.DateWIB(now)
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return today, nil
	}
	s := strings.TrimSpace(*raw)
	if _, err := time.Parse(docDateLayout, s); err != nil {
		return "", []domain.FieldError{{Field: "doc_date", Message: "format tanggal harus YYYY-MM-DD"}}
	}
	if s > today { // format YYYY-MM-DD terurut secara leksikal
		return "", []domain.FieldError{{Field: "doc_date", Message: "tanggal dokumen tidak boleh di masa depan"}}
	}
	return s, nil
}

func referenceTypeFor(sub domain.LPBSubType, number *string) *string {
	if sub == domain.LPBPurchase && number != nil {
		t := "PURCHASE_INVOICE"
		return &t
	}
	return nil
}

// ---------------------------------------------------------------- tampilan audit

func lpbHeaderView(l *domain.LPB) map[string]any {
	return map[string]any{
		"doc_date": l.DocDate, "supplier_id": ptrVal(l.SupplierID),
		"reference_number": ptrVal(l.ReferenceNumber), "notes": ptrVal(l.Notes),
	}
}

func itemsView(items []domain.LPBItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"product_id": it.ProductID, "rack_id": it.RackID, "dest_rack_id": ptrVal(it.DestRackID), "qty": it.Qty, "unit_cost": decVal(it.UnitCost),
		})
	}
	return out
}

func lpbView(l *domain.LPB, items []domain.LPBItem) map[string]any {
	v := lpbHeaderView(l)
	v["lpb_number"], v["sub_type"], v["warehouse_id"], v["status"] = l.Number, string(l.SubType), l.WarehouseID, string(l.Status)
	v["items"] = itemsView(items)
	return v
}

func equalViews(a, b map[string]any) bool {
	_, after := diffViews(a, b)
	return len(after) == 0
}

func sameDecimal(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func samePtr(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func sameID(a, b *int64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
