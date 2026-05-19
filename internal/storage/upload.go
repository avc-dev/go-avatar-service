package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
)

// Upload streams body into the configured bucket under key. size is the exact
// byte length of body (use -1 for unknown, but prefer a known size — the
// MinIO SDK can optimise the upload). contentType is stored as the object's
// Content-Type metadata.
func (m *MinIO) Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := m.client.PutObject(ctx, m.bucket, key, body, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("upload object %s: %w", key, err)
	}
	return nil
}
