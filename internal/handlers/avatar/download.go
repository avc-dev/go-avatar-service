package avatar

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
)

// Download handles GET /api/v1/avatars/{avatar_id}.
//
// Headers must be set BEFORE the first Write — once io.Copy starts, the
// response is committed with the implicit 200 OK. Query parameters (size,
// format) are accepted but ignored in this iteration; a future iteration
// wires them to thumbnail variants.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := uuid.Parse(chi.URLParam(r, "avatar_id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "avatar_id must be a valid UUID", err.Error())
		return
	}

	result, err := h.svc.DownloadOriginal(ctx, id)
	if err != nil {
		status, msg := mapServiceError(err)
		if status >= 500 {
			h.log.Error("download: service.DownloadOriginal failed", "err", err, "avatar_id", id)
		}
		httpx.WriteError(w, status, msg)
		return
	}
	defer result.Reader.Close()

	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(result.Size, 10))

	if _, err := io.Copy(w, result.Reader); err != nil {
		// Headers and the implicit 200 OK are already on the wire — we cannot
		// change the status now. Log a WARN so the partial transfer is
		// observable in operational logs.
		h.log.Warn("download: copy to response writer failed mid-stream",
			"err", err,
			"avatar_id", id,
		)
	}
}
