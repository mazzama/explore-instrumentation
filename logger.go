package main

import (
	"context"
	"github.com/go-chi/chi/v5/middleware"
	"io"
	"log/slog"
	"net/http"
)

const requestIDKey string = "request_id"

type contextHandler struct {
	handler slog.Handler
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		r.AddAttrs(slog.String("request_id", requestID))
	}

	return h.handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{
		handler: h.handler.WithAttrs(attrs),
	}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{
		handler: h.handler.WithGroup(name),
	}
}

func NewLogger(writer io.Writer) *slog.Logger {
	return slog.New(&contextHandler{
		handler: slog.NewJSONHandler(writer, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}),
	})
}

func requestLogger(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Context().Value(middleware.RequestIDKey).(string)
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			customLogger := logger.With(slog.String("request_id", requestID))

			customLogger.Info("Request started", slog.String("method", r.Method), slog.String("url", r.URL.String()))

			next.ServeHTTP(w, r.WithContext(ctx))

			customLogger.Info("Request completed", slog.String("method", r.Method), slog.String("url", r.URL.String()))
		})
	}
}
