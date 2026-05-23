package broker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Static interface assertion: *RabbitMQ must satisfy Publisher.
var _ Publisher = (*RabbitMQ)(nil)

// RabbitMQ is a Publisher implementation backed by a real RabbitMQ broker
// (via streadway/rabbitmq's maintained amqp091-go client). It additionally
// exposes Consume for the worker side.
//
// The struct holds a long-lived connection plus a dedicated publisher
// channel. Consumers receive their own freshly-opened channel (see Consume)
// to keep publisher and consumer flow control independent.
//
// pubMu serialises access to pubCh. amqp091-go explicitly forbids concurrent
// use of a *Channel — under load, two goroutines publishing in parallel
// interleave AMQP frames and the broker drops the connection. The publisher
// channel is hot-shared across all HTTP handlers, so the mutex is the
// minimum-fuss correctness fix. (An alternative would be a small channel
// pool, but the lock is cheap and publishes are short.)
type RabbitMQ struct {
	conn  *amqp.Connection
	pubCh *amqp.Channel
	pubMu sync.Mutex
	log   *slog.Logger
}

// NewRabbitMQ dials url, declares the project topology (exchanges, queues,
// bindings, DLX) and returns a ready-to-use RabbitMQ. On any error during
// setup all opened resources are released before the error is returned.
//
// The topology is idempotent: redeclaring an existing entity with matching
// parameters is a no-op, so multiple processes (server + worker) calling
// NewRabbitMQ concurrently is safe.
func NewRabbitMQ(url string, log *slog.Logger) (*RabbitMQ, error) {
	if log == nil {
		return nil, fmt.Errorf("broker: logger is required")
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("broker: dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("broker: open channel: %w", err)
	}

	if err := declareTopology(ch); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	return &RabbitMQ{
		conn:  conn,
		pubCh: ch,
		log:   log,
	}, nil
}

// declareTopology is the imperative form of topology.go: it materialises the
// declared constants on the broker. Kept package-private because callers
// outside the constructor should never need to invoke it.
func declareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(
		ExchangeName, ExchangeType,
		true,  // durable
		false, // autoDelete
		false, // internal
		false, // noWait
		nil,
	); err != nil {
		return fmt.Errorf("broker: declare exchange %s: %w", ExchangeName, err)
	}

	if err := ch.ExchangeDeclare(
		DLXName, "fanout",
		true, false, false, false,
		nil,
	); err != nil {
		return fmt.Errorf("broker: declare exchange %s: %w", DLXName, err)
	}

	if _, err := ch.QueueDeclare(
		QueueDead,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,
	); err != nil {
		return fmt.Errorf("broker: declare queue %s: %w", QueueDead, err)
	}

	if err := ch.QueueBind(QueueDead, "", DLXName, false, nil); err != nil {
		return fmt.Errorf("broker: bind queue %s -> %s: %w", QueueDead, DLXName, err)
	}

	workQueues := []struct {
		queue, routingKey string
	}{
		{QueueUploaded, RoutingUploaded},
		{QueueDeleted, RoutingDeleted},
	}
	for _, q := range workQueues {
		if _, err := ch.QueueDeclare(
			q.queue,
			true, false, false, false,
			amqp.Table{
				"x-dead-letter-exchange": DLXName,
			},
		); err != nil {
			return fmt.Errorf("broker: declare queue %s: %w", q.queue, err)
		}
		if err := ch.QueueBind(q.queue, q.routingKey, ExchangeName, false, nil); err != nil {
			return fmt.Errorf("broker: bind queue %s -> %s (%s): %w", q.queue, ExchangeName, q.routingKey, err)
		}
	}

	return nil
}

// Close releases the publisher channel and the underlying connection. Both
// are closed unconditionally; the first non-nil error encountered is
// returned so the caller still learns about a failure without leaking
// resources.
//
// We acquire pubMu briefly before closing the channel so an in-flight
// PublishWithContext finishes cleanly. In a well-behaved shutdown the HTTP
// server has already drained, but layering the lock here keeps the invariant
// "pubCh is closed only when no publish is running" easy to reason about.
func (r *RabbitMQ) Close() error {
	var firstErr error
	if r.pubCh != nil {
		r.pubMu.Lock()
		err := r.pubCh.Close()
		r.pubMu.Unlock()
		if err != nil && !errors.Is(err, amqp.ErrClosed) {
			firstErr = fmt.Errorf("broker: close publisher channel: %w", err)
		}
	}
	if r.conn != nil {
		if err := r.conn.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			if firstErr == nil {
				firstErr = fmt.Errorf("broker: close connection: %w", err)
			}
		}
	}
	return firstErr
}

// Check reports broker health for the /health endpoint. The check is a
// synchronous in-process lookup against the cached connection state; ctx is
// accepted to satisfy the handlers.HealthChecker contract but unused.
func (r *RabbitMQ) Check(_ context.Context) error {
	if r.conn == nil || r.conn.IsClosed() {
		return fmt.Errorf("broker: connection is closed")
	}
	return nil
}
