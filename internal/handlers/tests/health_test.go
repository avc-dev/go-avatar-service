package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/handlers"
)

// fakeChecker is a local stand-in for the HealthChecker interface. Returning a
// fixed error keeps the tests free of timing and shared-state concerns.
type fakeChecker struct{ err error }

func (f fakeChecker) Check(_ context.Context) error { return f.err }

// healthBody mirrors the unexported response shape so tests can decode it.
type healthBody struct {
	Status     string            `json:"status"`
	Components map[string]string `json:"components"`
}

func doHealth(t *testing.T, h *handlers.HealthHandler) (*httptest.ResponseRecorder, healthBody) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	var body healthBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return rec, body
}

func TestHealthHandler(t *testing.T) {
	t.Run("all OK: 200 + status ok + every component ok", func(t *testing.T) {
		checkers := map[string]handlers.HealthChecker{
			"postgres": fakeChecker{},
			"minio":    fakeChecker{},
			"rabbit":   fakeChecker{},
		}
		h := handlers.NewHealthHandler(checkers, time.Second)

		rec, body := doHealth(t, h)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "ok", body.Status)
		require.Equal(t, map[string]string{
			"postgres": "ok",
			"minio":    "ok",
			"rabbit":   "ok",
		}, body.Components)
	})

	t.Run("one fails: 503, status degraded, failing component prefixed", func(t *testing.T) {
		failErr := errors.New("connection refused")
		checkers := map[string]handlers.HealthChecker{
			"postgres": fakeChecker{},
			"minio":    fakeChecker{err: failErr},
			"rabbit":   fakeChecker{},
		}
		h := handlers.NewHealthHandler(checkers, time.Second)

		rec, body := doHealth(t, h)

		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Equal(t, "degraded", body.Status)
		require.Equal(t, "ok", body.Components["postgres"])
		require.Equal(t, "ok", body.Components["rabbit"])
		require.Equal(t, "error: connection refused", body.Components["minio"])
	})

	t.Run("all fail: 503, every component reports its error", func(t *testing.T) {
		checkers := map[string]handlers.HealthChecker{
			"postgres": fakeChecker{err: errors.New("pg down")},
			"minio":    fakeChecker{err: errors.New("s3 down")},
		}
		h := handlers.NewHealthHandler(checkers, time.Second)

		rec, body := doHealth(t, h)

		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Equal(t, "degraded", body.Status)
		require.Equal(t, "error: pg down", body.Components["postgres"])
		require.Equal(t, "error: s3 down", body.Components["minio"])
	})

	t.Run("HealthCheckerFunc adapter: invoked", func(t *testing.T) {
		var called bool
		fn := handlers.HealthCheckerFunc(func(_ context.Context) error {
			called = true
			return nil
		})
		h := handlers.NewHealthHandler(map[string]handlers.HealthChecker{"fn": fn}, time.Second)

		rec, body := doHealth(t, h)

		require.True(t, called, "HealthCheckerFunc must be invoked")
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "ok", body.Components["fn"])
	})

	t.Run("timeout propagation: per-checker error wraps DeadlineExceeded", func(t *testing.T) {
		slow := handlers.HealthCheckerFunc(func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})
		h := handlers.NewHealthHandler(map[string]handlers.HealthChecker{"slow": slow}, 10*time.Millisecond)

		rec, body := doHealth(t, h)

		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Equal(t, "degraded", body.Status)
		require.Equal(t, "error: "+context.DeadlineExceeded.Error(), body.Components["slow"])
	})

	t.Run("empty checkers map: 200 with empty components, no panic", func(t *testing.T) {
		h := handlers.NewHealthHandler(map[string]handlers.HealthChecker{}, time.Second)

		require.NotPanics(t, func() {
			rec, body := doHealth(t, h)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "ok", body.Status)
			require.Empty(t, body.Components)
		})
	})
}
