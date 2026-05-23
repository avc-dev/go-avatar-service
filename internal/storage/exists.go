package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/minio/minio-go/v7"
)

// Exists reports whether an object exists under key. A missing object yields
// (false, nil); any other error is propagated.
func (m *MinIO) Exists(ctx context.Context, key string) (bool, error) {
	_, err := m.client.StatObject(ctx, m.bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}

	var er minio.ErrorResponse
	if errors.As(err, &er) && er.Code == "NoSuchKey" {
		return false, nil
	}
	return false, fmt.Errorf("stat object %s: %w", key, err)
}
