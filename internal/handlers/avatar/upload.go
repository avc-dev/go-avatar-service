package avatar

import (
	"errors"
	"net/http"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
	"github.com/avc-dev/go-avatar-service/internal/middleware"
	svcavatar "github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

// Upload handles POST /api/v1/avatars.
//
// The route MUST be mounted under middleware.UserID — the handler treats a
// missing user id as a programmer error (route misconfiguration) and responds
// 500 rather than 401 to avoid masking a wiring bug as an auth failure.
//
// The multipart body is bounded by http.MaxBytesReader BEFORE ParseMultipartForm
// so an oversize body trips the limit during form parsing rather than after
// it has been buffered into memory.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, err := middleware.UserIDFromContext(ctx)
	if err != nil {
		h.log.Error("upload: X-User-ID missing from context — route is not wired with the UserID middleware",
			"err", err,
		)
		httpx.WriteError(w, http.StatusInternalServerError, "Internal error")
		return
	}

	// Bound the body before parsing so the parser respects the limit.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)

	if err := r.ParseMultipartForm(h.maxUploadBytes); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			httpx.WriteJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
				"error":    "File too large",
				"max_size": h.maxUploadBytes,
			})
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "Invalid multipart body", err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "file field is required", err.Error())
		return
	}
	defer func() { _ = file.Close() }()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	a, err := h.svc.Upload(ctx, svcavatar.UploadParams{
		UserID:   userID,
		FileName: header.Filename,
		MIMEType: contentType,
		Size:     header.Size,
		Reader:   file,
	})
	if err != nil {
		status, msg := mapServiceError(err)
		if status >= 500 {
			h.log.Error("upload: service.Upload failed", "err", err, "user_id", userID)
		}
		httpx.WriteError(w, status, msg)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, UploadResponse{
		ID:        a.ID,
		UserID:    a.UserID,
		URL:       "/api/v1/avatars/" + a.ID.String(),
		Status:    "processing",
		CreatedAt: a.CreatedAt,
	})
}
