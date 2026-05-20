package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

// HandleUploaded processes one AvatarUploadEvent: validates the message,
// guards idempotency via the persisted processing_status, generates thumbnails
// via the image resizer, and updates the DB status to completed or failed.
//
// Signature matches broker.Handler so the consumer loop can route deliveries
// directly to this method without an adapter.
func (w *Worker) HandleUploaded(ctx context.Context, body []byte, messageID string) error {
	var event domain.AvatarUploadEvent
	if err := json.Unmarshal(body, &event); err != nil {
		// Malformed message — retrying won't help; send to DLX immediately.
		return fmt.Errorf("worker: unmarshal upload event (msg %s): %w", messageID, err)
	}

	// 1. Read current state — also our idempotency gate.
	var avatar *domain.Avatar
	if err := withRetry(ctx, w.log, w.retryDelays, "get avatar",
		func(c context.Context) error {
			var getErr error
			avatar, getErr = w.repo.GetByID(c, event.AvatarID)
			return getErr
		},
	); err != nil {
		if errors.Is(err, repoavatar.ErrNotFound) {
			// Avatar was hard-deleted (or never persisted) before we got here.
			// The event is stale; ack so it doesn't loop forever.
			w.log.Info("worker: upload event for missing avatar, skipping",
				"avatar_id", event.AvatarID, "message_id", messageID)
			return nil
		}
		return err
	}

	// 2. Idempotency: skip if not pending.
	switch avatar.ProcessingStatus {
	case domain.ProcessingStatusPending:
		// Fall through to processing.
	case domain.ProcessingStatusProcessing,
		domain.ProcessingStatusCompleted,
		domain.ProcessingStatusFailed:
		w.log.Info("worker: upload event already handled, skipping",
			"avatar_id", event.AvatarID,
			"status", avatar.ProcessingStatus,
			"message_id", messageID)
		return nil
	default:
		// An unknown status value is a programmer error: the DB CHECK constraint
		// rules it out. Surface loudly via DLX.
		return fmt.Errorf("worker: unknown processing status %q for avatar %s",
			avatar.ProcessingStatus, event.AvatarID)
	}

	// 3. Move status to processing (claims the work for this worker).
	if err := withRetry(ctx, w.log, w.retryDelays, "mark processing",
		func(c context.Context) error {
			return w.repo.UpdateProcessingStatus(c, event.AvatarID, domain.ProcessingStatusProcessing, nil)
		},
	); err != nil {
		return fmt.Errorf("worker: claim avatar %s: %w", event.AvatarID, err)
	}

	// 4. Do the heavy lifting; on any failure, do a best-effort mark-as-failed.
	thumbs, err := w.generateThumbnails(ctx, event.AvatarID, event.S3Key)
	if err != nil {
		w.markFailed(ctx, event.AvatarID, err)
		return fmt.Errorf("worker: generate thumbnails for avatar %s: %w", event.AvatarID, err)
	}

	// 5. Finalise: completed status + thumbnail key map.
	if err := withRetry(ctx, w.log, w.retryDelays, "mark completed",
		func(c context.Context) error {
			return w.repo.UpdateProcessingStatus(c, event.AvatarID, domain.ProcessingStatusCompleted, thumbs)
		},
	); err != nil {
		// Thumbnails already uploaded to S3; only the status bump failed. We
		// return an error so the message goes to DLX — manual reconciliation
		// will fix the DB state, but the S3 work is not lost.
		return fmt.Errorf("worker: mark completed for avatar %s: %w", event.AvatarID, err)
	}

	w.log.Info("worker: avatar processed",
		"avatar_id", event.AvatarID,
		"thumb_count", len(thumbs),
		"message_id", messageID)
	return nil
}

// generateThumbnails downloads the original, then for each configured size
// produces a JPEG thumbnail and uploads it to S3 under thumbnails/{id}/{size}.jpg.
// Returns a map suitable for repo.UpdateProcessingStatus.
func (w *Worker) generateThumbnails(ctx context.Context, avatarID uuid.UUID, s3Key string) (map[string]string, error) {
	// Download original (with retry).
	var (
		original     []byte
		downloadSize int64
	)
	if err := withRetry(ctx, w.log, w.retryDelays, "download original",
		func(c context.Context) error {
			result, err := w.storage.Download(c, s3Key)
			if err != nil {
				return err
			}
			defer func() { _ = result.Reader.Close() }()

			buf, readErr := io.ReadAll(result.Reader)
			if readErr != nil {
				return fmt.Errorf("read body: %w", readErr)
			}
			original = buf
			downloadSize = result.Size
			return nil
		},
	); err != nil {
		return nil, fmt.Errorf("download original %s: %w", s3Key, err)
	}

	w.log.Debug("worker: original downloaded", "s3_key", s3Key, "size", downloadSize)

	// Resize + upload per size. Resize is deterministic and CPU-bound, so it
	// runs without retry; only the upload retries.
	thumbs := make(map[string]string, len(w.thumbSizes))
	for _, size := range w.thumbSizes {
		jpegBytes, err := w.resizer.ResizeToJPEG(ctx, bytes.NewReader(original), size.Width, size.Height, jpegQuality)
		if err != nil {
			return nil, fmt.Errorf("resize %s: %w", size.Name, err)
		}

		thumbKey := fmt.Sprintf("thumbnails/%s/%s.jpg", avatarID, size.Name)
		if uploadErr := withRetry(ctx, w.log, w.retryDelays, "upload thumbnail",
			func(c context.Context) error {
				return w.storage.Upload(c, thumbKey, bytes.NewReader(jpegBytes), int64(len(jpegBytes)), "image/jpeg")
			},
		); uploadErr != nil {
			return nil, fmt.Errorf("upload thumbnail %s: %w", thumbKey, uploadErr)
		}
		thumbs[size.Name] = thumbKey
	}

	return thumbs, nil
}

// markFailed is a best-effort transition to ProcessingStatusFailed used in
// error paths. Its own failure is logged but not propagated — the caller will
// already be returning the original error to the consumer.
func (w *Worker) markFailed(ctx context.Context, avatarID uuid.UUID, cause error) {
	// Use a fresh context for the mark — even if the original was cancelled
	// (e.g. graceful shutdown), we want one last attempt to record the state.
	markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := w.repo.UpdateProcessingStatus(markCtx, avatarID, domain.ProcessingStatusFailed, nil); err != nil {
		w.log.Error("worker: failed to mark avatar as failed",
			"avatar_id", avatarID,
			"cause", cause,
			"mark_err", err)
	}
}
