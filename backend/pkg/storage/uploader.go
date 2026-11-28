// Package storage mengabstraksikan penyimpanan file privat (foto POD, foto tiket, dokumen)
// dengan driver S3-compatible (Supabase Storage, MinIO, AWS S3) dan disk lokal (development/VPS).
//
// Aturan keamanan (plan Bagian 10):
//   - Ukuran maksimum 5 MB (MaxUploadBytes).
//   - Validasi MIME dari isi file (http.DetectContentType), bukan dari ekstensi/header klien.
//   - Hanya image/jpeg, image/png, image/webp, application/pdf yang diizinkan.
//   - Nama file selalu di-generate server (UUID v4) di bawah folder tervalidasi.
//   - Akses baca hanya melalui signed URL berumur pendek.
package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	// MaxUploadBytes adalah batas ukuran file unggahan (5 MB, plan Bagian 10).
	MaxUploadBytes int64 = 5 << 20
	// DefaultSignedURLTTL adalah masa berlaku bawaan signed URL baca/unggah.
	DefaultSignedURLTTL = 15 * time.Minute
)

var (
	// ErrFileTooLarge dikembalikan jika ukuran berkas melebihi MaxUploadBytes.
	ErrFileTooLarge = errors.New("ukuran file melebihi batas maksimal 5 MB")
	// ErrEmptyFile dikembalikan jika berkas kosong (0 byte).
	ErrEmptyFile = errors.New("file kosong tidak dapat diunggah")
	// ErrInvalidMimeType dikembalikan jika MIME hasil deteksi isi tidak termasuk whitelist.
	ErrInvalidMimeType = errors.New("tipe file tidak diizinkan (hanya JPEG, PNG, WebP, atau PDF)")
	// ErrInvalidKey dikembalikan jika key/folder tidak valid atau mengandung path traversal.
	ErrInvalidKey = errors.New("kunci penyimpanan file tidak valid")
	// ErrNotFound dikembalikan jika objek tidak ditemukan.
	ErrNotFound = errors.New("file tidak ditemukan")
	// ErrInvalidSignature dikembalikan jika tanda tangan signed URL tidak cocok atau kedaluwarsa.
	ErrInvalidSignature = errors.New("tautan file tidak valid atau sudah kedaluwarsa")
)

// allowedMIMETypes memetakan MIME hasil sniff ke ekstensi file kanonik.
var allowedMIMETypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"application/pdf": ".pdf",
}

// UploadResult adalah metadata berkas yang berhasil disimpan.
type UploadResult struct {
	Key         string
	ContentType string
	Size        int64
}

// Uploader adalah kontrak penyimpanan file privat lintas driver (S3 / local).
type Uploader interface {
	// Upload memvalidasi ukuran & MIME isi berkas, men-generate key UUID unik di bawah folder,
	// lalu menyimpannya ke storage privat.
	Upload(ctx context.Context, folder string, r io.Reader) (*UploadResult, error)
	// Delete menghapus objek berdasarkan key. Bersifat idempoten (tidak error bila sudah tidak ada).
	Delete(ctx context.Context, key string) error
	// SignedURL membuat URL baca berumur pendek untuk mengakses objek privat.
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	// SignedUploadURL membuat URL unggah berumur pendek untuk unggahan langsung berukuran besar.
	SignedUploadURL(ctx context.Context, key string, contentType string, ttl time.Duration) (string, error)
}

// preparedUpload hasil pembacaan & validasi berkas sebelum ditulis ke backend storage.
type preparedUpload struct {
	key         string
	contentType string
	data        []byte
}

// prepareUpload membaca maksimal MaxUploadBytes+1 byte, mendeteksi MIME dari magic bytes,
// memvalidasi whitelist, dan membangun key unik `<folder>/<uuid><ext>`.
func prepareUpload(folder string, r io.Reader) (*preparedUpload, error) {
	if r == nil {
		return nil, ErrEmptyFile
	}
	cleanFolder, err := sanitizeFolder(folder)
	if err != nil {
		return nil, err
	}

	// Baca maksimal MaxUploadBytes + 1 byte agar file melebihi 5 MB langsung ditolak tanpa OOM.
	limited := io.LimitReader(r, MaxUploadBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("storage: baca berkas: %w", err)
	}
	if len(data) == 0 {
		return nil, ErrEmptyFile
	}
	if int64(len(data)) > MaxUploadBytes {
		return nil, ErrFileTooLarge
	}

	mimeType := detectMIME(data)
	ext, ok := allowedMIMETypes[mimeType]
	if !ok {
		return nil, fmt.Errorf("%w: terdeteksi %s", ErrInvalidMimeType, mimeType)
	}

	id, err := newUUIDv4()
	if err != nil {
		return nil, fmt.Errorf("storage: generate uuid: %w", err)
	}

	key := id + ext
	if cleanFolder != "" {
		key = cleanFolder + "/" + key
	}

	return &preparedUpload{
		key:         key,
		contentType: mimeType,
		data:        data,
	}, nil
}

// detectMIME mendeteksi tipe MIME dari isi file menggunakan http.DetectContentType
// (standar library Go, mencakup JPEG, PNG, WebP, dan PDF).
func detectMIME(data []byte) string {
	sniffLen := len(data)
	if sniffLen > 512 {
		sniffLen = 512
	}
	ct := http.DetectContentType(data[:sniffLen])
	// Ambil tipe utama tanpa parameter (mis. "; charset=utf-8").
	if idx := strings.IndexByte(ct, ';'); idx >= 0 {
		ct = ct[:idx]
	}
	ct = strings.TrimSpace(strings.ToLower(ct))
	// Fallback tambahan: pastikan header WebP (RIFF....WEBP) dikenali walau < 12 byte dicek ketat.
	if ct == "application/octet-stream" && len(data) >= 12 &&
		bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return "image/webp"
	}
	return ct
}

// sanitizeFolder membersihkan nama folder dan menolak path traversal.
func sanitizeFolder(folder string) (string, error) {
	f := strings.TrimSpace(strings.ReplaceAll(folder, "\\", "/"))
	f = strings.Trim(f, "/")
	if f == "" {
		return "", nil
	}
	if strings.Contains(f, "..") || strings.ContainsAny(f, "\x00") {
		return "", fmt.Errorf("%w: folder %q", ErrInvalidKey, folder)
	}
	cleaned := path.Clean(f)
	if cleaned == "." || cleaned == "/" || strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("%w: folder %q", ErrInvalidKey, folder)
	}
	return cleaned, nil
}

// sanitizeKey memvalidasi key objek agar bersih, relatif, dan bebas path traversal.
func sanitizeKey(key string) (string, error) {
	k := strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	k = strings.TrimPrefix(k, "/")
	if k == "" || strings.Contains(k, "..") || strings.ContainsAny(k, "\x00") {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	cleaned := path.Clean(k)
	if cleaned == "." || cleaned == "/" || strings.HasPrefix(cleaned, "..") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return cleaned, nil
}

func normalizeTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return DefaultSignedURLTTL
	}
	return ttl
}

// newUUIDv4 menghasilkan UUID v4 acak (RFC 4122) tanpa dependensi eksternal.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
