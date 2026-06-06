package logger

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// traceHandler decorates another slog.Handler, stamping every record that is
// emitted within an active span with the trace_id and span_id pulled from the
// context. This is what makes "saw a spike on the dashboard -> jump to the
// exact trace in Jaeger" work: a log line and its span share the same trace_id.
//
// Doing it in a handler (rather than slog.With(...) at each call site, as the
// naive approach would) means every existing log call gets correlation for
// free, with no churn in the call sites and no risk of someone forgetting it.
type traceHandler struct {
	inner slog.Handler
}

// withTrace wraps inner so emitted records carry trace correlation fields.
func withTrace(inner slog.Handler) slog.Handler {
	return traceHandler{inner: inner}
}

// Enabled defers entirely to the wrapped handler's level filtering.
func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

func (h traceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// WithAttrs and WithGroup must re-wrap the derived handler — otherwise the
// default embedding would unwrap us and a logger built via slog.With(...) would
// silently lose trace correlation.
func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{inner: h.inner.WithGroup(name)}
}
