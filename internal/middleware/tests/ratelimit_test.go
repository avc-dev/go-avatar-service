package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/middleware"
)

// okHandler is the downstream used to confirm whether a request was let
// through (status 204) versus blocked by the limiter (429 written by the
// middleware itself).
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})

// runN drives the middleware-wrapped handler n times against the given
// request factory and returns the slice of status codes. We re-create the
// request each iteration because http.Handler may consume the body — even
// though our middleware doesn't, this keeps the test resilient to changes.
func runN(t *testing.T, mw func(http.Handler) http.Handler, n int, makeReq func() *http.Request) []int {
	t.Helper()
	wrapped := mw(okHandler)
	statuses := make([]int, 0, n)
	for i := 0; i < n; i++ {
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, makeReq())
		statuses = append(statuses, rec.Code)
	}
	return statuses
}

func TestIPRateLimit(t *testing.T) {
	t.Run("requests under the budget succeed", func(t *testing.T) {
		mw := middleware.IPRateLimit(5)
		statuses := runN(t, mw, 5, func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/x", nil)
			r.RemoteAddr = "10.0.0.1:1234"
			return r
		})
		for i, s := range statuses {
			require.Equal(t, http.StatusNoContent, s, "request %d should have passed", i)
		}
	})

	t.Run("budget+1 is rejected with 429 and Retry-After", func(t *testing.T) {
		mw := middleware.IPRateLimit(3)

		// Burn the budget.
		runN(t, mw, 3, func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/x", nil)
			r.RemoteAddr = "10.0.0.2:1234"
			return r
		})

		// One more — must be throttled.
		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/x", nil)
		req.RemoteAddr = "10.0.0.2:1234"
		rec := httptest.NewRecorder()
		mw(okHandler).ServeHTTP(rec, req)
		require.Equal(t, http.StatusTooManyRequests, rec.Code)
		require.Equal(t, "60", rec.Header().Get("Retry-After"))
	})

	t.Run("different IPs have independent buckets", func(t *testing.T) {
		mw := middleware.IPRateLimit(2)

		// Drain IP A.
		runN(t, mw, 2, func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = "10.0.0.10:1234"
			return r
		})

		// IP B starts fresh — must be admitted.
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.11:1234"
		rec := httptest.NewRecorder()
		mw(okHandler).ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("rpm<=0 disables the limiter (no-op middleware)", func(t *testing.T) {
		mw := middleware.IPRateLimit(0)
		// Far more than any sane budget — still all 204s.
		statuses := runN(t, mw, 100, func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = "10.0.0.20:1234"
			return r
		})
		for i, s := range statuses {
			require.Equal(t, http.StatusNoContent, s, "request %d should have passed (limiter disabled)", i)
		}
	})
}

func TestUserIDRateLimit(t *testing.T) {
	uid := uuid.Must(uuid.NewV7()).String()

	t.Run("different X-User-IDs have independent buckets", func(t *testing.T) {
		mw := middleware.UserIDRateLimit(2)

		// Drain user A.
		runN(t, mw, 2, func() *http.Request {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", nil)
			r.Header.Set("X-User-ID", uid)
			return r
		})

		// User B starts fresh.
		other := uuid.Must(uuid.NewV7()).String()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", nil)
		req.Header.Set("X-User-ID", other)
		rec := httptest.NewRecorder()
		mw(okHandler).ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("budget exhausted for same X-User-ID returns 429", func(t *testing.T) {
		mw := middleware.UserIDRateLimit(2)
		uid := uuid.Must(uuid.NewV7()).String()

		// Burn the budget.
		runN(t, mw, 2, func() *http.Request {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", nil)
			r.Header.Set("X-User-ID", uid)
			return r
		})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", nil)
		req.Header.Set("X-User-ID", uid)
		rec := httptest.NewRecorder()
		mw(okHandler).ServeHTTP(rec, req)
		require.Equal(t, http.StatusTooManyRequests, rec.Code)
	})

	t.Run("missing X-User-ID falls back to per-IP key", func(t *testing.T) {
		mw := middleware.UserIDRateLimit(1)
		// First request from this IP without the header — admitted.
		req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", nil)
		req.RemoteAddr = "10.0.0.99:1234"
		rec := httptest.NewRecorder()
		mw(okHandler).ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)

		// Second request from the same IP, still no header — limited.
		req2 := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", nil)
		req2.RemoteAddr = "10.0.0.99:1234"
		rec2 := httptest.NewRecorder()
		mw(okHandler).ServeHTTP(rec2, req2)
		require.Equal(t, http.StatusTooManyRequests, rec2.Code)
	})
}
