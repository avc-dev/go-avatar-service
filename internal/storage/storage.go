// Package storage provides an object-storage port (Storage) and a MinIO/S3
// implementation. Callers interact through the Storage interface so the
// underlying client can be swapped (e.g. for fakes in unit tests of higher
// layers); the MinIO impl is exposed for wiring in cmd/server.
package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

// tracer is the instrumentation scope for object-storage spans. Spans started
// here wrap each logical operation; the otelhttp-wrapped client transport adds
// a nested span for the actual S3 HTTP call underneath.
var tracer = otel.Tracer("github.com/avc-dev/go-avatar-service/internal/storage")

// Storage is the object-storage port. All methods take a context.Context first
// and propagate it to the underlying client so callers can cancel long-running
// transfers.
type Storage interface {
	Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (*DownloadResult, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	EnsureBucket(ctx context.Context) error
}

// DownloadResult carries the body and metadata returned by Storage.Download.
// Reader must be closed by the caller.
type DownloadResult struct {
	Reader      io.ReadCloser
	ContentType string
	Size        int64
}

// Config holds the constructor parameters for NewMinIO. cmd/server is
// expected to translate env config into this struct.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// MinIO is a Storage implementation backed by the MinIO Go SDK (S3-compatible).
type MinIO struct {
	client *minio.Client
	bucket string
	log    *slog.Logger
}

// Static interface assertion: MinIO must satisfy Storage.
var _ Storage = (*MinIO)(nil)

// NewMinIO constructs a MinIO client from cfg. It does not contact the server
// — callers should invoke EnsureBucket (or another method) to verify
// connectivity at startup.
func NewMinIO(cfg Config, log *slog.Logger) (*MinIO, error) {
	if log == nil {
		return nil, fmt.Errorf("storage: logger is required")
	}
	// Start from MinIO's own transport (keeps its connection-pool tuning) and
	// wrap it so every S3 HTTP call emits a client span nested under the
	// logical storage span. Reads the global propagator/provider — a no-op
	// until tracing is enabled.
	baseTransport, err := minio.DefaultTransport(cfg.UseSSL)
	if err != nil {
		return nil, fmt.Errorf("init minio transport: %w", err)
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:    cfg.UseSSL,
		Transport: otelhttp.NewTransport(baseTransport),
	})
	if err != nil {
		return nil, fmt.Errorf("init minio client: %w", err)
	}
	return &MinIO{
		client: client,
		bucket: cfg.Bucket,
		log:    log,
	}, nil
}
