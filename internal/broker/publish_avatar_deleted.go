package broker

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// PublishAvatarDeleted JSON-serialises event and publishes it to the project
// exchange with the avatar.deleted routing key. Messages are persistent and
// the avatar UUID is used as MessageId, mirroring PublishAvatarUploaded.
//
// pubMu is held for the whole PublishWithContext call — see RabbitMQ struct
// docs for the concurrency rationale.
func (r *RabbitMQ) PublishAvatarDeleted(ctx context.Context, event domain.AvatarDeleteEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("broker: marshal avatar.deleted for %s: %w", event.AvatarID, err)
	}
	r.pubMu.Lock()
	defer r.pubMu.Unlock()
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
