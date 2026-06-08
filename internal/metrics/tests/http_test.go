package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/metrics"
)

// The "route" label must be chi's template, not the concrete path — otherwise
// every distinct avatar id would mint a new time series. This test drives two
// different ids through the same templated route and asserts they collapse into
// a single series labelled with the template.
func TestHTTP_MiddlewareUsesRouteTemplate(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewHTTP(reg)

	r := chi.NewRouter()
	r.Use(m.Middleware)
	r.Get("/api/v1/avatars/{avatar_id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, id := range []string{"abc", "def"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+id, nil))
		require.Equal(t, http.StatusOK, rec.Code)
	}

	const expected = `
# HELP http_requests_total Total HTTP requests by method, route template and status code.
# TYPE http_requests_total counter
http_requests_total{code="200",method="GET",route="/api/v1/avatars/{avatar_id}"} 2
`
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(expected), "http_requests_total"))
}
