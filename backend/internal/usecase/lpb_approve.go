package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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

// approvable: jenis LPB yang efek stoknya sudah diimplementasikan. Jenis lain (DO, retur, opname, pengiriman,
// suku cadang) menyusul pada fase masing-masing.
func approvable(s domain.LPBSubType) bool {
	switch s {
	case domain.LPBPurchase, domain.LPBInitialStock, domain.LPBTransferIn, domain.LPBTransferOut, domain.LPBDisposal,
		domain.LPBRentalDelivery, domain.LPBConsumableDelivery, domain.LPBRentalReturn:
		return true
	}
	return false
}

// Approvals: antrean LPB menunggu persetujuan pada gudang pemanggil, yang tertua dulu.
func (u *LPBUsecase) Approvals(ctx context.Context, actor domain.Principal, f domain.LPBFilter, p domain.PageParams) ([]domain.LPBListItem, int64, error) {
	pending := domain.LPBPendingApproval
	f.Status = &pending
	return u.List(ctx, actor, f, p)
}

// Approve menyetujui LPB dan MENGUBAH STOK — satu-satunya tempat saldo dan status unit berubah untuk LPB
// (plan Bagian 6.2). Seluruhnya dalam satu transaksi (atomik), dengan urutan lock yang tetap:
//
//	header dokumen → gudang & rak (FOR SHARE) → saldo (urut produk, rak) → unit (urut id)
//
// Gagal di mana pun = tidak ada perubahan apa pun (saldo, unit, ledger, nomor dokumen/asset tag).
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
		for _, it := range items {
			if it.HasSerialNumber && len(it.Serials) != it.Qty {
				return fmt.Errorf("%w: %s di rak %s (ter-scan %d dari %d)",
					domain.ErrSerialCountMismatch, it.SKU, it.RackCode, len(it.Serials), it.Qty)
			}
		}

		// 3. Gudang dan rak ditahan (FOR SHARE): tidak dapat dikunci opname / dinonaktifkan selama approve.
		racks, err := u.lockLocations(ctx, l, items)
		if err != nil {
			return err
		}

		// 4. Mutasi per jenis dokumen.
		var summary []map[string]any
		extra := map[string]any{}
		switch l.SubType {
		case domain.LPBPurchase, domain.LPBInitialStock:
			summary, err = u.applyReceipt(ctx, actor, l, items, racks, now)
		case domain.LPBTransferIn:
			summary, err = u.applyTransferIn(ctx, actor, l, items)
		case domain.LPBDisposal:
			summary, err = u.applyDisposal(ctx, actor, l, items)
		case domain.LPBTransferOut:
			summary, err = u.applyTransferOut(ctx, actor, l, items, now, extra)
		case domain.LPBRentalDelivery, domain.LPBConsumableDelivery:
			summary, err = u.applyDeliveryOut(ctx, actor, l, items)
		case domain.LPBRentalReturn:
			summary, err = u.applyRentalReturn(ctx, actor, l, items, racks)
		}
		if err != nil {
			return err
		}

		// 5. Dokumen final + token verifikasi QR (acak, bukan id yang dapat ditebak).
		token, err := newVerifyToken()
		if err != nil {
			return err
		}
		if err := u.repo.MarkApproved(ctx, l.ID, actor.UserID, now, token); err != nil {
			return err
		}
		after := map[string]any{"status": string(domain.LPBApproved), "items": summary}
		for k, v := range extra {
			after[k] = v
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action: domain.AuditLPBApprove, EntityType: domain.EntityLPB, EntityID: &l.ID,
			Before: map[string]any{"status": string(domain.LPBPendingApproval)},
			After:  after,
		})
	})
	if err != nil {
		return nil, err
	}
	return u.reload(ctx, id)
}

// lockLocations menahan gudang dan rak yang dipakai dokumen (FOR SHARE, urut id) lalu memeriksanya: rak asal
// milik gudang dokumen, rak (asal dan tujuan) aktif dan tidak dikunci opname, gudang tujuan aktif, produk aktif.
// Mengembalikan rak yang dikunci menurut id.
func (u *LPBUsecase) lockLocations(ctx context.Context, l *domain.LPB, items []domain.LPBItem) (map[int64]domain.Rack, error) {
	whSet := map[int64]bool{l.WarehouseID: true}
	rackSet := map[int64]bool{}
	for _, it := range items {
		rackSet[it.RackID] = true
		if it.DestRackID != nil {
			rackSet[*it.DestRackID] = true
			if it.DestWarehouseID != nil {
				whSet[*it.DestWarehouseID] = true
			}
		}
	}
	for _, id := range sortedKeys(whSet) {
		wh, err := u.warehouses.FindByIDForShare(ctx, id)
		if err != nil {
			return nil, err
		}
		if !wh.IsActive {
			field := "items"
			if id == l.WarehouseID {
				field = "warehouse_id"
			}
			return nil, lpbInvalid([]domain.FieldError{{Field: field, Message: fmt.Sprintf("gudang %s tidak aktif", wh.Code)}})
		}
	}

	locked, err := u.racks.LockForShare(ctx, sortedKeys(rackSet))
	if err != nil {
		return nil, err
	}
	racks := make(map[int64]domain.Rack, len(locked))
	for _, r := range locked {
		racks[r.ID] = r
	}
	check := func(id int64, own bool, label string) error {
		r, ok := racks[id]
		switch {
		case !ok:
			return lpbInvalid([]domain.FieldError{{Field: "items", Message: fmt.Sprintf("rak %s tidak ditemukan", label)}})
		case r.LockedByOpnameID != nil:
			return fmt.Errorf("%w: rak %s", domain.ErrRackLocked, r.Code)
		case !r.IsActive || (own && r.WarehouseID != l.WarehouseID):
			return lpbInvalid([]domain.FieldError{{Field: "items", Message: fmt.Sprintf("rak %s tidak aktif atau bukan milik gudang LPB", r.Code)}})
		}
		return nil
	}
	for _, it := range items {
		if err := check(it.RackID, true, it.RackCode); err != nil {
			return nil, err
		}
		if it.DestRackID != nil {
			if err := check(*it.DestRackID, false, derefStr(it.DestRackCode)); err != nil {
				return nil, err
			}
			if it.HasSerialNumber && !racks[*it.DestRackID].IsSellable {
				return nil, lpbInvalid([]domain.FieldError{{Field: "items", Message: fmt.Sprintf("produk %s berserial number hanya dapat dipindah ke rak yang dapat dijual", it.SKU)}})
			}
		}
		p, err := u.products.FindByID(ctx, it.ProductID)
		if err != nil {
			return nil, err
		}
		if !p.IsActive {
			return nil, lpbInvalid([]domain.FieldError{{Field: "items", Message: fmt.Sprintf("produk %s tidak aktif", p.SKU)}})
		}
	}
	return racks, nil
}

// Reject menolak LPB yang menunggu persetujuan (alasan wajib). Tidak mengubah stok. Pembuat dapat
// merevisinya (REJECTED → DRAFT). LPB TRANSFER_IN (pasangan transfer keluar yang sudah menggerakkan stok)
// tidak dapat ditolak: koreksinya lewat selisih stock opname (fase berikutnya).
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
		if l.SubType == domain.LPBTransferIn {
			return lpbInvalid([]domain.FieldError{{
				Field:   "sub_type",
				Message: "transfer masuk tidak dapat ditolak karena barang sudah keluar dari gudang asal; setujui setelah barang tiba, atau koreksi lewat stock opname bila barang tidak tiba",
			}})
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
