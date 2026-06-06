package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/minio/minio-go/v7"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Exists reports whether an object exists under key. A missing object yields
// (false, nil); any other error is propagated.
func (m *MinIO) Exists(ctx context.Context, key string) (bool, error) {
	ctx, span := tracer.Start(ctx, "storage.Exists", trace.WithAttributes(
		attribute.String("s3.bucket", m.bucket),
		attribute.String("s3.key", key),
	))
	defer span.End()

	_, err := m.client.StatObject(ctx, m.bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}

	var er minio.ErrorResponse
	if errors.As(err, &er) && er.Code == "NoSuchKey" {
		return false, nil
	}
	return false, failSpan(span, fmt.Errorf("stat object %s: %w", key, err))
}
