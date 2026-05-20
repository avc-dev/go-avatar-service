package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"

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

// PublishAvatarUploaded JSON-serialises event and publishes it to the
// project exchange with the avatar.uploaded routing key. Messages are
// persistent so the queue survives broker restarts; the avatar UUID doubles
// as MessageId so downstream observability can correlate by it.
func (r *RabbitMQ) PublishAvatarUploaded(ctx context.Context, event domain.AvatarUploadEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("broker: marshal avatar.uploaded for %s: %w", event.AvatarID, err)
	}
	if err := r.pubCh.PublishWithContext(
		ctx,
		ExchangeName,
		RoutingUploaded,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    event.AvatarID.String(),
			Body:         body,
		},
	); err != nil {
		return fmt.Errorf("broker: publish avatar.uploaded for %s: %w", event.AvatarID, err)
	}
	return nil
}
