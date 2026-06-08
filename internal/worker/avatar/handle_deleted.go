package avatar

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// HandleDeleted processes one AvatarDeleteEvent by removing every object
// listed in event.S3Keys from object storage. S3 delete is idempotent
// (deleting a missing key succeeds), so retried deliveries are safe.
//
// Per-key failures accumulate: each key is attempted independently with the
// usual retry policy, and the method returns an error only after all keys
// have been attempted. Returning an error sends the message to the DLX where
// a reconciliation job (future iteration) or an operator can finish the
// cleanup.
func (w *Worker) HandleDeleted(ctx context.Context, body []byte, messageID string) error {
	start := time.Now()
	status := statusSuccess
	defer func() { w.metrics.Record(eventDeleted, status, time.Since(start).Seconds()) }()

	var event domain.AvatarDeleteEvent
	if err := json.Unmarshal(body, &event); err != nil {
		status = statusFailed
		return fmt.Errorf("worker: unmarshal delete event (msg %s): %w", messageID, err)
	}

	var failed []string
	var firstErr error
	for _, key := range event.S3Keys {
		if err := withRetry(ctx, w.log, w.retryDelays, "delete s3 object",
			func(c context.Context) error {
				return w.storage.Delete(c, key)
			},
		); err != nil {
			failed = append(failed, key)
			if firstErr == nil {
				firstErr = err
			}
			w.log.Warn("worker: delete object exhausted retries",
				"avatar_id", event.AvatarID,
				"s3_key", key,
				"message_id", messageID,
				"err", err)
			// Continue with remaining keys — best-effort cleanup.
		}
	}

	if firstErr != nil {
		status = statusFailed
		return fmt.Errorf("worker: delete event for avatar %s left %d/%d keys: %w",
			event.AvatarID, len(failed), len(event.S3Keys), firstErr)
	}

	w.log.Info("worker: delete event processed",
		"avatar_id", event.AvatarID,
		"key_count", len(event.S3Keys),
		"message_id", messageID)
	return nil
}
