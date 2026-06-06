package broker

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Handler processes a single delivered message body. The messageID matches
// the value the publisher set in amqp.Publishing.MessageId (the avatar UUID
// in this project). Returning a non-nil error causes Consume to nack the
// message without requeueing it, which routes it to the DLX.
type Handler func(ctx context.Context, body []byte, messageID string) error

// Consume blocks while delivering messages from queueName to handler. It
// opens a dedicated channel (so consumer flow control is independent from
// the publisher), sets prefetch=1 to limit in-flight work, and acknowledges
// each message manually based on the handler's return value.
//
// Returns:
//   - nil is never returned; Consume runs until ctx is cancelled or the
//     delivery channel closes (broker disconnect).
//   - ctx.Err() (wrapped) on cancellation.
//   - a wrapped error if the delivery channel closes — the caller (worker
//     main loop) is expected to decide whether to retry or exit.
//
// In-process retry is deliberately the handler's responsibility; this layer
// only owns the wire-level ack/nack contract.
func (r *RabbitMQ) Consume(ctx context.Context, queueName string, handler Handler) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("broker: open consumer channel for %s: %w", queueName, err)
	}

	if err := ch.Qos(1, 0, false); err != nil {
		_ = ch.Close()
		return fmt.Errorf("broker: set qos for %s: %w", queueName, err)
	}

	deliveries, err := ch.Consume(
		queueName,
		"",    // consumer tag (server-generated)
		false, // autoAck — we ack manually
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,
	)
	if err != nil {
		_ = ch.Close()
		return fmt.Errorf("broker: start consume %s: %w", queueName, err)
	}

	for {
		select {
		case <-ctx.Done():
			// Best-effort cleanup; if Close fails the connection is likely
			// already torn down and there is nothing useful to report.
			_ = ch.Close()
			return fmt.Errorf("broker: consume %s cancelled: %w", queueName, ctx.Err())
		case d, ok := <-deliveries:
			if !ok {
				// Channel closed by the broker (or by Close on shutdown).
				// Let the caller decide on retry.
				return fmt.Errorf("broker: consume %s: delivery channel closed", queueName)
			}
			r.deliver(ctx, queueName, d, handler)
		}
	}
}

// deliver processes one message: it continues the publisher's trace (extracted
// from the message headers) under a consumer span, runs the handler, and
// ack/nacks based on the outcome. Pulling this out of the Consume loop gives the
// span a clean defer-bounded lifetime instead of a per-iteration close.
//
// ctx carries the worker's cancellation; Extract layers the remote span context
// on top so the consumer span links to the producer span (same trace_id) while
// still being cancellable on shutdown.
func (r *RabbitMQ) deliver(ctx context.Context, queueName string, d amqp.Delivery, handler Handler) {
	msgCtx := otel.GetTextMapPropagator().Extract(ctx, amqpHeaderCarrier(d.Headers))
	msgCtx, span := tracer.Start(msgCtx, "rabbitmq.process "+queueName,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.source.name", queueName),
			attribute.String("messaging.message.id", d.MessageId),
			attribute.String("messaging.operation", "process"),
		),
	)
	defer span.End()

	if err := handler(msgCtx, d.Body, d.MessageId); err != nil {
		_ = failSpan(span, err)
		if nackErr := d.Nack(false /*multiple*/, false /*requeue*/); nackErr != nil {
			r.log.ErrorContext(msgCtx, "broker: nack failed",
				"queue", queueName,
				"message_id", d.MessageId,
				"err", nackErr,
			)
		}
		r.log.WarnContext(msgCtx, "broker: handler returned error, message routed to DLX",
			"queue", queueName,
			"message_id", d.MessageId,
			"err", err,
		)
		return
	}
	if err := d.Ack(false); err != nil {
		r.log.ErrorContext(msgCtx, "broker: ack failed",
			"queue", queueName,
			"message_id", d.MessageId,
			"err", err,
		)
	}
}
