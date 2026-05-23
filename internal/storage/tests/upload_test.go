package storage_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpload(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name        string
		key         string
		payload     []byte
		contentType string
	}{
		{
			name:        "small payload round-trips",
			key:         "uploads/small.txt",
			payload:     []byte("hello, minio"),
			contentType: "text/plain",
		},
		{
			name:        "empty payload allowed",
			key:         "uploads/empty.bin",
			payload:     []byte{},
			contentType: "application/octet-stream",
		},
		{
			name:        "binary payload round-trips byte-identical",
			key:         "uploads/binary.bin",
			payload:     []byte{0x00, 0xff, 0x10, 0x7f, 0x80, 0xab, 0xcd, 0xef},
			contentType: "application/octet-stream",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanBucket(t)
			s := newMinIO(t)

			err := s.Upload(ctx, tc.key, bytes.NewReader(tc.payload), int64(len(tc.payload)), tc.contentType)
			require.NoError(t, err)

			exists, err := s.Exists(ctx, tc.key)
			require.NoError(t, err)
			require.True(t, exists, "uploaded object should exist")

			res, err := s.Download(ctx, tc.key)
			require.NoError(t, err)
			defer res.Reader.Close()

			require.Equal(t, int64(len(tc.payload)), res.Size, "size must match input size")

			got, err := io.ReadAll(res.Reader)
			require.NoError(t, err)
			require.Equal(t, tc.payload, got, "downloaded body must be byte-identical to uploaded body")
		})
	}
}
