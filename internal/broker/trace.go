package broker

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
)

// tracer is the instrumentation scope for broker producer/consumer spans.
var tracer = otel.Tracer("github.com/avc-dev/go-avatar-service/internal/broker")

// amqpHeaderCarrier adapts an amqp.Table to propagation.TextMapCarrier so the
// global propagator can inject the trace context into outgoing message headers
// and extract it on the consumer side. This is the seam that joins the
// publisher's trace to the worker's: without it, processing a message would
// start a brand-new, disconnected trace.
//
// amqp header values are typed (interface{}); trace context is always written
// as strings, so Get type-asserts to string and ignores anything else.
type amqpHeaderCarrier amqp.Table

func (c amqpHeaderCarrier) Get(key string) string {
	if v, ok := c[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (c amqpHeaderCarrier) Set(key, value string) {
	c[key] = value
}

func (c amqpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
