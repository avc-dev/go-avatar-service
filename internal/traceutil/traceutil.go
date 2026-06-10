// Package traceutil holds small helpers shared across the instrumented layers
// (storage, broker, ...). Keeping them here means a change to span-error
// handling lives in one place instead of being copy-pasted per package.
package traceutil

import (
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// FailSpan marks span as errored and returns err unchanged, for ergonomic
// `return traceutil.FailSpan(span, fmt.Errorf(...))` at error sites.
func FailSpan(span trace.Span, err error) error {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return err
}
