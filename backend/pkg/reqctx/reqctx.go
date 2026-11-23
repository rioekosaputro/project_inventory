// Package reqctx membawa metadata request (request id, IP & User-Agent klien) lewat
// context.Context, sehingga usecase/repository (mis. penulisan audit_logs) dapat
// membacanya tanpa bergantung pada Gin.
package reqctx

import "context"

type (
	requestIDKey struct{}
	clientKey    struct{}
	userIDKey    struct{}
)

// WithRequestID mengembalikan context turunan yang membawa request id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID mengembalikan request id, atau "" bila tidak ada.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// Client adalah identitas jaringan pemanggil.
type Client struct {
	IP        string
	UserAgent string
}

// WithClient mengembalikan context turunan yang membawa IP & User-Agent klien.
func WithClient(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, clientKey{}, c)
}

// ClientFrom mengembalikan Client dari context (zero value bila tidak ada).
func ClientFrom(ctx context.Context) Client {
	c, _ := ctx.Value(clientKey{}).(Client)
	return c
}

// WithUserID menandai pengguna yang sedang terautentikasi pada context. Dipasang middleware
// Authenticate; AuditRepo memakainya sebagai pelaku bila AuditEntry.UserID kosong, sehingga setiap
// perubahan data tercatat siapa yang melakukannya tanpa tiap usecase harus mengingat mengisinya.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

// UserIDFrom mengembalikan id pengguna terautentikasi dari context.
func UserIDFrom(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey{}).(int64)
	return id, ok
}
