package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExists(t *testing.T) {
	ctx := context.Background()

	t.Run("true for existing key", func(t *testing.T) {
		cleanBucket(t)
		s := newMinIO(t)

		key := "exists/yes.txt"
		putRaw(t, key, "text/plain", []byte("here"))

		ok, err := s.Exists(ctx, key)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("false and no error for missing key", func(t *testing.T) {
		cleanBucket(t)
		s := newMinIO(t)

		ok, err := s.Exists(ctx, "exists/no.txt")
		require.NoError(t, err)
		require.False(t, ok)
	})
}
