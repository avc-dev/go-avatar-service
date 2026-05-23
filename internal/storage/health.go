package storage

import (
	"context"
	"fmt"
)

// Check verifies connectivity to the object-storage backend and that the
// configured bucket exists. It satisfies handlers.HealthChecker structurally
// — the storage package does NOT import handlers; only the method shape is
// required.
//
// The probe is a single BucketExists call: cheap (one HEAD), exercises both
// the network path and credentials, and surfaces the most common misconfig
// (bucket missing) as a distinct error rather than as a generic 401/500.
func (m *MinIO) Check(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("minio bucket %s: %w", m.bucket, err)
	}
	if !exists {
		return fmt.Errorf("minio bucket %s does not exist", m.bucket)
	}
	return nil
}
