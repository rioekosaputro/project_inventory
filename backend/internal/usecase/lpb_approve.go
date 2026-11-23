package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"rental-inventory-api/internal/domain"
)

const (
	settingAllowSelfApprove = "lpb.allow_self_approve"
	maxRejectReasonLen      = 500
	minRejectReasonLen      = 3
	verifyTokenBytes        = 32
)

// approvable: jenis LPB yang efek stoknya sudah diimplementasikan. Jenis lain menyusul pada task
// masing-masing (OUTBOUND manual: T1.8; DO/retur/transfer/opname: fase berikutnya).
func approvable(s domain.LPBSubType) bool {
	return s == domain.LPBPurchase || s == domain.LPBInitialStock
}

// Approvals: antrean LPB menunggu persetujuan pada gudang pemanggil, yang tertua dulu.
func (u *LPBUsecase) Approvals(ctx context.Context, actor domain.Principal, f domain.LPBFilter, p domain.PageParams) ([]domain.LPBListItem, int64, error) {
	pending := domain.LPBPendingApproval
	f.Status = &pending
	return u.List(ctx, actor, f, p)
}

// Approve menyetujui LPB masuk dan MENGUBAH STOK — satu-satunya tempat saldo bertambah untuk
// PURCHASE/INITIAL_STOCK (plan Bagian 6.2). Seluruhnya dalam satu transaksi (atomik):
//
//	lock: header dokumen → gudang & rak (FOR SHARE) → saldo (urut produk, rak) → unit baru
//
// Gagal di mana pun = tidak ada perubahan apa pun (saldo, unit, ledger, nomor asset tag).
func (u *LPBUsecase) Approve(ctx context.Context, actor domain.Principal, id int64) (*LPBDetail, error) {
	now := u.clock.Now()

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		// 1. Header dokumen dikunci lebih dulu; semua validasi dilakukan SETELAH lock.
		l, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if l.Status != domain.LPBPendingApproval {
			return fmt.Errorf("%w: LPB berstatus %s", domain.ErrLPBNotPending, l.Status)
		}
		if !actor.CanAccessWarehouse(l.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if actor.UserID == l.CreatedBy {
			allowed, err := u.settings.Bool(ctx, settingAllowSelfApprove, false)
			if err != nil {
				return err
			}
			if !allowed {
				return domain.ErrSelfApproveForbidden
			}
		}
		if !approvable(l.SubType) {
			return lpbInvalid([]domain.FieldError{{Field: "sub_type", Message: fmt.Sprintf("persetujuan LPB %s belum tersedia", l.SubType)}})
		}

		// 2. Item (urut produk, rak = urutan lock saldo).
		items, err := u.repo.Items(ctx, l.ID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return domain.ErrLPBEmptyItems
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].ProductID != items[j].ProductID {
				return items[i].ProductID < items[j].ProductID
			}
			return items[i].RackID < items[j].RackID
		})

		// 3. Gudang dan rak ditahan (FOR SHARE): tidak dapat dikunci opname / dinonaktifkan selama approve.
		wh, err := u.warehouses.FindByIDForShare(ctx, l.WarehouseID)
		if err != nil {
			return err
		}
		if !wh.IsActive {
			return lpbInvalid([]domain.FieldError{{Field: "warehouse_id", Message: "gudang tidak aktif"}})
		}
		rackIDs := make([]int64, 0, len(items))
		seen := map[int64]bool{}
		for _, it := range items {
			if !seen[it.RackID] {
				seen[it.RackID] = true
				rackIDs = append(rackIDs, it.RackID)
			}
		}
		sort.Slice(rackIDs, func(i, j int) bool { return rackIDs[i] < rackIDs[j] })
		locked, err := u.racks.LockForShare(ctx, rackIDs)
		if err != nil {
			return err
		}
		racks := make(map[int64]domain.Rack, len(locked))
		for _, r := range locked {
			racks[r.ID] = r
		}
		for _, it := range items {
			r, ok := racks[it.RackID]
			switch {
			case !ok:
				return lpbInvalid([]domain.FieldError{{Field: "items", Message: fmt.Sprintf("rak %s tidak ditemukan", it.RackCode)}})
			case r.LockedByOpnameID != nil:
				return fmt.Errorf("%w: rak %s", domain.ErrRackLocked, r.Code)
			case !r.IsActive || r.WarehouseID != l.WarehouseID:
				return lpbInvalid([]domain.FieldError{{Field: "items", Message: fmt.Sprintf("rak %s tidak aktif atau bukan milik gudang LPB", r.Code)}})
			}
			p, err := u.products.FindByID(ctx, it.ProductID)
			if err != nil {
				return err
			}
			if !p.IsActive {
				return lpbInvalid([]domain.FieldError{{Field: "items", Message: fmt.Sprintf("produk %s tidak aktif", p.SKU)}})
			}
		}

		// 4. Mutasi per item.
		summary := make([]map[string]any, 0, len(items))
		for _, it := range items {
			if it.HasSerialNumber && len(it.Serials) != it.Qty {
				return fmt.Errorf("%w: %s di rak %s (ter-scan %d dari %d)",
					domain.ErrSerialCountMismatch, it.SKU, it.RackCode, len(it.Serials), it.Qty)
			}

			before, after, err := u.stock.AddStock(ctx, it.ProductID, it.RackID, it.Qty)
			if err != nil {
				return err
			}

			var serialIDs []int64
			if it.HasSerialNumber {
				serials := append([]domain.LPBItemSerial(nil), it.Serials...)
				sort.Slice(serials, func(i, j int) bool { return serials[i].SerialNumber < serials[j].SerialNumber })
				racksellable := racks[it.RackID].IsSellable
				for _, s := range serials {
					status := domain.SerialAvailable
					if s.Condition == domain.ConditionBroken {
						if racksellable {
							return fmt.Errorf("%w: unit rusak %s harus berada di rak yang tidak dapat dijual", domain.ErrSerialNotValid, s.SerialNumber)
						}
						status = domain.SerialBroken
					}
					tag, err := u.docs.NextAssetTag(ctx, now)
					if err != nil {
						return err
					}
					serialID, err := u.stock.InsertSerial(ctx, &domain.NewSerial{
						ProductID: it.ProductID, WarehouseID: l.WarehouseID, RackID: it.RackID,
						AssetTag: tag, SerialNumber: s.SerialNumber, Status: status, Condition: s.Condition,
						SupplierID: l.SupplierID, PurchaseDate: &l.DocDate, PurchasePrice: it.UnitCost,
					})
					if err != nil {
						if errors.Is(err, domain.ErrSerialAlreadyExists) {
							return fmt.Errorf("%w: %s", domain.ErrSerialAlreadyExists, s.SerialNumber)
						}
						return err
					}
					if err := u.repo.SetItemSerialLink(ctx, s.ID, serialID); err != nil {
						return err
					}
					refType := "LPB"
					note := "LPB " + l.Number
					if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
						SerialID: serialID, ToStatus: string(status), ToRackID: &it.RackID,
						EventType: "LPB_APPROVED", ReferenceType: &refType, ReferenceID: &l.ID,
						UserID: &actor.UserID, Notes: &note,
					}); err != nil {
						return err
					}
					serialIDs = append(serialIDs, serialID)
				}
			}

			if err := u.stock.InsertLedger(ctx, domain.LedgerEntry{
				ProductID: it.ProductID, WarehouseID: l.WarehouseID, RackID: it.RackID,
				Direction: "IN", Qty: it.Qty, Before: before, After: after, MovementType: l.SubType,
				LPBID: l.ID, LPBItemID: it.ID, SerialIDs: serialIDs, UnitCost: it.UnitCost, CreatedBy: actor.UserID,
			}); err != nil {
				return err
			}
			summary = append(summary, map[string]any{
				"product_id": it.ProductID, "rack_id": it.RackID, "qty": it.Qty,
				"balance_before": before, "balance_after": after, "serials": len(serialIDs),
			})
		}

		// 5. Dokumen final + token verifikasi QR (acak, bukan id yang dapat ditebak).
		token, err := newVerifyToken()
		if err != nil {
			return err
		}
		if err := u.repo.MarkApproved(ctx, l.ID, actor.UserID, now, token); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action: domain.AuditLPBApprove, EntityType: domain.EntityLPB, EntityID: &l.ID,
			Before: map[string]any{"status": string(domain.LPBPendingApproval)},
			After:  map[string]any{"status": string(domain.LPBApproved), "items": summary},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.reload(ctx, id)
}

