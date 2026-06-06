package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HTTP          HTTPConfig
	Log           LogConfig
	Postgres      PostgresConfig
	MinIO         MinIOConfig
	RabbitMQ      RabbitMQConfig
	Security      SecurityConfig
	Observability ObservabilityConfig
}

type HTTPConfig struct {
	Port              string        `env:"HTTP_PORT" envDefault:"8080"`
	ReadTimeout       time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"5s"`
	ReadHeaderTimeout time.Duration `env:"HTTP_READ_HEADER_TIMEOUT" envDefault:"2s"`
	WriteTimeout      time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"10s"`
	IdleTimeout       time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"120s"`
	ShutdownTimeout   time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" envDefault:"10s"`
	MaxUploadBytes    int64         `env:"HTTP_MAX_UPLOAD_BYTES" envDefault:"10485760"`
}

type LogConfig struct {
	Level  string `env:"LOG_LEVEL" envDefault:"info"`
	Format string `env:"LOG_FORMAT" envDefault:"json"`
}

type PostgresConfig struct {
	DSN string `env:"POSTGRES_DSN,required"`
}

type MinIOConfig struct {
	Endpoint  string `env:"MINIO_ENDPOINT" envDefault:"localhost:9000"`
	AccessKey string `env:"MINIO_ACCESS_KEY,required"`
	SecretKey string `env:"MINIO_SECRET_KEY,required"`
	Bucket    string `env:"MINIO_BUCKET" envDefault:"avatars"`
	UseSSL    bool   `env:"MINIO_USE_SSL" envDefault:"false"`
}

type RabbitMQConfig struct {
	URL      string `env:"RABBITMQ_URL,required"`
	Exchange string `env:"RABBITMQ_EXCHANGE" envDefault:"avatars.exchange"`
}

// SecurityConfig groups the knobs for the defensive middleware layer (CORS,
// rate limiting). All fields have safe defaults so the service can be brought
// up without setting any of these explicitly, but a real deployment should
// override CORSAllowedOrigins to something other than empty if it expects
// cross-origin traffic.
//
// Rate-limit values are interpreted as requests-per-minute; a value <=0
// disables the corresponding limiter (useful for e2e tests).
type SecurityConfig struct {
	// CORSAllowedOrigins is a comma-separated list of origins permitted in
	// CORS preflight. Empty (default) denies all cross-origin requests, which
	// is the right baseline for an embeddable avatar service that hasn't yet
	// declared its consumers.
	CORSAllowedOrigins   []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`
	RateLimitReadPerMin  int      `env:"RATE_LIMIT_READ_PER_MIN" envDefault:"100"`
	RateLimitWritePerMin int      `env:"RATE_LIMIT_WRITE_PER_MIN" envDefault:"10"`
}

// ObservabilityConfig groups the OpenTelemetry tracing knobs. Tracing is on by
// default but degrades gracefully: with Enabled=false (or no collector
// reachable) the global tracer provider stays the OTel no-op, so the otelhttp /
// otelpgx wrappers create zero-cost spans and the rest of the code is unaware.
//
// ServiceName is intentionally NOT an env field: the server and worker share
// the same environment, so each binary passes its own constant name to
// observability.Init — that keeps them distinct nodes in the Jaeger dependency
// graph (gophprofile-server -> rabbitmq -> gophprofile-worker).
type ObservabilityConfig struct {
	Enabled bool `env:"OTEL_ENABLED" envDefault:"true"`
	// OTLPEndpoint is the host:port of the OTLP/gRPC collector. Default targets
	// the collector service inside compose; override to localhost:4317 for a
	// process running outside the compose network.
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:"otel-collector:4317"`
	// Environment populates the deployment.environment resource attribute so a
	// single Jaeger/Grafana can distinguish local vs other deployments.
	Environment string `env:"OTEL_ENVIRONMENT" envDefault:"local"`
	// SampleRatio is the head-sampling probability for root spans (parent
	// decisions are always respected). 1.0 traces everything — the right
	// default for dev; lower it under real load.
	SampleRatio float64 `env:"OTEL_TRACES_SAMPLER_RATIO" envDefault:"1.0"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
