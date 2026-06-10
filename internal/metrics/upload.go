package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Upload holds the business metrics for the avatar upload operation. A nil
// *Upload is a valid no-op recorder, so the HTTP handler can be constructed
// without metrics (e.g. in unit tests) and still call Record safely.
type Upload struct {
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewUpload builds the upload business metrics and registers them on reg.
func NewUpload(reg prometheus.Registerer) (*Upload, error) {
	total := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "avatars_uploads_total",
		Help: "Total avatar uploads by outcome (success|error).",
	}, []string{"status"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "avatars_upload_duration_seconds",
		Help:    "Avatar upload handling duration in seconds by outcome.",
		Buckets: prometheus.DefBuckets,
	}, []string{"status"})
	if err := register(reg, total, duration); err != nil {
		return nil, fmt.Errorf("register upload metrics: %w", err)
	}
	return &Upload{total: total, duration: duration}, nil
}

// Record observes one completed upload attempt. Safe on a nil receiver.
func (m *Upload) Record(status string, seconds float64) {
	if m == nil {
		return
	}
	m.total.WithLabelValues(status).Inc()
	m.duration.WithLabelValues(status).Observe(seconds)
}
