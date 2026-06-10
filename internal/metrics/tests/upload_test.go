package metrics_test

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/metrics"
)

func TestUpload_RecordCountsByStatus(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := metrics.NewUpload(reg)
	require.NoError(t, err)

	m.Record("success", 0.10)
	m.Record("success", 0.20)
	m.Record("error", 0.05)

	const expected = `
# HELP avatars_uploads_total Total avatar uploads by outcome (success|error).
# TYPE avatars_uploads_total counter
avatars_uploads_total{status="error"} 1
avatars_uploads_total{status="success"} 2
`
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(expected), "avatars_uploads_total"))
}

func TestUpload_NilReceiverIsNoOp(t *testing.T) {
	var m *metrics.Upload // never constructed
	require.NotPanics(t, func() { m.Record("success", 1) })
}
