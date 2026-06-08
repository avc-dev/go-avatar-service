package avatar

import (
	"errors"
	"net/http"
	"time"

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

	// Validate the upload's real content type via magic bytes — the multipart
	// Content-Type and filename extension are client-supplied and can lie. The
	// returned reader replays the bytes consumed for sniffing, so the service
	// receives an unchanged stream.
	detectedMIME, body, err := sniffImageMIME(file)
	if err != nil {
		if errors.Is(err, errUnsupportedMIME) {
			httpx.WriteJSON(w, http.StatusUnsupportedMediaType, map[string]any{
				"error":         "Unsupported image type",
				"detected_mime": detectedMIME,
				"allowed":       []string{"image/jpeg", "image/png", "image/gif", "image/webp"},
			})
			return
		}
		h.log.Error("upload: sniff failed", "err", err, "user_id", userID)
		httpx.WriteError(w, http.StatusBadRequest, "Could not read uploaded file", err.Error())
		return
	}

	// Time the actual upload work (not the preceding client-side validation):
	// the business counter should reflect upload operations that ran, while
	// 4xx validation rejections are already visible in the HTTP RED metrics.
	uploadStart := time.Now()
	a, err := h.svc.Upload(ctx, svcavatar.UploadParams{
		UserID:   userID,
		FileName: header.Filename,
		MIMEType: detectedMIME,
		Size:     header.Size,
		Reader:   body,
	})
	if err != nil {
		h.uploadMetrics.Record("error", time.Since(uploadStart).Seconds())
		status, msg := mapServiceError(err)
		if status >= 500 {
			h.log.Error("upload: service.Upload failed", "err", err, "user_id", userID)
		}
		httpx.WriteError(w, status, msg)
		return
	}
	h.uploadMetrics.Record("success", time.Since(uploadStart).Seconds())

	httpx.WriteJSON(w, http.StatusCreated, UploadResponse{
		ID:        a.ID,
		UserID:    a.UserID,
		URL:       "/api/v1/avatars/" + a.ID.String(),
		Status:    "processing",
		CreatedAt: a.CreatedAt,
	})
}
