package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"rental-inventory-api/internal/domain"
)

// AddSerialInput: satu hasil scan pada item LPB. Untuk penerimaan baru (PURCHASE/INITIAL_STOCK) berisi
// serial number pabrikan; untuk DISPOSAL boleh asset tag atau serial number unit yang sudah terdaftar.
type AddSerialInput struct {
	SerialNumber string
	Condition    string // GOOD (bawaan) | MINOR_DEFECT | BROKEN
}

// AddSerialResult: SN yang tersimpan beserta progres scan item (untuk scanner POS).
type AddSerialResult struct {
	Serial  domain.LPBItemSerial
	Scanned int
	Qty     int
}

// normalizeSerial: pangkas dan huruf besar (SN dibandingkan tanpa peduli huruf agar "abc1" dan "ABC1"
// tidak lolos sebagai dua unit). Karakter kontrol ditolak.
func normalizeSerial(raw string) (string, []domain.FieldError) {
	sn := strings.ToUpper(strings.TrimSpace(raw))
	bad := func(msg string) []domain.FieldError {
		return []domain.FieldError{{Field: "serial_number", Message: msg}}
	}
	if n := utf8.RuneCountInString(sn); n < 1 || n > maxScanCodeLen {
		return "", bad(fmt.Sprintf("wajib diisi, maksimal %d karakter", maxScanCodeLen))
	}
	for _, r := range sn {
		if unicode.IsControl(r) {
			return "", bad("mengandung karakter yang tidak valid")
		}
	}
	return sn, nil
}

// AddSerial menambah satu SN hasil scan ke item draft (validasi real-time, plan Bagian 5.4).
// Tidak mengubah stok maupun status unit.
func (u *LPBUsecase) AddSerial(ctx context.Context, actor domain.Principal, lpbID, itemID int64, in AddSerialInput) (*AddSerialResult, error) {
	cond := domain.SerialCondition(strings.ToUpper(strings.TrimSpace(in.Condition)))
	if in.Condition == "" {
		cond = domain.ConditionGood
	}
	if !cond.Valid() {
		return nil, lpbInvalid([]domain.FieldError{{Field: "condition", Message: "harus GOOD, MINOR_DEFECT, atau BROKEN"}})
	}
	sn, errs := normalizeSerial(in.SerialNumber)
	if len(errs) > 0 {
		return nil, lpbInvalid(errs)
	}

	var result *AddSerialResult
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		l, err := u.lockEditable(ctx, actor, lpbID)
		if err != nil {
			return err
		}
		item, err := u.repo.FindItem(ctx, l.ID, itemID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("%w: item LPB tidak ditemukan", domain.ErrNotFound)
			}
			return err
		}
		if !item.HasSerialNumber {
			return lpbInvalid([]domain.FieldError{{Field: "serial_number", Message: fmt.Sprintf("produk %s tidak memakai serial number", item.SKU)}})
		}
		if len(item.Serials) >= item.Qty {
			return fmt.Errorf("%w: item %s sudah memiliki %d dari %d serial number", domain.ErrSerialCountMismatch, item.SKU, len(item.Serials), item.Qty)
		}
		rack, err := u.racks.FindByID(ctx, item.RackID)
		if err != nil {
			return err
		}
		if rack.LockedByOpnameID != nil {
			return fmt.Errorf("%w: rak %s", domain.ErrRackLocked, rack.Code)
		}
		if !rack.IsActive {
			return lpbInvalid([]domain.FieldError{{Field: "rack_id", Message: "rak tidak aktif"}})
		}

		var (
			productSerialID *int64
			assetTag        *string
		)
		switch l.SubType {
		case domain.LPBPurchase, domain.LPBInitialStock:
			exists, err := u.repo.ProductSerialExists(ctx, item.ProductID, sn)
			if err != nil {
				return err
			}
			if exists {
				return fmt.Errorf("%w: %s", domain.ErrSerialAlreadyExists, sn)
			}
			if cond == domain.ConditionBroken && rack.IsSellable {
				return lpbInvalid([]domain.FieldError{{Field: "condition", Message: "unit rusak harus ditempatkan di rak yang tidak dapat dijual (mis. karantina)"}})
			}
		case domain.LPBDisposal:
			unit, err := u.resolveUnit(ctx, item.ProductID, sn)
			if err != nil {
				return err
			}
			if unit.Status != domain.SerialBroken || unit.RackID == nil || *unit.RackID != item.RackID {
				return fmt.Errorf("%w: unit %s berstatus %s dan tidak berada di rak %s sebagai BROKEN",
					domain.ErrSerialNotValid, unit.AssetTag, unit.Status, rack.Code)
			}
			sn, productSerialID, assetTag = strings.ToUpper(unit.SerialNumber), &unit.ID, &unit.AssetTag
		default:
			return lpbInvalid([]domain.FieldError{{Field: "sub_type", Message: "jenis LPB ini tidak mendukung scan serial number"}})
		}

		// Serialkan scan SN yang sama lintas LPB: pemeriksaan "dipakai LPB lain" di bawah bukan constraint DB.
		if err := u.repo.LockSerialKey(ctx, item.ProductID, sn); err != nil {
			return err
		}
		// Duplikat pada LPB yang sama (item/rak lain, produk sama) dan pada LPB lain yang masih terbuka.
		all, err := u.repo.Items(ctx, l.ID)
		if err != nil {
			return err
		}
		for _, other := range all {
			if other.ProductID != item.ProductID {
				continue
			}
			for _, s := range other.Serials {
				if strings.EqualFold(s.SerialNumber, sn) {
					return &domain.DuplicateError{Field: "serial_number", Message: "serial number sudah di-scan pada LPB ini"}
				}
			}
		}
		if number, found, err := u.repo.SerialClaimedByOtherLPB(ctx, item.ProductID, sn, productSerialID, l.ID); err != nil {
			return err
		} else if found {
			return fmt.Errorf("%w: %s (LPB %s)", domain.ErrSerialInOtherLPB, sn, number)
		}

		saved, err := u.repo.InsertItemSerial(ctx, &domain.LPBItemSerial{
			LPBItemID: item.ID, ProductSerialID: productSerialID, SerialNumber: sn, AssetTag: assetTag,
			Condition: cond, ScannedBy: &actor.UserID,
		})
		if err != nil {
			return err
		}
		result = &AddSerialResult{Serial: *saved, Scanned: len(item.Serials) + 1, Qty: item.Qty}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RemoveSerial menghapus satu SN salah scan dari item draft.
