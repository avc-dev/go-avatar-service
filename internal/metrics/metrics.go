// Package metrics defines the Prometheus instruments for the service and the
// helpers that register them.
//
// Design notes that match the rest of the codebase:
//   - No global/default registry and no promauto. Each binary builds its own
//     *prometheus.Registry and passes it in, mirroring the project's
//     no-globals, explicit-DI rule. That also keeps tests hermetic — a fresh
//     registry per test, no cross-test leakage.
//   - Recorders (Upload, Processing) are nil-safe: a nil receiver is a no-op,
//     so layers can be constructed without metrics (unit tests) without nil
//     guards at every call site.
//   - Metric names follow the spec verbatim where it pins them
//     (avatars_uploads_total, avatars_upload_duration_seconds,
//     avatars_storage_bytes) so its example dashboards/alerts work unchanged.
//
// Deliberate deviation from the spec: the example labels avatars metrics by
// user_id. That is unbounded cardinality (one time series per user), which
// bloats the TSDB — so user_id is dropped from labels here. Per-user data
// belongs in traces/logs, not metric labels.
package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// RegisterRuntime registers the standard Go runtime and process collectors,
// which back the resource-utilisation panels (goroutines, heap, GC, CPU,
// open fds) with zero custom code.
func RegisterRuntime(reg prometheus.Registerer) {
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

// RegisterPool exposes pgx pool saturation as gauges, labelled by role
// ("server"/"worker") so one dashboard can show both processes' pools.
// pool.Stat() is an in-memory snapshot, so sampling it at scrape time via
// GaugeFunc is cheap and cannot block the scrape.
func RegisterPool(reg prometheus.Registerer, pool *pgxpool.Pool, role string) {
	labels := prometheus.Labels{"role": role}
	gauge := func(name, help string, read func(*pgxpool.Stat) float64) prometheus.Collector {
		return prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		}, func() float64 { return read(pool.Stat()) })
	}
	reg.MustRegister(
		gauge("db_pool_total_conns", "Total connections currently in the pgx pool.",
			func(s *pgxpool.Stat) float64 { return float64(s.TotalConns()) }),
		gauge("db_pool_acquired_conns", "Connections currently checked out of the pgx pool.",
			func(s *pgxpool.Stat) float64 { return float64(s.AcquiredConns()) }),
		gauge("db_pool_idle_conns", "Idle connections in the pgx pool.",
			func(s *pgxpool.Stat) float64 { return float64(s.IdleConns()) }),
		gauge("db_pool_max_conns", "Configured maximum pool size.",
			func(s *pgxpool.Stat) float64 { return float64(s.MaxConns()) }),
	)
}
