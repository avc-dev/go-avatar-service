package storage

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
)

// Delete removes an object from the configured bucket. Per S3 semantics the
// operation is idempotent — deleting a non-existent key is not an error.
func (m *MinIO) Delete(ctx context.Context, key string) error {
	err := m.client.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("delete object %s: %w", key, err)
	}
	return nil
}
