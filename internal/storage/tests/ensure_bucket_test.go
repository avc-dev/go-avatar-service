package storage_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/storage"
)

func TestEnsureBucket(t *testing.T) {
	ctx := context.Background()

	t.Run("creates bucket when missing", func(t *testing.T) {
		bucket := "ensure-" + uuid.Must(uuid.NewV7()).String()
		s := storageWithBucket(t, bucket)

		require.NoError(t, s.EnsureBucket(ctx))

		exists, err := testAdmin.BucketExists(ctx, bucket)
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run("idempotent on existing bucket", func(t *testing.T) {
		bucket := "ensure-" + uuid.Must(uuid.NewV7()).String()
		s := storageWithBucket(t, bucket)

		require.NoError(t, s.EnsureBucket(ctx))
		// second call must not error
		require.NoError(t, s.EnsureBucket(ctx))
	})
}

// storageWithBucket builds a *storage.MinIO bound to an ad-hoc bucket name,
// reusing the shared container's endpoint and credentials. The bucket is
// scheduled for removal via t.Cleanup.
func storageWithBucket(t *testing.T, bucket string) *storage.MinIO {
	t.Helper()

	endpoint, err := testContainer.PortEndpoint(context.Background(), "9000/tcp", "")
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := storage.NewMinIO(storage.Config{
		Endpoint:  endpoint,
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    bucket,
		UseSSL:    false,
	}, logger)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = testAdmin.RemoveBucket(context.Background(), bucket)
	})

	return s
}
