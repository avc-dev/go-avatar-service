package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HTTP     HTTPConfig
	Log      LogConfig
	Postgres PostgresConfig
	MinIO    MinIOConfig
	RabbitMQ RabbitMQConfig
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

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
