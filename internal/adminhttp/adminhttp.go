// Package adminhttp serves the ops endpoints — /health and /metrics — that both
// the server and worker expose on their own admin port, away from public
// traffic. Keeping them off the public port means probes and Prometheus scrapes
// don't go through the Ingress.
//
// It's a plain ServeMux rather than chi + otelhttp: this traffic is operational
// and has no place in the business traces.
package adminhttp

import (
	"net/http"

	"github.com/avc-dev/go-avatar-service/internal/handlers"
)

// Router wires /health and /metrics. metricsHandler is the Prometheus handler
// for this binary's registry.
func Router(healthH *handlers.HealthHandler, metricsHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthH.Get)
	mux.Handle("GET /metrics", metricsHandler)
	return mux
}
