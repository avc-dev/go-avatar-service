package storage

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/minio/minio-go/v7"
)

// EnsureBucket creates the configured bucket if it does not already exist.
// Safe to call repeatedly. The region is required by the SDK; MinIO ignores
// it but a real S3 backend would honour it.
func (m *MinIO) EnsureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("check bucket %s: %w", m.bucket, err)
	}
	if exists {
		return nil
	}
	if err := m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		return fmt.Errorf("create bucket %s: %w", m.bucket, err)
	}
	m.log.Info("storage: bucket created", slog.String("bucket", m.bucket))
	return nil
}
