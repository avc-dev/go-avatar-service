package avatar

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
)

// GetUserAvatar handles GET /api/v1/users/{user_id}/avatar.
//
// Two service calls are required: first look up the user's current avatar,
// then stream its binary. A 404 from the first call is reported as such — a
// placeholder image is deliberately not served here (deferred to Iter 6).
func (h *Handler) GetUserAvatar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, err := parseUserIDPath(chi.URLParam(r, "user_id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "user_id must be a valid UUID v7", err.Error())
		return
	}

	current, err := h.svc.GetCurrentByUserID(ctx, userID)
	if err != nil {
		status, msg := mapServiceError(err)
		if status >= 500 {
			h.log.Error("get_user_avatar: service.GetCurrentByUserID failed",
				"err", err,
				"user_id", userID,
			)
		}
		httpx.WriteError(w, status, msg)
		return
	}

	result, err := h.svc.DownloadOriginal(ctx, current.ID)
	if err != nil {
		status, msg := mapServiceError(err)
		if status >= 500 {
			h.log.Error("get_user_avatar: service.DownloadOriginal failed",
				"err", err,
				"avatar_id", current.ID,
			)
		}
		httpx.WriteError(w, status, msg)
		return
	}
	defer result.Reader.Close()

	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(result.Size, 10))

	if _, err := io.Copy(w, result.Reader); err != nil {
		h.log.Warn("get_user_avatar: copy to response writer failed mid-stream",
			"err", err,
			"avatar_id", current.ID,
		)
	}
}
