package domain

import (
	"errors"
	"time"
)

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

	// Manajemen user
	ErrEmailTaken = errors.New("Email sudah terdaftar")
	ErrLastAdmin  = errors.New("Minimal harus ada satu ADMIN aktif")

	// Autentikasi (plan Bagian 10)
	ErrInvalidCredentials     = errors.New("Email atau password salah")
	ErrInvalidToken           = errors.New("Sesi tidak valid atau sudah berakhir, silakan login kembali")
	ErrAccountDisabled        = errors.New("Akun Anda dinonaktifkan, hubungi administrator")
	ErrPasswordChangeRequired = errors.New("Anda wajib mengganti password terlebih dahulu")
	ErrTooManyRequests        = errors.New("Terlalu banyak percobaan, coba lagi nanti")

	// LPB & stok (plan Bagian 6 / 7.4)
	ErrLPBNotFound            = errors.New("LPB tidak ditemukan")
	ErrLPBNotPending          = errors.New("LPB tidak dalam status menunggu persetujuan")
	ErrLPBNotApproved         = errors.New("LPB belum disetujui; surat jalan hanya dapat dicetak untuk LPB yang sudah disetujui")
	ErrPDFServiceUnavailable  = errors.New("Layanan cetak PDF sedang tidak tersedia")
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

	// Master data (T1.2)
	ErrLocationHasStock = errors.New("Lokasi masih menyimpan stok; kosongkan terlebih dahulu")
	ErrProductInUse     = errors.New("Produk sudah memiliki stok atau transaksi, SKU dan mode serial number tidak dapat diubah")
	ErrCategoryInUse    = errors.New("Kategori sudah dipakai produk, jenis barang tidak dapat diubah")

	// Idempotency (plan Bagian 7.5)
	ErrIdempotencyKeyRequired = errors.New("Header Idempotency-Key wajib diisi untuk aksi ini")
	ErrIdempotencyInProgress  = errors.New("Permintaan dengan Idempotency-Key ini masih diproses, coba lagi sebentar lagi")
	ErrIdempotencyMismatch    = errors.New("Idempotency-Key ini sudah dipakai untuk permintaan lain")

	// Inventory (T1.4)
	ErrSerialAmbiguous = errors.New("Serial number cocok dengan lebih dari satu produk; scan asset tag unit")
)

// DuplicateError: nilai unik (kode, SKU, nama) sudah dipakai. Dipetakan ke 409 beserta field-nya
// agar form dapat menandai kolom yang bentrok. errors.Is(err, ErrConflict) bernilai true.
type DuplicateError struct {
	Field   string
	Message string // mis. "kode gudang sudah dipakai"
}

func (e *DuplicateError) Error() string { return e.Message }

func (e *DuplicateError) Is(target error) bool { return target == ErrConflict }

// RateLimitError menyertakan berapa lama klien harus menunggu (header Retry-After).
// errors.Is(err, ErrTooManyRequests) bernilai true.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string { return ErrTooManyRequests.Error() }

func (e *RateLimitError) Is(target error) bool { return target == ErrTooManyRequests }

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
