package storage_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDownload(t *testing.T) {
	ctx := t.Context()

	t.Run("returns content-type and size for existing object", func(t *testing.T) {
		cleanBucket(t)
		s := newMinIO(t)

		key := "downloads/exists.txt"
		payload := []byte("download me")
		putRaw(t, key, "text/plain", payload)

		res, err := s.Download(ctx, key)
		require.NoError(t, err)
		defer res.Reader.Close()

		require.Equal(t, "text/plain", res.ContentType)
		require.Equal(t, int64(len(payload)), res.Size)

		body, err := io.ReadAll(res.Reader)
		require.NoError(t, err)
		require.Equal(t, payload, body)
	})

	// Missing key returns an error. We do NOT introduce a custom ErrNotFound
	// in this layer — callers that need to discriminate can inspect the
	// wrapped minio.ErrorResponse via errors.As.
	t.Run("returns error for missing key", func(t *testing.T) {
		cleanBucket(t)
		s := newMinIO(t)

		_, err := s.Download(ctx, "downloads/nope.txt")
		require.Error(t, err)
	})
}
