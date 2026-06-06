package main

import (
	"net/http"

	"github.com/avc-dev/go-avatar-service/internal/handlers"
)

// buildAdminRouter wires the worker's minimal admin surface. The worker does no
// public HTTP, but it still needs an endpoint Prometheus can scrape and an
// operator can probe — so it exposes /health here. The /metrics handler is
// mounted on this same mux in a later step.
//
// This is a plain ServeMux (not chi + otelhttp): admin traffic is operational,
// not part of any business trace, so it stays out of the trace stream by design.
func buildAdminRouter(healthH *handlers.HealthHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthH.Get)
	return mux
}
