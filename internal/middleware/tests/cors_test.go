package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/middleware"
)

// downstream204 is the protected handler used by the CORS tests; if the
// middleware allows the request through, the recorder shows 204.
var downstream204 = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})

func TestCORS(t *testing.T) {
	t.Run("preflight from whitelisted origin returns Allow-Origin header", func(t *testing.T) {
		mw := middleware.CORS([]string{"https://forum.example.com"})

		req := httptest.NewRequest(http.MethodOptions, "/api/v1/avatars", nil)
		req.Header.Set("Origin", "https://forum.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		// Without this header, go-chi/cors treats the OPTIONS as a regular
		// request and does NOT short-circuit to a 204 preflight reply.
		req.Header.Set("Access-Control-Request-Headers", "Content-Type")

		rec := httptest.NewRecorder()
		mw(downstream204).ServeHTTP(rec, req)

		require.Equal(t, "https://forum.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
		// chi/cors writes 200 for preflight (not 204) — that's the library's
		// convention, also valid per the CORS spec. The test cements the
		// contract so a future library swap doesn't quietly break clients
		// that pattern-match on the status.
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("request from non-whitelisted origin gets no Allow-Origin header", func(t *testing.T) {
		mw := middleware.CORS([]string{"https://forum.example.com"})

		// go-chi/cors lets the request through to the handler but does NOT
		// echo Access-Control-Allow-Origin. The browser is then responsible
		// for blocking the response from JS — same contract as standard
		// CORS implementations.
		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/x", nil)
		req.Header.Set("Origin", "https://evil.example.com")

		rec := httptest.NewRecorder()
		mw(downstream204).ServeHTTP(rec, req)

		require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, http.StatusNoContent, rec.Code, "non-preflight requests should still reach the handler")
	})

	t.Run("empty whitelist denies all cross-origin", func(t *testing.T) {
		mw := middleware.CORS(nil)

		req := httptest.NewRequest(http.MethodOptions, "/api/v1/avatars", nil)
		req.Header.Set("Origin", "https://anyone.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type")

		rec := httptest.NewRecorder()
		mw(downstream204).ServeHTTP(rec, req)

		require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("same-origin (no Origin header) is unaffected", func(t *testing.T) {
		mw := middleware.CORS(nil)

		// SPA on the same domain doesn't send Origin for navigation —
		// middleware must let it through transparently.
		req := httptest.NewRequest(http.MethodGet, "/web/index.html", nil)
		rec := httptest.NewRecorder()
		mw(downstream204).ServeHTTP(rec, req)

		require.Equal(t, http.StatusNoContent, rec.Code)
	})
}
