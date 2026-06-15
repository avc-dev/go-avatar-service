package metrics_test

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/metrics"
)

func TestProcessing_RecordCountsByEventAndStatus(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := metrics.NewProcessing(reg)
	require.NoError(t, err)

	m.Record("uploaded", "success", 0.3)
	m.Record("uploaded", "skipped", 0.001)
	m.Record("deleted", "failed", 0.2)

	const expected = `
# HELP avatars_processing_total Total avatar events processed by the worker, by event and outcome.
# TYPE avatars_processing_total counter
avatars_processing_total{event="deleted",status="failed"} 1
avatars_processing_total{event="uploaded",status="skipped"} 1
avatars_processing_total{event="uploaded",status="success"} 1
`
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(expected), "avatars_processing_total"))
}

func TestProcessing_NilReceiverIsNoOp(t *testing.T) {
	var m *metrics.Processing
	require.NotPanics(t, func() { m.Record("uploaded", "success", 1) })
}
