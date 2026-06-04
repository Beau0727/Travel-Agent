package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

type requestIDKey struct{}

var requestCounter uint64

func Configure() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
}

func NewRequestID() string {
	seq := atomic.AddUint64(&requestCounter, 1)
	return fmt.Sprintf("req_%d_%06d", time.Now().UnixNano(), seq)
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func RequestIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

func Info(ctx context.Context, msg string, args ...any) {
	ctx = normalizeContext(ctx)
	slog.InfoContext(ctx, msg, withRequestID(ctx, args...)...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	ctx = normalizeContext(ctx)
	slog.WarnContext(ctx, msg, withRequestID(ctx, args...)...)
}

func Error(ctx context.Context, msg string, args ...any) {
	ctx = normalizeContext(ctx)
	slog.ErrorContext(ctx, msg, withRequestID(ctx, args...)...)
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func withRequestID(ctx context.Context, args ...any) []any {
	requestID := RequestIDFrom(ctx)
	if requestID == "" {
		return args
	}
	withID := make([]any, 0, len(args)+2)
	withID = append(withID, "request_id", requestID)
	withID = append(withID, args...)
	return withID
}
