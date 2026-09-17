package resilience

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sony/gobreaker/v2"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// EventPublisher is the broker surface this package guards — the same methods
// the avatar service publishes through.
type EventPublisher interface {
	PublishAvatarUploaded(ctx context.Context, event domain.AvatarUploadEvent) error
	PublishAvatarDeleted(ctx context.Context, event domain.AvatarDeleteEvent) error
}

// Publisher puts a circuit breaker in front of an EventPublisher. Publish
// failures are already non-fatal in the upload saga (we log and leave the avatar
// pending), but the breaker keeps a flapping broker from adding latency to every
// request that follows.
type Publisher struct {
	inner EventPublisher
	cb    *gobreaker.CircuitBreaker[any]
}

// NewPublisher wraps inner with a breaker named "event-publisher".
func NewPublisher(inner EventPublisher, s Settings, log *slog.Logger) *Publisher {
	return &Publisher{inner: inner, cb: newBreaker[any]("event-publisher", s, log)}
}

func (p *Publisher) PublishAvatarUploaded(ctx context.Context, event domain.AvatarUploadEvent) error {
	_, err := p.cb.Execute(func() (any, error) {
		return nil, p.inner.PublishAvatarUploaded(ctx, event)
	})
	if err != nil {
		return fmt.Errorf("publish avatar uploaded %s: %w", event.AvatarID, err)
	}
	return nil
}

func (p *Publisher) PublishAvatarDeleted(ctx context.Context, event domain.AvatarDeleteEvent) error {
	_, err := p.cb.Execute(func() (any, error) {
		return nil, p.inner.PublishAvatarDeleted(ctx, event)
	})
	if err != nil {
		return fmt.Errorf("publish avatar deleted %s: %w", event.AvatarID, err)
	}
	return nil
}
