package domain

import "errors"

// Sentinel error domain. Teks pesan DITAMPILKAN ke pengguna (Bahasa Indonesia),
// jadi huruf awal kapital dan tanpa detail internal. Usecase boleh membungkus dengan
// fmt.Errorf("%w: detail", ErrX) — pesan lengkapnya ikut dikirim ke klien, jadi
// jangan menyisipkan data sensitif/teknis. Pemetaan ke HTTP ada di delivery/http/httperr.
var (
	// Umum
	ErrNotFound     = errors.New("Data tidak ditemukan")
	ErrUnauthorized = errors.New("Autentikasi diperlukan")
	ErrForbidden    = errors.New("Anda tidak memiliki akses untuk aksi ini")
	ErrConflict     = errors.New("Data bertentangan dengan data yang sudah ada")

	// LPB & stok (plan Bagian 6 / 7.4)
	ErrLPBNotFound            = errors.New("LPB tidak ditemukan")
	ErrLPBNotPending          = errors.New("LPB tidak dalam status menunggu persetujuan")
	ErrInsufficientStock      = errors.New("Stok di rak tidak mencukupi")
	ErrSerialAlreadyExists    = errors.New("Serial number sudah terdaftar")
	ErrRackLocked             = errors.New("Rak sedang dikunci oleh stock opname")
	ErrReservationMismatch    = errors.New("Reservasi tidak sesuai dengan dokumen")
	ErrSerialInOtherLPB       = errors.New("Serial number sudah dipakai di LPB lain yang belum selesai")
	ErrSerialCountMismatch    = errors.New("Jumlah serial number tidak sama dengan qty")
	ErrSerialNotValid         = errors.New("Status serial number tidak valid untuk transaksi ini")
	ErrLPBEmptyItems          = errors.New("LPB tidak memiliki item")
	ErrSelfApproveForbidden   = errors.New("Pembuat LPB tidak boleh menyetujui LPB sendiri")
	ErrForbiddenWarehouse     = errors.New("Anda tidak memiliki akses ke gudang ini")
	ErrInvalidStateTransition = errors.New("Perubahan status tidak diizinkan")
)

// FieldError adalah detail kesalahan satu field.
type FieldError struct {
	Field   string
	Message string
}

// ValidationError dikembalikan usecase untuk pelanggaran aturan bisnis yang
// bisa dijelaskan per field (dipetakan ke 422).
type ValidationError struct {
	Message string
	Errors  []FieldError
}

func (e *ValidationError) Error() string { return e.Message }
