package storage

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Delete removes an object from the configured bucket. Per S3 semantics the
// operation is idempotent — deleting a non-existent key is not an error.
func (m *MinIO) Delete(ctx context.Context, key string) error {
	ctx, span := tracer.Start(ctx, "storage.Delete", trace.WithAttributes(
		attribute.String("s3.bucket", m.bucket),
		attribute.String("s3.key", key),
	))
	defer span.End()

	err := m.client.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return failSpan(span, fmt.Errorf("delete object %s: %w", key, err))
	}
	return nil
}
