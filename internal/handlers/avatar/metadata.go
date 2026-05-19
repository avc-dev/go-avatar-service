package avatar

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
)

// Metadata handles GET /api/v1/avatars/{avatar_id}/metadata.
func (h *Handler) Metadata(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := uuid.Parse(chi.URLParam(r, "avatar_id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "avatar_id must be a valid UUID", err.Error())
		return
	}

	a, err := h.svc.GetMetadata(ctx, id)
	if err != nil {
		status, msg := mapServiceError(err)
		if status >= 500 {
			h.log.Error("metadata: service.GetMetadata failed", "err", err, "avatar_id", id)
		}
		httpx.WriteError(w, status, msg)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toMetadataResponse(a))
}
