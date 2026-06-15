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
// claims the avatar atomically (idempotency gate), generates thumbnails via
// the image resizer, and updates the DB status to completed or failed.
//
// Idempotency is enforced by a single conditional UPDATE in the repository
// (ClaimForProcessing): only a row whose processing_status is exactly
// 'pending' is moved to 'processing' and returned. A duplicate delivery
// races against the same row and at most one worker wins — the loser sees
// ErrNotFound and acks without doing work. This collapses two previously
// separate steps ("read state" + "mark processing") into one TOCTOU-free
// operation.
//
// Signature matches broker.Handler so the consumer loop can route deliveries
// directly to this method without an adapter.
func (w *Worker) HandleUploaded(ctx context.Context, body []byte, messageID string) error {
	// status is read by the deferred recorder; default to success and downgrade
	// it on the skip/failure paths below.
	start := time.Now()
	status := statusSuccess
	defer func() { w.metrics.Record(eventUploaded, status, time.Since(start).Seconds()) }()

	var event domain.AvatarUploadEvent
	if err := json.Unmarshal(body, &event); err != nil {
		// Malformed message — retrying won't help; send to DLX immediately.
		status = statusFailed
		return fmt.Errorf("worker: unmarshal upload event (msg %s): %w", messageID, err)
	}

	// Atomic claim: either we own the row now (status=processing) or
	// somebody else already claimed/finished it (or it's gone). Both
	// non-actionable cases collapse to ErrNotFound.
	var avatar *domain.Avatar
	if err := withRetry(ctx, w.log, w.retryDelays, "claim avatar",
		func(c context.Context) error {
			var claimErr error
			avatar, claimErr = w.repo.ClaimForProcessing(c, event.AvatarID)
			return claimErr
		},
	); err != nil {
		if errors.Is(err, repoavatar.ErrNotFound) {
			status = statusSkipped
			w.log.Info("worker: upload event not actionable (missing or already claimed)",
				"avatar_id", event.AvatarID, "message_id", messageID)
			return nil
		}
		status = statusFailed
		return err
	}

	// Use the S3 key from the claimed row, not the event body — the DB is the
	// source of truth, and using it here means a malformed (but well-typed)
	// event with a forged S3 key cannot redirect us to a different object.
	//
	// generateThumbnails returns the partial map of thumbnails it managed to
	// upload before the failure, so markFailed can best-effort-delete them and
	// avoid leaving orphans in S3 for the operator to chase down later.
	thumbs, err := w.generateThumbnails(ctx, event.AvatarID, avatar.S3Key)
	if err != nil {
		status = statusFailed
		w.markFailed(ctx, event.AvatarID, thumbs, err)
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
		status = statusFailed
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
//
// Return contract on failure:
//   - The returned map contains every thumbnail that was successfully uploaded
//     to S3 *before* the failure. Callers (specifically markFailed) use this
//     to best-effort-delete those orphans so we don't accumulate garbage in
//     object storage between failed runs.
//   - On full success the map has one entry per configured thumbnail size and
//     err is nil.
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
	// runs without retry; only the upload retries. The map accumulates
	// successfully-uploaded keys as we go, so a mid-loop failure can hand
	// back the partial set for cleanup.
	thumbs := make(map[string]string, len(w.thumbSizes))
	for _, size := range w.thumbSizes {
		jpegBytes, err := w.resizer.ResizeToJPEG(ctx, bytes.NewReader(original), size.Width, size.Height, jpegQuality)
		if err != nil {
			return thumbs, fmt.Errorf("resize %s: %w", size.Name, err)
		}

		thumbKey := fmt.Sprintf("thumbnails/%s/%s.jpg", avatarID, size.Name)
		if uploadErr := withRetry(ctx, w.log, w.retryDelays, "upload thumbnail",
			func(c context.Context) error {
				return w.storage.Upload(c, thumbKey, bytes.NewReader(jpegBytes), int64(len(jpegBytes)), "image/jpeg")
			},
		); uploadErr != nil {
			return thumbs, fmt.Errorf("upload thumbnail %s: %w", thumbKey, uploadErr)
		}
		thumbs[size.Name] = thumbKey
	}

	return thumbs, nil
}

// markFailed is a best-effort failure path. It does two things, neither of
// which is allowed to mask the original cause:
//  1. Delete any thumbnail objects that were uploaded before the failure
//     (partialThumbs) so we don't leave orphans in S3 for a future operator
//     or reconciliation job to chase down.
//  2. Transition the DB row to ProcessingStatusFailed so observers (the
//     web UI, future re-processing scripts) can see the terminal state.
//
// All errors are logged at ERROR but not propagated — the caller is already
// returning the underlying cause to the consumer, which will route the
// message to the DLX for inspection.
func (w *Worker) markFailed(ctx context.Context, avatarID uuid.UUID, partialThumbs map[string]string, cause error) {
	// Use a fresh context for both cleanup and the status mark — the original
	// may be cancelled (e.g. graceful shutdown), and we want one last attempt
	// at each side effect.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	for size, key := range partialThumbs {
		if err := w.storage.Delete(cleanupCtx, key); err != nil {
			// Best-effort: a leaked thumbnail is annoying but not fatal, and
			// retrying it here would block status persistence below.
			w.log.Warn("worker: failed to clean up partial thumbnail on failure path",
				"avatar_id", avatarID,
				"size", size,
				"s3_key", key,
				"err", err)
		}
	}

	if err := w.repo.UpdateProcessingStatus(cleanupCtx, avatarID, domain.ProcessingStatusFailed, nil); err != nil {
		w.log.Error("worker: failed to mark avatar as failed",
			"avatar_id", avatarID,
			"cause", cause,
			"mark_err", err)
	}
}
