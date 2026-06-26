// Package resilience puts circuit breakers in front of the external
// dependencies (object storage, broker). Once a backend starts failing the
// breaker opens and calls fail fast instead of stacking up on a dead endpoint;
// after a cooldown it lets a few probes through to see if things recovered.
//
// The decorators expose the same methods the service and worker already call,
// so wiring is just "wrap the client before passing it down". Health checks go
// straight to the raw client on purpose — when a dependency is sick is exactly
// when we want the real probe result, not a short-circuit.
package resilience

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/sony/gobreaker/v2"
)

// Settings tunes a circuit breaker. Zero fields fall back to sane defaults
// (see withDefaults), so a partially-filled struct is fine.
type Settings struct {
	// MaxRequests is how many probes are let through while half-open before the
	// breaker decides to close again or re-open.
	MaxRequests uint32
	// Interval resets the closed-state counts every so often. Zero means they
	// only reset when the breaker opens.
	Interval time.Duration
	// Timeout is how long the breaker stays open before it tries a probe.
	Timeout time.Duration
	// MinRequests is the smallest sample we'll judge the failure ratio on — keeps
	// one early error from tripping the breaker.
	MinRequests uint32
	// FailureRatio is the failures/total that trips the breaker, once at least
	// MinRequests calls are in the window.
	FailureRatio float64
}

func (s Settings) withDefaults() Settings {
	if s.MaxRequests == 0 {
		s.MaxRequests = 3
	}
	if s.Interval == 0 {
		s.Interval = 30 * time.Second
	}
	if s.Timeout == 0 {
		s.Timeout = 15 * time.Second
	}
	if s.MinRequests == 0 {
		s.MinRequests = 5
	}
	if s.FailureRatio <= 0 {
		s.FailureRatio = 0.6
	}
	return s
}

// newBreaker builds a typed gobreaker from s and logs every state change, so an
// opened breaker shows up in the logs (and, through the slog trace handler, next
// to the trace that tripped it).
func newBreaker[T any](name string, s Settings, log *slog.Logger) *gobreaker.CircuitBreaker[T] {
	s = s.withDefaults()
	return gobreaker.NewCircuitBreaker[T](gobreaker.Settings{
		Name:        name,
		MaxRequests: s.MaxRequests,
		Interval:    s.Interval,
		Timeout:     s.Timeout,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			ratio := float64(c.TotalFailures) / float64(c.Requests)
			return c.Requests >= s.MinRequests && ratio >= s.FailureRatio
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Warn("circuit breaker state change",
				"breaker", name, "from", from.String(), "to", to.String())
		},
		IsSuccessful: func(err error) bool {
			// A cancelled request is the caller giving up, not the backend
			// failing, so don't hold it against the breaker. A real timeout
			// (DeadlineExceeded) does count — that's usually the backend lagging.
			return err == nil || errors.Is(err, context.Canceled)
		},
	})
}
