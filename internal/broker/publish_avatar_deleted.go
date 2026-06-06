package broker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// PublishAvatarDeleted JSON-serialises event and publishes it to the project
// exchange with the avatar.deleted routing key. Messages are persistent and
// the avatar UUID is used as MessageId, mirroring PublishAvatarUploaded. Trace
// context is injected into the headers by the shared publish helper.
func (r *RabbitMQ) PublishAvatarDeleted(ctx context.Context, event domain.AvatarDeleteEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("broker: marshal avatar.deleted for %s: %w", event.AvatarID, err)
	}
	return r.publish(ctx, RoutingDeleted, "avatar.deleted", event.AvatarID.String(), body)
}
