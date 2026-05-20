package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
)

// HealthChecker is the contract for a single component's liveness probe.
// Implementations may be method values on adapters (e.g. (*MinIO).Check) or
// inline closures wrapped in HealthCheckerFunc.
type HealthChecker interface {
	Check(ctx context.Context) error
}

// HealthCheckerFunc adapts a plain function to the HealthChecker interface,
// mirroring the http.HandlerFunc pattern. Useful when the check is a one-liner
// like (*pgxpool.Pool).Ping that doesn't deserve its own wrapper type.
type HealthCheckerFunc func(ctx context.Context) error

// Check implements HealthChecker.
func (f HealthCheckerFunc) Check(ctx context.Context) error { return f(ctx) }

// HealthHandler aggregates per-component checks into the /health endpoint.
// Components are probed in parallel-friendly order via the same context;
// each result is reported in the JSON body. The HTTP status is 200 if all
// components pass and 503 if any fails — the contract readiness probes
// (and most load balancers) expect.
type HealthHandler struct {
	checkers map[string]HealthChecker
	timeout  time.Duration
}

// NewHealthHandler constructs a HealthHandler. timeout caps how long the
// whole health check (across all components) may take; individual checkers
// share the same deadline.
func NewHealthHandler(checkers map[string]HealthChecker, timeout time.Duration) *HealthHandler {
	return &HealthHandler{
		checkers: checkers,
		timeout:  timeout,
	}
}

// healthResponse is the wire shape of a /health body.
type healthResponse struct {
	Status     string            `json:"status"`
	Components map[string]string `json:"components"`
}

// Get implements GET /health.
func (h *HealthHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	components := make(map[string]string, len(h.checkers))
	allOK := true
	for name, c := range h.checkers {
		if err := c.Check(ctx); err != nil {
			components[name] = "error: " + err.Error()
			allOK = false
			continue
		}
		components[name] = "ok"
	}

	status := http.StatusOK
	overall := "ok"
	if !allOK {
		status = http.StatusServiceUnavailable
		overall = "degraded"
	}

	httpx.WriteJSON(w, status, healthResponse{
		Status:     overall,
		Components: components,
	})
}
