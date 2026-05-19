package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDelete(t *testing.T) {
	ctx := context.Background()

	t.Run("removes existing object", func(t *testing.T) {
		cleanBucket(t)
		s := newMinIO(t)

		key := "delete/exists.txt"
		putRaw(t, key, "text/plain", []byte("bye"))

		exists, err := s.Exists(ctx, key)
		require.NoError(t, err)
		require.True(t, exists)

		require.NoError(t, s.Delete(ctx, key))

		exists, err = s.Exists(ctx, key)
		require.NoError(t, err)
		require.False(t, exists, "object must be gone after Delete")
	})

	// S3 semantics: delete is idempotent. Removing a non-existent key must
	// return nil, NOT an error.
	t.Run("missing key returns nil", func(t *testing.T) {
		cleanBucket(t)
		s := newMinIO(t)

		require.NoError(t, s.Delete(ctx, "delete/nope.txt"))
	})
}
