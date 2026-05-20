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

func TestDownloadThumbnail(t *testing.T) {
	ctx := context.Background()
	id := uuid.Must(uuid.NewV7())

	avatarWithThumbs := func() *domain.Avatar {
		return &domain.Avatar{
			ID:    id,
			S3Key: "originals/" + id.String() + ".jpg",
			ThumbnailS3Keys: map[string]string{
				"100x100": "thumbnails/" + id.String() + "/100x100.jpg",
				"300x300": "thumbnails/" + id.String() + "/300x300.jpg",
			},
		}
	}

	t.Run("happy: returns thumb body via S3 key from the map", func(t *testing.T) {
		svc, repo, st, _ := newService(t)
		meta := avatarWithThumbs()
		repo.EXPECT().GetByID(ctx, id).Return(meta, nil)

		body := io.NopCloser(strings.NewReader("thumb-bytes"))
		st.EXPECT().Download(ctx, meta.ThumbnailS3Keys["100x100"]).Return(&storage.DownloadResult{
			Reader:      body,
			ContentType: "image/jpeg",
			Size:        11,
		}, nil)

		result, err := svc.DownloadThumbnail(ctx, id, "100x100")
		require.NoError(t, err)
		require.True(t, result.Reader == body)
		require.Equal(t, "image/jpeg", result.ContentType)
		require.Equal(t, int64(11), result.Size)
		require.Same(t, meta, result.Avatar)
	})

	t.Run("avatar not found: ErrNotFound, storage not consulted", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		repo.EXPECT().GetByID(ctx, id).Return(nil, repoavatar.ErrNotFound)

		_, err := svc.DownloadThumbnail(ctx, id, "100x100")
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})

	t.Run("size key not present: ErrNotFound, storage not consulted", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		meta := avatarWithThumbs()
		repo.EXPECT().GetByID(ctx, id).Return(meta, nil)

		_, err := svc.DownloadThumbnail(ctx, id, "999x999")
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})

	t.Run("thumbnails not generated yet (nil map): ErrNotFound", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		meta := &domain.Avatar{ID: id, S3Key: "originals/x.jpg"} // ThumbnailS3Keys = nil
		repo.EXPECT().GetByID(ctx, id).Return(meta, nil)

		_, err := svc.DownloadThumbnail(ctx, id, "100x100")
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})

	t.Run("storage failure propagates wrapped", func(t *testing.T) {
		svc, repo, st, _ := newService(t)
		meta := avatarWithThumbs()
		repo.EXPECT().GetByID(ctx, id).Return(meta, nil)

		rawErr := errors.New("S3 down")
		st.EXPECT().Download(ctx, meta.ThumbnailS3Keys["100x100"]).Return(nil, rawErr)

		_, err := svc.DownloadThumbnail(ctx, id, "100x100")
		require.Error(t, err)
		require.NotErrorIs(t, err, avatar.ErrNotFound)
		require.ErrorIs(t, err, rawErr)
	})
}
