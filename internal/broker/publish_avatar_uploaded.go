package broker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// PublishAvatarUploaded JSON-serialises event and publishes it to the
// project exchange with the avatar.uploaded routing key. Messages are
// persistent so the queue survives broker restarts; the avatar UUID doubles
// as MessageId so consumer logs can correlate by it. Trace context is injected
// into the headers by the shared publish helper.
func (r *RabbitMQ) PublishAvatarUploaded(ctx context.Context, event domain.AvatarUploadEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("broker: marshal avatar.uploaded for %s: %w", event.AvatarID, err)
	}
	return r.publish(ctx, RoutingUploaded, "avatar.uploaded", event.AvatarID.String(), body)
}
