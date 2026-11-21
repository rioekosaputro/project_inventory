// Package password membungkus bcrypt untuk hashing & verifikasi password.
package password

import (
	"errors"
	"fmt"

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

// Verify true bila plain cocok dengan hash. Hash rusak dianggap tidak cocok.
func Verify(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
