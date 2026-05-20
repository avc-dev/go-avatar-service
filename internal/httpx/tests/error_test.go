package httpx_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
)

func TestWriteError(t *testing.T) {
	t.Run("without details: status, content-type, body without details key", func(t *testing.T) {
		rec := httptest.NewRecorder()

		httpx.WriteError(rec, http.StatusBadRequest, "bad input")

		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		// Assert the wire shape: details is omitempty, so it must NOT appear.
		var raw map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
		require.Equal(t, "bad input", raw["error"])
		_, hasDetails := raw["details"]
		require.False(t, hasDetails, "details key must be omitted when empty")
	})

	t.Run("with details: body includes details", func(t *testing.T) {
		rec := httptest.NewRecorder()

		httpx.WriteError(rec, http.StatusUnprocessableEntity, "invalid uuid", "parse error: bad char")

		require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var body httpx.ErrorResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Equal(t, "invalid uuid", body.Error)
		require.Equal(t, "parse error: bad char", body.Details)
	})

	t.Run("variadic details: only first used", func(t *testing.T) {
		rec := httptest.NewRecorder()

		httpx.WriteError(rec, http.StatusBadRequest, "msg", "first", "second", "third")

		var body httpx.ErrorResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Equal(t, "msg", body.Error)
		require.Equal(t, "first", body.Details)
	})
}

func TestWriteJSON(t *testing.T) {
	t.Run("map body: status, content-type, body match", func(t *testing.T) {
		rec := httptest.NewRecorder()
		payload := map[string]any{"max_size": 1024, "error": "too large"}

		httpx.WriteJSON(rec, http.StatusRequestEntityTooLarge, payload)

		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var got map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Equal(t, "too large", got["error"])
		// JSON numbers decode as float64 into map[string]any.
		require.Equal(t, float64(1024), got["max_size"])
	})

	t.Run("struct body: encoded correctly", func(t *testing.T) {
		type sample struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
		rec := httptest.NewRecorder()

		httpx.WriteJSON(rec, http.StatusOK, sample{Name: "alpha", Count: 7})

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var got sample
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Equal(t, sample{Name: "alpha", Count: 7}, got)
	})
}
