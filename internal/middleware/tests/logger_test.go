package middleware_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/middleware"
)

// decodeFirstLogLine parses the first JSON log entry from buf. The Logger
// middleware writes exactly one line per request.
func decodeFirstLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	line, err := buf.ReadBytes('\n')
	require.NoError(t, err, "expected at least one log line; got: %q", buf.String())

	var entry map[string]any
	require.NoError(t, json.Unmarshal(line, &entry))
	return entry
}

func newJSONLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLoggerMiddleware(t *testing.T) {
	t.Run("simple GET: method, path, status=200, duration>0, bytes=0", func(t *testing.T) {
		var buf bytes.Buffer
		log := newJSONLogger(&buf)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// no explicit status, no body — relies on default 200
			w.WriteHeader(http.StatusOK)
		})
		h := middleware.Logger(log)(next)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		entry := decodeFirstLogLine(t, &buf)
		require.Equal(t, "GET", entry["method"])
		require.Equal(t, "/test", entry["path"])
		require.Equal(t, float64(http.StatusOK), entry["status"])
		require.Equal(t, float64(0), entry["bytes"])
		// slog encodes time.Duration as nanoseconds (number) in JSON.
		duration, ok := entry["duration"].(float64)
		require.True(t, ok, "duration must be numeric, got %T", entry["duration"])
		require.Greater(t, duration, float64(0))
	})

	t.Run("explicit status + body: status and bytes captured", func(t *testing.T) {
		var buf bytes.Buffer
		log := newJSONLogger(&buf)

		const body = "hello teapot"
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			_, _ = io.WriteString(w, body)
		})
		h := middleware.Logger(log)(next)

		req := httptest.NewRequest(http.MethodPost, "/brew", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		entry := decodeFirstLogLine(t, &buf)
		require.Equal(t, "POST", entry["method"])
		require.Equal(t, "/brew", entry["path"])
		require.Equal(t, float64(http.StatusTeapot), entry["status"])
		require.Equal(t, float64(len(body)), entry["bytes"])
	})

	t.Run("request_id present when chi RequestID ran earlier", func(t *testing.T) {
		var buf bytes.Buffer
		log := newJSONLogger(&buf)

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		// Chain: RequestID -> Logger -> handler. Composed manually as the
		// brief requires; no chi router involved.
		h := chimw.RequestID(middleware.Logger(log)(next))

		req := httptest.NewRequest(http.MethodGet, "/with-id", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		entry := decodeFirstLogLine(t, &buf)
		reqID, ok := entry["request_id"].(string)
		require.True(t, ok, "request_id must be a string, got %T", entry["request_id"])
		require.NotEmpty(t, reqID)
	})
}
