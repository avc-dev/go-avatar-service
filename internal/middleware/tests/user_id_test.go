package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
	"github.com/avc-dev/go-avatar-service/internal/middleware"
)

// spyHandler is the downstream handler used to verify whether UserID let the
// request through and what userID was visible from context.
type spyHandler struct {
	called bool
	gotID  uuid.UUID
	gotErr error
}

func (s *spyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.called = true
	s.gotID, s.gotErr = middleware.UserIDFromContext(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func decodeErr(t *testing.T, body []byte) httpx.ErrorResponse {
	t.Helper()
	var er httpx.ErrorResponse
	require.NoError(t, json.Unmarshal(body, &er))
	return er
}

func TestUserIDMiddleware(t *testing.T) {
	t.Run("valid uuid v7: next invoked, context populated, response untouched", func(t *testing.T) {
		spy := &spyHandler{}
		h := middleware.UserID(spy)
		id := uuid.Must(uuid.NewV7())

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-User-ID", id.String())
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		require.True(t, spy.called, "next handler must be invoked on valid uuid v7")
		require.NoError(t, spy.gotErr)
		require.Equal(t, id, spy.gotID)
		// The spy explicitly wrote 204 — proves middleware did not interfere.
		require.Equal(t, http.StatusNoContent, rec.Code)
	})

	type rejectCase struct {
		name          string
		setHeader     bool
		headerVal     string
		wantMessage   string
		wantDetailsIn string // substring expected in details (empty means no check)
	}

	cases := []rejectCase{
		{
			name:        "header missing",
			setHeader:   false,
			wantMessage: "X-User-ID header is required",
		},
		{
			name:        "header empty string",
			setHeader:   true,
			headerVal:   "",
			wantMessage: "X-User-ID header is required",
		},
		{
			name:        "header whitespace only",
			setHeader:   true,
			headerVal:   "   \t  ",
			wantMessage: "X-User-ID header is required",
		},
		{
			name:          "malformed uuid",
			setHeader:     true,
			headerVal:     "not-a-uuid",
			wantMessage:   "X-User-ID must be a valid UUID",
			wantDetailsIn: "invalid",
		},
		{
			name:        "uuid v4",
			setHeader:   true,
			headerVal:   uuid.New().String(), // uuid.New returns v4
			wantMessage: "X-User-ID must be UUID v7",
		},
		{
			name:        "uuid v1",
			setHeader:   true,
			headerVal:   mustUUIDv1(t).String(),
			wantMessage: "X-User-ID must be UUID v7",
		},
	}

	for _, tc := range cases {
		t.Run("reject: "+tc.name, func(t *testing.T) {
			spy := &spyHandler{}
			h := middleware.UserID(spy)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.setHeader {
				req.Header.Set("X-User-ID", tc.headerVal)
			}
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			require.False(t, spy.called, "next handler must NOT be invoked on rejection")
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

			body := decodeErr(t, rec.Body.Bytes())
			require.Equal(t, tc.wantMessage, body.Error)
			if tc.wantDetailsIn != "" {
				require.Contains(t, body.Details, tc.wantDetailsIn)
			}
		})
	}
}

func TestUserIDFromContext_Missing(t *testing.T) {
	id, err := middleware.UserIDFromContext(t.Context())

	require.ErrorIs(t, err, middleware.ErrMissingUserID)
	require.Equal(t, uuid.Nil, id)
}

// mustUUIDv1 returns a v1 UUID via the google/uuid package. Wrapped here so a
// failure in the rare path is reported via t.Fatalf rather than panicking.
func mustUUIDv1(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewUUID()
	require.NoError(t, err)
	require.EqualValues(t, 1, id.Version())
	return id
}
