package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Processing holds the worker's async-processing metrics. Like Upload, a nil
// *Processing is a no-op so the worker can run without metrics in tests.
type Processing struct {
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewProcessing builds the worker processing metrics and registers them on reg.
func NewProcessing(reg prometheus.Registerer) (*Processing, error) {
	total := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "avatars_processing_total",
		Help: "Total avatar events processed by the worker, by event and outcome.",
	}, []string{"event", "status"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "avatars_processing_duration_seconds",
		Help:    "Worker processing duration in seconds by event type.",
		Buckets: prometheus.DefBuckets,
	}, []string{"event"})
	if err := register(reg, total, duration); err != nil {
		return nil, fmt.Errorf("register processing metrics: %w", err)
	}
	return &Processing{total: total, duration: duration}, nil
}

// Record observes one processed message. event is the event type ("uploaded"|
// "deleted"); status is the outcome ("success"|"failed"|"skipped"). Safe on a
// nil receiver.
func (m *Processing) Record(event, status string, seconds float64) {
	if m == nil {
		return
	}
	m.total.WithLabelValues(event, status).Inc()
	m.duration.WithLabelValues(event).Observe(seconds)
}
