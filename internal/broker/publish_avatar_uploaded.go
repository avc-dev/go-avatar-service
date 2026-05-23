package broker

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// PublishAvatarUploaded JSON-serialises event and publishes it to the
// project exchange with the avatar.uploaded routing key. Messages are
// persistent so the queue survives broker restarts; the avatar UUID doubles
// as MessageId so consumer logs can correlate by it.
//
// pubMu is held for the whole PublishWithContext call — see RabbitMQ struct
// docs for the concurrency rationale.
func (r *RabbitMQ) PublishAvatarUploaded(ctx context.Context, event domain.AvatarUploadEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("broker: marshal avatar.uploaded for %s: %w", event.AvatarID, err)
	}
	r.pubMu.Lock()
	defer r.pubMu.Unlock()
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
