// Package broker defines the Publisher port for outbound domain events and
// provides a RabbitMQ-backed implementation. See rabbitmq.go for the wire-level
// publisher/consumer and topology.go for the exchange/queue/routing-key
// constants shared between server and worker.
package broker

import (
	"context"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// Publisher is the outbound port for domain events. Implementations must be
// safe for concurrent use; callers may invoke methods from multiple goroutines.
type Publisher interface {
	PublishAvatarUploaded(ctx context.Context, event domain.AvatarUploadEvent) error
	PublishAvatarDeleted(ctx context.Context, event domain.AvatarDeleteEvent) error
}
