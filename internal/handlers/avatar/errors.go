package avatar

import (
	"errors"
	"net/http"

	svcavatar "github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

// mapServiceError translates a service-layer error into an HTTP status and a
// user-facing message. The default case is a 500: callers are expected to log
// the wrapped err themselves (handlers do this at slog.Error before writing
// the response) so the original cause is preserved in the logs even though
// the wire body is intentionally generic.
func mapServiceError(err error) (int, string) {
	switch {
	case errors.Is(err, svcavatar.ErrNotFound):
		return http.StatusNotFound, "Avatar not found"
	case errors.Is(err, svcavatar.ErrForbidden):
		return http.StatusForbidden, "Forbidden"
	default:
		return http.StatusInternalServerError, "Internal error"
	}
}
