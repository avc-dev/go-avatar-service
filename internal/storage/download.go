package storage

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Download fetches an object by key. It performs a Stat first so size and
// content-type are populated and so a missing-key error surfaces immediately
// (otherwise GetObject is lazy and would only fail on first Read). The caller
// owns the returned Reader and must Close it.
func (m *MinIO) Download(ctx context.Context, key string) (*DownloadResult, error) {
	ctx, span := tracer.Start(ctx, "storage.Download", trace.WithAttributes(
		attribute.String("s3.bucket", m.bucket),
		attribute.String("s3.key", key),
	))
	defer span.End()

	obj, err := m.client.GetObject(ctx, m.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, failSpan(span, fmt.Errorf("get object %s: %w", key, err))
	}

	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, failSpan(span, fmt.Errorf("stat object %s: %w", key, err))
	}
	span.SetAttributes(attribute.Int64("s3.object_size", info.Size))

	return &DownloadResult{
		Reader:      obj,
		ContentType: info.ContentType,
		Size:        info.Size,
	}, nil
}
