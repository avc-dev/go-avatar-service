// Package middleware contains project-specific HTTP middleware that complements
// the generic helpers from go-chi/chi/middleware. Currently:
//
//   - Logger: structured per-request logs via log/slog, correlated with the
//     chi-generated request id.
//   - UserID: validation and context-injection of the X-User-ID header.
//
// Each middleware is exposed as a constructor returning the standard
// `func(http.Handler) http.Handler` so it composes cleanly with chi.Use.
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// Logger returns middleware that emits one structured log per request. It
// relies on chi's RequestID middleware being installed earlier in the chain
// so a request_id field can be attached.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				log.LogAttrs(r.Context(), slog.LevelInfo, "http request",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", ww.Status()),
					slog.Int("bytes", ww.BytesWritten()),
					slog.Duration("duration", time.Since(start)),
					slog.String("request_id", chimw.GetReqID(r.Context())),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
