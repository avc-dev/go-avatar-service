package avatar_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
	"github.com/avc-dev/go-avatar-service/internal/services/avatar"
	"github.com/avc-dev/go-avatar-service/internal/storage"
)

func TestDownloadOriginal(t *testing.T) {
	ctx := context.Background()
	id := uuid.Must(uuid.NewV7())

	t.Run("happy: returns body and metadata", func(t *testing.T) {
		svc, repo, st, _ := newService(t)
		meta := &domain.Avatar{ID: id, S3Key: "originals/x.jpg", MIMEType: "image/jpeg"}
		repo.EXPECT().GetByID(ctx, id).Return(meta, nil)

		body := io.NopCloser(strings.NewReader("binary data"))
		st.EXPECT().Download(ctx, meta.S3Key).Return(&storage.DownloadResult{
			Reader:      body,
			ContentType: "image/jpeg",
			Size:        11,
		}, nil)

		result, err := svc.DownloadOriginal(ctx, id)
		require.NoError(t, err)
		require.True(t, result.Reader == body, "service must pass the reader through unchanged")
		require.Equal(t, "image/jpeg", result.ContentType)
		require.Equal(t, int64(11), result.Size)
		require.Same(t, meta, result.Avatar)
	})

	t.Run("metadata not found: storage is not consulted", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		repo.EXPECT().GetByID(ctx, id).Return(nil, repoavatar.ErrNotFound)

		_, err := svc.DownloadOriginal(ctx, id)
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})

	t.Run("storage failure propagates wrapped", func(t *testing.T) {
		svc, repo, st, _ := newService(t)
		meta := &domain.Avatar{ID: id, S3Key: "originals/x.jpg"}
		repo.EXPECT().GetByID(ctx, id).Return(meta, nil)

		rawErr := errors.New("S3 down")
		st.EXPECT().Download(ctx, meta.S3Key).Return(nil, rawErr)

		_, err := svc.DownloadOriginal(ctx, id)
		require.Error(t, err)
		require.NotErrorIs(t, err, avatar.ErrNotFound)
		require.ErrorIs(t, err, rawErr)
	})
}
