package avatar

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
	"github.com/avc-dev/go-avatar-service/internal/middleware"
)

// Delete handles DELETE /api/v1/avatars/{avatar_id}.
//
// The route MUST be mounted under middleware.UserID — see Upload for the
// rationale on treating a missing user id as 500.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	requestingUserID, err := middleware.UserIDFromContext(ctx)
	if err != nil {
		h.log.Error("delete: X-User-ID missing from context — route is not wired with the UserID middleware",
			"err", err,
		)
		httpx.WriteError(w, http.StatusInternalServerError, "Internal error")
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "avatar_id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "avatar_id must be a valid UUID", err.Error())
		return
	}

	if err := h.svc.Delete(ctx, id, requestingUserID); err != nil {
		status, msg := mapServiceError(err)
		if status >= 500 {
			h.log.Error("delete: service.Delete failed",
				"err", err,
				"avatar_id", id,
				"user_id", requestingUserID,
			)
		}
		httpx.WriteError(w, status, msg)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
