package usecase

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"

	"rental-inventory-api/internal/domain"
)

// Patch adalah field PATCH yang boleh dikosongkan: Set=false → tidak dikirim (tidak diubah);
// Set=true dan Value=nil → dikirim null (dikosongkan).
type Patch[T any] struct {
	Set   bool
	Value *T
}

// Batas panjang untuk teks bebas yang di DB berupa TEXT (mencegah payload berlebihan).
const (
	maxAddressLen     = 500
	maxDescriptionLen = 1000
)

var (
	// Kode gudang/supplier: huruf besar & angka, dipisah tanda hubung (WH-SMG-01). Tanpa '/' dan spasi
	// karena kode gudang dipakai sebagai scope nomor dokumen (pkg/docnum).
	codePattern = regexp.MustCompile(`^[A-Z0-9]+(-[A-Z0-9]+)*$`)
	// Kode rak: Zona-Rak-Level (A-02-01); titik dan garis bawah diperbolehkan.
	rackCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]*$`)
	// SKU juga dipakai sebagai barcode barang non-SN.
	skuPattern   = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._/-]*$`)
	moneyPattern = regexp.MustCompile(`^\d{1,13}(\.\d{1,2})?$`) // NUMERIC(15,2), tanpa notasi ilmiah/negatif
)

func masterInvalid(entity string, errs []domain.FieldError) error {
	return &domain.ValidationError{Message: entity + " tidak valid", Errors: errs}
}

func normalizeCode(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func requiredText(field, s string, maxLen int) []domain.FieldError {
	if n := utf8.RuneCountInString(s); n < 1 || n > maxLen {
		return []domain.FieldError{{Field: field, Message: fmt.Sprintf("wajib diisi, maksimal %d karakter", maxLen)}}
	}
	return nil
}

func optionalText(field string, p *string, maxLen int) []domain.FieldError {
	if p != nil && utf8.RuneCountInString(*p) > maxLen {
		return []domain.FieldError{{Field: field, Message: fmt.Sprintf("maksimal %d karakter", maxLen)}}
	}
	return nil
}

func validateCodeFormat(field, code string, maxLen int, pattern *regexp.Regexp, hint string) []domain.FieldError {
	if code == "" || len(code) > maxLen || !pattern.MatchString(code) {
		return []domain.FieldError{{Field: field, Message: fmt.Sprintf("wajib diisi, maksimal %d karakter, %s", maxLen, hint)}}
	}
	return nil
}

// cleanOpt: trim; kosong → nil (NULL di DB).
func cleanOpt(p *string) *string { return cleanPhone(p) }

// applyPatch menerapkan Patch ke nilai nullable saat ini.
func applyPatch(cur *string, p Patch[string]) *string {
	if !p.Set {
		return cur
	}
	return cleanOpt(p.Value)
}

// parseMoney: "550000.00" / "550000" → decimal; kosong/nil → nil. Tanpa float, tanpa notasi ilmiah.
func parseMoney(field string, raw *string) (*decimal.Decimal, []domain.FieldError) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	s := strings.TrimSpace(*raw)
	if !moneyPattern.MatchString(s) {
		return nil, []domain.FieldError{{Field: field, Message: "harus berupa angka tidak negatif dengan maksimal 2 desimal, mis. 550000.00"}}
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil, []domain.FieldError{{Field: field, Message: "angka tidak valid"}}
	}
	return &d, nil
}

// scopeOf mengembalikan id gudang yang membatasi pemanggil (warehouse scoping, plan Bagian 5.1);
// nil = boleh semua gudang (ADMIN atau akun tanpa warehouse_id).
func scopeOf(actor domain.Principal) *int64 {
	if actor.Role == domain.RoleAdmin {
		return nil
	}
	return actor.WarehouseID
}

// ---------------------------------------------------------------- audit

func auditAction(entity, verb string) string { return strings.ToUpper(entity) + "_" + verb }

// strVal/ptrVal membuat nilai audit yang bisa dibandingkan: pointer nil → nil, selain itu isinya.
func ptrVal[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func decVal(p *decimal.Decimal) any {
	if p == nil {
		return nil
	}
	return p.StringFixed(2)
}

// diffViews mengembalikan hanya field yang berubah (sebelum/sesudah).
func diffViews(a, b map[string]any) (before, after map[string]any) {
	before, after = map[string]any{}, map[string]any{}
	for k, av := range a {
		if bv := b[k]; !reflect.DeepEqual(av, bv) {
			before[k], after[k] = av, bv
		}
	}
	return before, after
}

// auditCreate mencatat pembuatan entitas master beserta nilainya.
func auditCreate(ctx context.Context, audit domain.AuditRepository, entity string, id int64, view map[string]any) error {
	return audit.Insert(ctx, domain.AuditEntry{
		Action: auditAction(entity, domain.AuditVerbCreate), EntityType: entity, EntityID: &id, After: view,
	})
}

// auditUpdate mencatat perubahan (hanya field yang berubah) dan, bila status aktif berubah,
// satu entri ACTIVATE/DEACTIVATE tersendiri. Tidak menulis apa pun bila tidak ada perubahan.
func auditUpdate(ctx context.Context, audit domain.AuditRepository, entity string, id int64, before, after map[string]any) error {
	b, a := diffViews(before, after)
	if len(a) == 0 {
		return nil
	}
	if err := audit.Insert(ctx, domain.AuditEntry{
		Action: auditAction(entity, domain.AuditVerbUpdate), EntityType: entity, EntityID: &id, Before: b, After: a,
	}); err != nil {
		return err
	}
	was, _ := before["is_active"].(bool)
	now, _ := after["is_active"].(bool)
	switch {
	case was && !now:
		return audit.Insert(ctx, domain.AuditEntry{Action: auditAction(entity, domain.AuditVerbDeactivate), EntityType: entity, EntityID: &id})
	case !was && now:
		return audit.Insert(ctx, domain.AuditEntry{Action: auditAction(entity, domain.AuditVerbActivate), EntityType: entity, EntityID: &id})
	}
	return nil
}
