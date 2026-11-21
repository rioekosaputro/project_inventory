// Package password membungkus bcrypt untuk hashing & verifikasi password.
package password

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"golang.org/x/crypto/bcrypt"
)

const cost = 12

// MinLength adalah panjang minimum password yang diterima Hash.
const MinLength = 8

var ErrTooShort = fmt.Errorf("password minimal %d karakter", MinLength)

// Hash menghasilkan hash bcrypt. bcrypt menolak input > 72 byte; error itu diteruskan
// agar password panjang tidak terpotong diam-diam.
func Hash(plain string) (string, error) {
	if len(plain) < MinLength {
		return "", ErrTooShort
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return "", errors.New("password maksimal 72 byte")
		}
		return "", err
	}
	return string(b), nil
}

// Alfabet password sementara: tanpa karakter yang mudah tertukar (0/O/o, 1/l/I) karena
// akan dibacakan/diketik ulang oleh manusia. 55 karakter ≈ 5,8 bit per karakter.
const tempAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"

// Generate membuat password acak kriptografis sepanjang n karakter (min MinLength)
// dengan rejection sampling agar distribusinya tidak bias.
func Generate(n int) (string, error) {
	if n < MinLength {
		return "", ErrTooShort
	}
	out := make([]byte, n)
	max := big.NewInt(int64(len(tempAlphabet)))
	for i := range out {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("password: gagal membuat acak: %w", err)
		}
		out[i] = tempAlphabet[idx.Int64()]
	}
	return string(out), nil
}

// Verify true bila plain cocok dengan hash. Hash rusak dianggap tidak cocok.
func Verify(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
