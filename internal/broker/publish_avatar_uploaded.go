package broker

import (
	"context"
	"log/slog"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// PublishAvatarUploaded logs the event at DEBUG level and returns nil.
func (n *NoOp) PublishAvatarUploaded(_ context.Context, event domain.AvatarUploadEvent) error {
	n.log.Debug("broker noop: publish",
		slog.String("event_type", "avatar.uploaded"),
		slog.String("avatar_id", event.AvatarID.String()),
		slog.String("user_id", event.UserID.String()),
		slog.String("s3_key", event.S3Key),
	)
	return nil
}