// Reject menolak LPB yang menunggu persetujuan (alasan wajib). Tidak mengubah stok. Pembuat dapat
// merevisinya (REJECTED → DRAFT).
func (u *LPBUsecase) Reject(ctx context.Context, actor domain.Principal, id int64, rawReason string) (*LPBDetail, error) {
	reason := strings.TrimSpace(rawReason)
	if n := utf8.RuneCountInString(reason); n < minRejectReasonLen || n > maxRejectReasonLen {
		return nil, lpbInvalid([]domain.FieldError{{
			Field: "reason", Message: fmt.Sprintf("wajib diisi, %d–%d karakter", minRejectReasonLen, maxRejectReasonLen),
		}})
	}
	now := u.clock.Now()

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		l, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if l.Status != domain.LPBPendingApproval {
			return fmt.Errorf("%w: LPB berstatus %s", domain.ErrLPBNotPending, l.Status)
		}
		if !actor.CanAccessWarehouse(l.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if err := u.repo.MarkRejected(ctx, l.ID, actor.UserID, now, reason); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action: domain.AuditLPBReject, EntityType: domain.EntityLPB, EntityID: &l.ID,
			Before: map[string]any{"status": string(domain.LPBPendingApproval)},
			After:  map[string]any{"status": string(domain.LPBRejected), "reason": reason},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.reload(ctx, id)
}

func newVerifyToken() (string, error) {
	b := make([]byte, verifyTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gagal membuat token verifikasi: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
