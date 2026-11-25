// Package qrcode menghasilkan gambar QR code (PNG / data URI) untuk verifikasi dokumen.
package qrcode

import (
	"encoding/base64"
	"errors"
	"fmt"

	goqr "github.com/skip2/go-qrcode"
)

const defaultSize = 200

// DataURI menghasilkan PNG QR code dalam format data URI ("data:image/png;base64,...")
// agar dapat disematkan langsung ke HTML template tanpa permintaan jaringan tambahan.
func DataURI(content string, size int) (string, error) {
	if content == "" {
		return "", errors.New("qrcode: konten kosong")
	}
	if size <= 0 {
		size = defaultSize
	}
	png, err := goqr.Encode(content, goqr.Medium, size)
	if err != nil {
		return "", fmt.Errorf("qrcode: encode gagal: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}
