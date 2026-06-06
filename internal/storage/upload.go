package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Upload streams body into the configured bucket under key. size is the exact
// byte length of body (use -1 for unknown, but prefer a known size — the
// MinIO SDK can optimise the upload). contentType is stored as the object's
// Content-Type metadata.
func (m *MinIO) Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	ctx, span := tracer.Start(ctx, "storage.Upload", trace.WithAttributes(
		attribute.String("s3.bucket", m.bucket),
		attribute.String("s3.key", key),
		attribute.Int64("s3.object_size", size),
	))
	defer span.End()

	_, err := m.client.PutObject(ctx, m.bucket, key, body, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return failSpan(span, fmt.Errorf("upload object %s: %w", key, err))
	}
	return nil
}
