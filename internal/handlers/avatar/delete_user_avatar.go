package avatar

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
	"github.com/avc-dev/go-avatar-service/internal/middleware"
)

// DeleteUserAvatar handles DELETE /api/v1/users/{user_id}/avatar.
//
// Ownership is enforced at the handler boundary: the authenticated user id
// (from the X-User-ID middleware) must equal the {user_id} path parameter.
// The service layer also re-checks ownership when it deletes the specific
// avatar record, but rejecting the mismatch here is cheaper and gives a
// clearer error to the client.
func (h *Handler) DeleteUserAvatar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	requestingUserID, err := middleware.UserIDFromContext(ctx)
	if err != nil {
		h.log.Error("delete_user_avatar: X-User-ID missing from context — route is not wired with the UserID middleware",
			"err", err,
		)
		httpx.WriteError(w, http.StatusInternalServerError, "Internal error")
		return
	}

	userID, err := parseUserIDPath(chi.URLParam(r, "user_id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "user_id must be a valid UUID v7", err.Error())
		return
	}

	if requestingUserID != userID {
		httpx.WriteError(w, http.StatusForbidden, "Forbidden", "you can only delete your own avatars")
		return
	}

	if err := h.svc.DeleteCurrentByUserID(ctx, userID); err != nil {
		status, msg := mapServiceError(err)
		if status >= 500 {
			h.log.Error("delete_user_avatar: service.DeleteCurrentByUserID failed",
				"err", err,
				"user_id", userID,
			)
		}
		httpx.WriteError(w, status, msg)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
