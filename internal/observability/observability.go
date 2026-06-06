// Package observability wires the OpenTelemetry tracing pipeline for the
// service. It deliberately exposes a single entry point (Init) that installs a
// global TracerProvider and text-map propagator, so the rest of the codebase
// can stay OTel-agnostic: instrumentation wrappers (otelhttp, otelpgx, the
// broker carrier) all read the globals rather than receiving a provider
// threaded through every constructor.
//
// Metrics and logs are handled elsewhere (Prometheus client + slog); this
// package owns traces only.
package observability

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/avc-dev/go-avatar-service/internal/config"
)

// ShutdownFunc flushes and releases the tracing pipeline. It is always safe to
// call (even when tracing was disabled) and should be deferred from main with a
// bounded context so a slow collector cannot hang shutdown.
type ShutdownFunc func(context.Context) error

// noopShutdown is returned whenever there is nothing to tear down.
func noopShutdown(context.Context) error { return nil }

// Init installs the global tracer provider and propagator for serviceName.
//
// When cfg.Enabled is false it sets only the propagator (so inbound/outbound
// trace context still flows for would-be downstream collectors) and returns a
// no-op shutdown, leaving the global provider as OTel's built-in no-op. That
// keeps unit tests and collector-less local runs quiet and cheap while the
// instrumentation wrappers stay wired unconditionally.
//
// The OTLP/gRPC exporter connects lazily, so Init does not fail or block if the
// collector is not yet up — spans are buffered by the batch processor and
// dropped on overflow rather than blocking request handling.
func Init(ctx context.Context, cfg config.ObservabilityConfig, serviceName string, log *slog.Logger) (ShutdownFunc, error) {
	// Propagator is installed even when tracing is disabled: it is pure
	// header (de)serialisation with no backend dependency, and keeping it on
	// means the broker carrier and otelhttp behave identically in both modes.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if !cfg.Enabled {
		log.Info("tracing disabled; global provider stays no-op", "service", serviceName)
		return noopShutdown, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: create otlp exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			attribute.String("service.name", serviceName),
			attribute.String("service.version", serviceVersion()),
			attribute.String("deployment.environment", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: build resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		// ParentBased keeps a trace intact end-to-end: once the server samples
		// a request, the propagated decision forces the worker to record it
		// too, so distributed traces never come out half-sampled.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)
	otel.SetTracerProvider(tp)

	log.Info("tracing enabled",
		"service", serviceName,
		"endpoint", cfg.OTLPEndpoint,
		"environment", cfg.Environment,
		"sample_ratio", cfg.SampleRatio,
	)

	return func(shutdownCtx context.Context) error {
		// Bound the flush so a dead collector cannot stall process exit beyond
		// the caller's own deadline.
		if _, ok := shutdownCtx.Deadline(); !ok {
			var cancel context.CancelFunc
			shutdownCtx, cancel = context.WithTimeout(shutdownCtx, 5*time.Second)
			defer cancel()
		}
		if err := tp.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("observability: shutdown tracer provider: %w", err)
		}
		return nil
	}, nil
}

// serviceVersion reports the module version baked into the binary (set by the
// Go toolchain for `go install`/module builds), falling back to "dev" for plain
// `go build`/`go run` where no version is embedded.
func serviceVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
