package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

// HTTP holds the RED (Rate, Errors, Duration) instruments for the HTTP server.
type HTTP struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewHTTP builds the HTTP RED metrics and registers them on reg.
func NewHTTP(reg prometheus.Registerer) *HTTP {
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests by method, route template and status code.",
	}, []string{"method", "route", "code"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds by method and route template.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})
	reg.MustRegister(requests, duration)
	return &HTTP{requests: requests, duration: duration}
}

// Middleware records the request count (with status code) and latency. The
// "route" label is chi's matched route TEMPLATE (e.g. "/api/v1/avatars/
// {avatar_id}"), read after the inner chain has run, so concrete UUIDs never
// become label values — the same low-cardinality discipline as the tracing
// SpanName middleware.
func (m *HTTP) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			// No template matched (404 within the group); collapse to a single
			// series rather than leaking the raw path as a label.
			route = "unmatched"
		}
		m.requests.WithLabelValues(r.Method, route, strconv.Itoa(ww.Status())).Inc()
		m.duration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}
