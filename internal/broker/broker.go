// Package broker defines the Publisher port for outbound domain events and
// provides a NoOp implementation suitable for local development and tests.
// A real RabbitMQ-backed publisher is introduced in a later iteration.
package broker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// Publisher is the outbound port for domain events. Implementations must be
// safe for concurrent use; callers may invoke methods from multiple goroutines.
type Publisher interface {
	PublishAvatarUploaded(ctx context.Context, event domain.AvatarUploadEvent) error
	PublishAvatarDeleted(ctx context.Context, event domain.AvatarDeleteEvent) error
}

// NoOp is a Publisher that logs the event at DEBUG and returns nil. Useful
// during early iterations when the broker is not yet wired, and as a default
// in tests of higher layers that do not assert on outbound events.
type NoOp struct {
	log *slog.Logger
}

// Static interface assertion: NoOp must satisfy Publisher.
var _ Publisher = (*NoOp)(nil)

// NewNoOp constructs a NoOp publisher. A non-nil logger is required.
func NewNoOp(log *slog.Logger) *NoOp {
	if log == nil {
		// Panic here rather than return an error: the constructor is called
		// at startup and a nil logger is a programmer error, not a runtime
		// condition.
		panic(fmt.Errorf("broker: logger is required"))
	}
	return &NoOp{log: log}
}
