package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"
)

// SpanName renames the server span created by the surrounding otelhttp handler
// to use chi's matched route pattern (e.g. "GET /api/v1/avatars/{avatar_id}")
// instead of the raw URL. Without this, span names contain the concrete UUID
// from every request, exploding span-name cardinality in Jaeger and making
// per-endpoint latency aggregation impossible.
//
// The rename happens after next.ServeHTTP returns, because chi only finishes
// populating the RouteContext once routing has run. The otelhttp span is still
// open at that point (it ends after the whole inner chain returns), so the new
// name is what gets exported.
func SpanName(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		rc := chi.RouteContext(r.Context())
		if rc == nil {
			return
		}
		pattern := rc.RoutePattern()
		if pattern == "" {
			return
		}
		trace.SpanFromContext(r.Context()).SetName(r.Method + " " + pattern)
	})
}
