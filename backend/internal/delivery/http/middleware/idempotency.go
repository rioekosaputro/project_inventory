package middleware

import (
	"bytes"
	"context"
	"log/slog"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/domain"
)

const (
	// HeaderIdempotencyKey: kunci dibuat klien (mis. UUID) satu kali per aksi pengguna.
	HeaderIdempotencyKey = "Idempotency-Key"
	// HeaderIdempotencyReplayed menandai respons ulang dari permintaan sebelumnya.
	HeaderIdempotencyReplayed = "Idempotency-Replayed"
)

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,100}$`)

// captureWriter menyalin isi respons agar dapat disimpan.
type captureWriter struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (w *captureWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *captureWriter) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// Idempotency mewajibkan header Idempotency-Key pada endpoint kritis (approve LPB, POD, pembayaran)
// dan menjamin aksinya terjadi paling banyak sekali per (kunci, pengguna), plan Bagian 7.5:
//
//   - permintaan pertama diproses dan respons finalnya disimpan (2xx/4xx);
//   - permintaan ulang dengan kunci yang sama mengembalikan respons yang SAMA tanpa menjalankan
//     ulang aksinya (klik ganda / retry jaringan) + header Idempotency-Replayed: true;
//   - permintaan ulang saat yang pertama masih berjalan → 409 (bukan dijalankan paralel);
//   - kunci yang sama untuk endpoint lain → 422;
//   - respons 5xx/panik tidak disimpan: kuncinya dibebaskan agar klien dapat mengulang.
//
// Pasang SETELAH Authenticate dan RequirePermission (kunci hanya dikonsumsi pemanggil yang berhak).
// Penyimpanan di database (bukan memori proses) sehingga berlaku lintas instance/serverless.
func Idempotency(repo domain.IdempotencyRepository, ttl time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader(HeaderIdempotencyKey)
		if !idempotencyKeyPattern.MatchString(key) {
			httperr.Write(c, domain.ErrIdempotencyKeyRequired)
			return
		}
		p, ok := PrincipalFrom(c)
		if !ok {
			httperr.Write(c, domain.ErrUnauthorized)
			return
		}
		endpoint := c.Request.Method + " " + c.Request.URL.Path

		started, rec, err := repo.Begin(c.Request.Context(), key, p.UserID, endpoint, ttl)
		if err != nil {
			httperr.Write(c, err)
			return
		}
		if !started {
			switch {
			case rec.Endpoint != endpoint:
				httperr.Write(c, domain.ErrIdempotencyMismatch)
			case rec.StatusCode == nil:
				httperr.Write(c, domain.ErrIdempotencyInProgress)
			default:
				c.Header(HeaderIdempotencyReplayed, "true")
				c.Data(*rec.StatusCode, "application/json; charset=utf-8", rec.Body)
				c.Abort()
			}
			return
		}

		// Konteks tanpa pembatalan: penyimpanan hasil harus selesai walau klien memutus koneksi.
		persist := context.WithoutCancel(c.Request.Context())
		w := &captureWriter{ResponseWriter: c.Writer}
		c.Writer = w
		defer func() {
			if r := recover(); r != nil {
				_ = repo.Abort(persist, key, p.UserID)
				panic(r)
			}
		}()

		c.Next()

		status := w.Status()
		var perr error
		if status >= 500 {
			perr = repo.Abort(persist, key, p.UserID)
		} else {
			perr = repo.Complete(persist, key, p.UserID, status, w.buf.Bytes())
		}
		if perr != nil {
			slog.ErrorContext(persist, "idempotency: gagal menyimpan hasil", "error", perr, "endpoint", endpoint)
		}
	}
}
