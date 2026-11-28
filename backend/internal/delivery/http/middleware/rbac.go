package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/domain"
)

// RequirePermission menolak (403) pemanggil yang role-nya tidak punya permission.
// Harus dipasang SETELAH Authenticate. Kunci permission yang tidak terdaftar di matriks
// membuat aplikasi panic saat route didaftarkan (salah ketik tertangkap saat startup,
// bukan menjadi route yang diam-diam terbuka/tertutup).
//
// Ini baru lapisan pertama: cek kepemilikan data & warehouse scope tetap di usecase.
func RequirePermission(perm domain.Permission) gin.HandlerFunc {
	if !domain.IsKnownPermission(perm) {
		panic(fmt.Sprintf("middleware: permission tidak dikenal %q (periksa internal/domain/permission.go)", perm))
	}
	return func(c *gin.Context) {
		p, ok := PrincipalFrom(c)
		if !ok { // RequirePermission dipasang tanpa Authenticate: gagal tertutup
			httperr.Write(c, domain.ErrUnauthorized)
			return
		}
		if !domain.Can(p.Role, perm) {
			httperr.Write(c, domain.ErrForbidden)
			return
		}
		c.Next()
	}
}

// RequireAnyPermission mengizinkan pemanggil jika memiliki minimal satu dari daftar permission.
func RequireAnyPermission(perms ...domain.Permission) gin.HandlerFunc {
	for _, perm := range perms {
		if !domain.IsKnownPermission(perm) {
			panic(fmt.Sprintf("middleware: permission tidak dikenal %q", perm))
		}
	}
	return func(c *gin.Context) {
		p, ok := PrincipalFrom(c)
		if !ok {
			httperr.Write(c, domain.ErrUnauthorized)
			return
		}
		for _, perm := range perms {
			if domain.Can(p.Role, perm) {
				c.Next()
				return
			}
		}
		httperr.Write(c, domain.ErrForbidden)
	}
}
