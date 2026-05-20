// Package avatar implements the async worker for the avatar aggregate. It
// consumes broker events (uploaded, deleted) and orchestrates thumbnail
// generation and object-storage cleanup.
//
// Consumer-side dependency interfaces (Repository, Storage, ImageResizer) are
// declared here so the package owns its contract. The concrete adapters in
// internal/repository/avatar, internal/storage, and internal/imageproc satisfy
// them structurally — Go's duck typing keeps the layers decoupled.
//
// Idempotency strategy: every handler reads the current persistence state
// (processing_status) before doing work and short-circuits when the avatar is
// already completed, failed, or being processed by another worker. RabbitMQ
// guarantees at-least-once delivery; this status gate downgrades it to
// effectively-exactly-once processing without requiring a dedup table.
//
// Retry strategy: transient network errors against PG/S3 are retried in-process
// with exponential backoff (configurable per Worker via WithRetryDelays). When
// retries are exhausted, the handler returns an error and the broker consumer
// nacks the message to the dead-letter exchange for manual inspection.
package avatar

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	"github.com/avc-dev/go-avatar-service/internal/storage"
)

// ThumbnailSize describes one thumbnail variant to generate per uploaded avatar.
type ThumbnailSize struct {
	Name   string // map key + part of the S3 path, e.g. "100x100"
	Width  int
	Height int
}

// defaultThumbnailSizes is the project-wide thumbnail matrix mandated by the
// spec (section 1.4: "Создание миниатюр 100x100, 300x300").
var defaultThumbnailSizes = []ThumbnailSize{
	{Name: "100x100", Width: 100, Height: 100},
	{Name: "300x300", Width: 300, Height: 300},
}

// defaultRetryDelays is the production exponential backoff schedule. With 3
// delays the worker attempts each transient operation up to 4 times total
// (1 initial + 3 retries). Total worst-case wall time for retries: ~2.6s.
var defaultRetryDelays = []time.Duration{
	100 * time.Millisecond,
	500 * time.Millisecond,
	2 * time.Second,
}

// jpegQuality is the encoding quality used for every generated thumbnail. 85
// is the conventional sweet spot for image quality vs. file size.
const jpegQuality = 85

// Repository is the persistence dependency. Only the methods the worker
// actually calls are declared.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Avatar, error)
	UpdateProcessingStatus(ctx context.Context, id uuid.UUID, status domain.ProcessingStatus, thumbs map[string]string) error
}

// Storage is the object-storage dependency. Mirrors internal/storage.Storage
// but restricted to the worker's needs (no EnsureBucket / Exists).
type Storage interface {
	Download(ctx context.Context, key string) (*storage.DownloadResult, error)
	Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Delete(ctx context.Context, key string) error
}

// ImageResizer is the CPU-bound image processing dependency (decoded + resized
// + JPEG-encoded in one call). The concrete *imageproc.Resizer satisfies this.
type ImageResizer interface {
	ResizeToJPEG(ctx context.Context, src io.Reader, width, height, quality int) ([]byte, error)
}

// Worker orchestrates async processing of avatar lifecycle events. It is
// constructed once at process boot in cmd/worker and reused across all
// incoming messages.
type Worker struct {
	repo        Repository
	storage     Storage
	resizer     ImageResizer
	log         *slog.Logger
	retryDelays []time.Duration
	thumbSizes  []ThumbnailSize
}

// Option mutates a Worker during construction. Use the With* helpers; the
// type itself is exported only so external callers can store and pass option
// slices through their own configuration layer.
type Option func(*Worker)

// WithRetryDelays overrides the default exponential backoff schedule. Mainly
// intended for tests, which need short delays to keep run times low. Passing
// an empty slice disables retries entirely (single attempt per operation).
func WithRetryDelays(delays []time.Duration) Option {
	return func(w *Worker) { w.retryDelays = delays }
}

// WithThumbnailSizes overrides the thumbnail matrix. Used by tests to limit
// the number of variants generated; in production callers should rely on the
// default sizes that match the spec.
func WithThumbnailSizes(sizes []ThumbnailSize) Option {
	return func(w *Worker) { w.thumbSizes = sizes }
}

// New constructs a Worker.
func New(repo Repository, st Storage, resizer ImageResizer, log *slog.Logger, opts ...Option) *Worker {
	w := &Worker{
		repo:        repo,
		storage:     st,
		resizer:     resizer,
		log:         log,
		retryDelays: defaultRetryDelays,
		thumbSizes:  defaultThumbnailSizes,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}