func (u *LPBUsecase) RemoveSerial(ctx context.Context, actor domain.Principal, lpbID, itemID, serialRowID int64) error {
	return u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		l, err := u.lockEditable(ctx, actor, lpbID)
		if err != nil {
			return err
		}
		if _, err := u.repo.FindItem(ctx, l.ID, itemID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("%w: item LPB tidak ditemukan", domain.ErrNotFound)
			}
			return err
		}
		if err := u.repo.DeleteItemSerial(ctx, itemID, serialRowID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("%w: serial number pada item ini tidak ditemukan", domain.ErrNotFound)
			}
			return err
		}
		return nil
	})
}

// resolveUnit mencari unit terdaftar (asset tag atau SN) milik produk tertentu.
func (u *LPBUsecase) resolveUnit(ctx context.Context, productID int64, code string) (*domain.Serial, error) {
	found, err := u.inventory.FindSerialsByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	for i := range found {
		if found[i].ProductID == productID {
			return &found[i], nil
		}
	}
	return nil, lpbInvalid([]domain.FieldError{{Field: "serial_number", Message: "unit dengan kode ini tidak ditemukan untuk produk pada item ini"}})
}

// validateForSubmit: aturan Bagian 5.4 "saat submit" + pengecekan ulang aturan scan (data dapat berubah
// sejak scan). Validasi final dengan row lock dilakukan saat approve.
func (u *LPBUsecase) validateForSubmit(ctx context.Context, l *domain.LPB, items []domain.LPBItem) error {
	checks := make([]domain.LPBItem, len(items))
	copy(checks, items)
	if errs, err := u.checkItems(ctx, l.WarehouseID, checks); err != nil {
		return err
	} else if len(errs) > 0 {
		return lpbInvalid(errs)
	}

	for _, it := range items {
		where := fmt.Sprintf("%s di rak %s", it.SKU, it.RackCode)

		if !it.HasSerialNumber {
			if l.Type == domain.LPBOutbound {
				avail, err := u.repo.AvailableQty(ctx, it.ProductID, it.RackID)
				if err != nil {
					return err
				}
				if avail < it.Qty {
					return fmt.Errorf("%w: %s (tersedia %d, diminta %d)", domain.ErrInsufficientStock, where, avail, it.Qty)
				}
			}
			continue
		}

		if len(it.Serials) != it.Qty {
			return fmt.Errorf("%w: %s (ter-scan %d dari %d)", domain.ErrSerialCountMismatch, where, len(it.Serials), it.Qty)
		}
		var ids []int64
		for _, s := range it.Serials {
			switch l.SubType {
			case domain.LPBPurchase, domain.LPBInitialStock:
				exists, err := u.repo.ProductSerialExists(ctx, it.ProductID, s.SerialNumber)
				if err != nil {
					return err
				}
				if exists {
					return fmt.Errorf("%w: %s", domain.ErrSerialAlreadyExists, s.SerialNumber)
				}
				if s.Condition == domain.ConditionBroken && it.RackIsSellable {
					return fmt.Errorf("%w: unit rusak %s harus berada di rak yang tidak dapat dijual", domain.ErrSerialNotValid, s.SerialNumber)
				}
			case domain.LPBDisposal:
				if s.ProductSerialID != nil {
					ids = append(ids, *s.ProductSerialID)
				}
			}
			if number, found, err := u.repo.SerialClaimedByOtherLPB(ctx, it.ProductID, s.SerialNumber, s.ProductSerialID, l.ID); err != nil {
				return err
			} else if found {
				return fmt.Errorf("%w: %s (LPB %s)", domain.ErrSerialInOtherLPB, s.SerialNumber, number)
			}
		}

		if l.SubType == domain.LPBDisposal {
			states, err := u.repo.SerialStates(ctx, ids)
			if err != nil {
				return err
			}
			if len(states) != len(ids) {
				return fmt.Errorf("%w: ada unit yang tidak lagi terdaftar", domain.ErrSerialNotValid)
			}
			for _, st := range states {
				if st.Status != domain.SerialBroken || st.RackID == nil || *st.RackID != it.RackID {
					return fmt.Errorf("%w: unit %s berstatus %s dan tidak lagi berada di rak %s sebagai BROKEN",
						domain.ErrSerialNotValid, st.AssetTag, st.Status, it.RackCode)
				}
			}
		}
	}
	return nil
}
