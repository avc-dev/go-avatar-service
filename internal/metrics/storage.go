package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Storage holds the storage-usage business gauge. It is fed by a periodic
// sampler (see cmd/server) rather than a scrape-time DB query, so a slow
// database can never stall a Prometheus scrape.
type Storage struct {
	bytes prometheus.Gauge
}

// NewStorage builds the storage-usage gauge and registers it on reg.
func NewStorage(reg prometheus.Registerer) (*Storage, error) {
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "avatars_storage_bytes",
		Help: "Total bytes of stored (non-deleted) avatar originals.",
	})
	if err := register(reg, g); err != nil {
		return nil, fmt.Errorf("register storage metric: %w", err)
	}
	return &Storage{bytes: g}, nil
}

// Set publishes the latest total. Safe on a nil receiver.
func (m *Storage) Set(totalBytes int64) {
	if m == nil {
		return
	}
	m.bytes.Set(float64(totalBytes))
}
