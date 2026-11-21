// Package reqctx membawa metadata request (request id) lewat context.Context,
// sehingga usecase/repository (mis. penulisan audit_logs.request_id) dapat
// membacanya tanpa bergantung pada Gin.
package reqctx

import "context"

type requestIDKey struct{}

// WithRequestID mengembalikan context turunan yang membawa request id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID mengembalikan request id, atau "" bila tidak ada.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
