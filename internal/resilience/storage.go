package resilience

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/sony/gobreaker/v2"

	"github.com/avc-dev/go-avatar-service/internal/storage"
)

// ObjectStorage is the slice of blob storage this package guards. The methods
// match what the avatar service and worker already call, so a *Storage drops in
// wherever the raw client was used.
type ObjectStorage interface {
	Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (*storage.DownloadResult, error)
	Delete(ctx context.Context, key string) error
}

// Storage adds a circuit breaker in front of an ObjectStorage. The three
// operations share one breaker — they all hit the same backend, so they share
// its health budget too.
type Storage struct {
	inner ObjectStorage
	cb    *gobreaker.CircuitBreaker[any]
}

// NewStorage wraps inner with a breaker named "object-storage".
func NewStorage(inner ObjectStorage, s Settings, log *slog.Logger) *Storage {
	return &Storage{inner: inner, cb: newBreaker[any]("object-storage", s, log)}
}

func (s *Storage) Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := s.cb.Execute(func() (any, error) {
		return nil, s.inner.Upload(ctx, key, body, size, contentType)
	})
	if err != nil {
		return fmt.Errorf("object storage upload %s: %w", key, err)
	}
	return nil
}

func (s *Storage) Download(ctx context.Context, key string) (*storage.DownloadResult, error) {
	res, err := s.cb.Execute(func() (any, error) {
		return s.inner.Download(ctx, key)
	})
	if err != nil {
		return nil, fmt.Errorf("object storage download %s: %w", key, err)
	}
	// Download always hands back a typed *storage.DownloadResult (maybe nil), so
	// the assertion is safe once err is nil.
	return res.(*storage.DownloadResult), nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	_, err := s.cb.Execute(func() (any, error) {
		return nil, s.inner.Delete(ctx, key)
	})
	if err != nil {
		return fmt.Errorf("object storage delete %s: %w", key, err)
	}
	return nil
}
