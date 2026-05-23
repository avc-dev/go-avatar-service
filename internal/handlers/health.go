package handlers

import (
	"context"
	"net/http"
	"sync"
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
//
// Checkers run in parallel under a shared deadline so total latency is
// bounded by the slowest component rather than the sum of all components.
// Each goroutine writes its outcome into a fixed slot of the results slice;
// no shared map mutation is needed and the WaitGroup guarantees every slot
// is populated before we serialise the response.
func (h *HealthHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	type result struct {
		name   string
		status string
	}

	results := make([]result, len(h.checkers))
	var wg sync.WaitGroup
	idx := 0
	for name, c := range h.checkers {
		wg.Add(1)
		// Capture name+c+idx in the closure args, not the loop vars, so each
		// goroutine writes to its own slot regardless of iteration order.
		go func(i int, name string, c HealthChecker) {
			defer wg.Done()
			if err := c.Check(ctx); err != nil {
				results[i] = result{name: name, status: "error: " + err.Error()}
				return
			}
			results[i] = result{name: name, status: "ok"}
		}(idx, name, c)
		idx++
	}
	wg.Wait()

	components := make(map[string]string, len(results))
	allOK := true
	for _, r := range results {
		components[r.name] = r.status
		if r.status != "ok" {
			allOK = false
		}
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
