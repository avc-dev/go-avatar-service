package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
)

// userIDCtxKey is the unexported type used as the context.WithValue key so it
// cannot collide with keys defined in other packages.
type userIDCtxKey struct{}

// ErrMissingUserID is returned by UserIDFromContext when the middleware did
// not run or did not store an id (e.g. the handler is mounted under a route
// group that didn't apply this middleware).
var ErrMissingUserID = errors.New("X-User-ID not in context")

// UserID is HTTP middleware that extracts and validates the X-User-ID header,
// then stores the parsed uuid in the request context. It must only be applied
// to routes that require authentication (writes), not to public reads.
//
// Validation policy (project-wide): the header must be present, non-empty,
// parseable as a UUID, and specifically version 7. Any failure returns 400
// with a JSON ErrorResponse and does NOT invoke the next handler.
func UserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimSpace(r.Header.Get("X-User-ID"))
		if raw == "" {
			httpx.WriteError(w, http.StatusBadRequest, "X-User-ID header is required")
			return
		}

		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "X-User-ID must be a valid UUID", err.Error())
			return
		}
		if id.Version() != 7 {
			httpx.WriteError(w, http.StatusBadRequest, "X-User-ID must be UUID v7")
			return
		}

		ctx := context.WithValue(r.Context(), userIDCtxKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserIDFromContext retrieves the validated user id stored by the UserID
// middleware. Returns ErrMissingUserID when the middleware did not run for
// this request — handlers should treat that as a programmer error (route
// wiring bug), not a 401/403.
func UserIDFromContext(ctx context.Context) (uuid.UUID, error) {
	id, ok := ctx.Value(userIDCtxKey{}).(uuid.UUID)
	if !ok {
		return uuid.Nil, ErrMissingUserID
	}
	return id, nil
}
