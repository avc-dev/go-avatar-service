package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"

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

// PublishAvatarDeleted JSON-serialises event and publishes it to the project
// exchange with the avatar.deleted routing key. Messages are persistent and
// the avatar UUID is used as MessageId, mirroring PublishAvatarUploaded.
func (r *RabbitMQ) PublishAvatarDeleted(ctx context.Context, event domain.AvatarDeleteEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("broker: marshal avatar.deleted for %s: %w", event.AvatarID, err)
	}
	if err := r.pubCh.PublishWithContext(
		ctx,
		ExchangeName,
		RoutingDeleted,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    event.AvatarID.String(),
			Body:         body,
		},
	); err != nil {
		return fmt.Errorf("broker: publish avatar.deleted for %s: %w", event.AvatarID, err)
	}
	return nil
}
