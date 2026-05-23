package avatar

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
)

// ListUserAvatars handles GET /api/v1/users/{user_id}/avatars.
//
// An empty list is a valid result — the response body always carries a
// non-nil "avatars" array.
func (h *Handler) ListUserAvatars(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, err := parseUserIDPath(chi.URLParam(r, "user_id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "user_id must be a valid UUID v7", err.Error())
		return
	}

	list, err := h.svc.ListByUserID(ctx, userID)
	if err != nil {
		status, msg := mapServiceError(err)
		if status >= 500 {
			h.log.Error("list_user_avatars: service.ListByUserID failed",
				"err", err,
				"user_id", userID,
			)
		}
		httpx.WriteError(w, status, msg)
		return
	}

	items := make([]MetadataResponse, 0, len(list))
	for _, a := range list {
		items = append(items, toMetadataResponse(a))
	}
	httpx.WriteJSON(w, http.StatusOK, ListResponse{Avatars: items})
}
