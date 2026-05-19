package broker

import (
	"context"
	"log/slog"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// PublishAvatarDeleted logs the event at DEBUG level and returns nil.
func (n *NoOp) PublishAvatarDeleted(_ context.Context, event domain.AvatarDeleteEvent) error {
	n.log.Debug("broker noop: publish",
		slog.String("event_type", "avatar.deleted"),
		slog.String("avatar_id", event.AvatarID.String()),
		slog.Int("s3_keys_count", len(event.S3Keys)),
	)
	return nil
}
