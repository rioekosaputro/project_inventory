// Package reqctx membawa metadata request (request id, IP & User-Agent klien) lewat
// context.Context, sehingga usecase/repository (mis. penulisan audit_logs) dapat
// membacanya tanpa bergantung pada Gin.
package reqctx

import "context"

type (
	requestIDKey struct{}
	clientKey    struct{}
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
