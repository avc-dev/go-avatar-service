package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/avc-dev/go-avatar-service/internal/config"
	"github.com/avc-dev/go-avatar-service/internal/logger"
)

// sampledContext returns a context carrying a valid, sampled span context with
// the given ids — enough for SpanContextFromContext(ctx).IsValid() to be true
// without standing up a real tracer provider.
func sampledContext(t *testing.T, traceHex, spanHex string) context.Context {
	t.Helper()
	traceID, err := trace.TraceIDFromHex(traceHex)
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex(spanHex)
	require.NoError(t, err)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

func TestTraceHandler_InjectsTraceFieldsInsideSpan(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithWriter(config.LogConfig{Level: "info", Format: "json"}, &buf)

	const (
		traceHex = "0123456789abcdef0123456789abcdef"
		spanHex  = "0123456789abcdef"
	)
	ctx := sampledContext(t, traceHex, spanHex)

	log.InfoContext(ctx, "uploading avatar", "user_id", "u-1")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	assert.Equal(t, traceHex, rec["trace_id"], "log line must carry the active trace_id")
	assert.Equal(t, spanHex, rec["span_id"], "log line must carry the active span_id")
	assert.Equal(t, "u-1", rec["user_id"], "explicit attributes must be preserved")
}

func TestTraceHandler_NoTraceFieldsWithoutSpan(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithWriter(config.LogConfig{Level: "info", Format: "json"}, &buf)

	// No span in context: correlation fields must be absent, not empty strings.
	log.InfoContext(context.Background(), "no span here")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	_, hasTrace := rec["trace_id"]
	_, hasSpan := rec["span_id"]
	assert.False(t, hasTrace, "trace_id must not appear outside a span")
	assert.False(t, hasSpan, "span_id must not appear outside a span")
}

// WithAttrs must preserve trace correlation — a logger derived via With(...)
// should still stamp trace ids, which only holds if traceHandler re-wraps
// itself in WithAttrs rather than letting the embedded handler escape.
func TestTraceHandler_SurvivesWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithWriter(config.LogConfig{Level: "info", Format: "json"}, &buf).
		With("service", "test")

	ctx := sampledContext(t, "abcdefabcdefabcdefabcdefabcdefab", "abcdefabcdefabcd")
	log.InfoContext(ctx, "derived logger")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	assert.Equal(t, "abcdefabcdefabcdefabcdefabcdefab", rec["trace_id"])
	assert.Equal(t, "test", rec["service"])
}
