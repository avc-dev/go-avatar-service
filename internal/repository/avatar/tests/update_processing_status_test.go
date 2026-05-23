package avatar_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	"github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

func TestUpdateProcessingStatus(t *testing.T) {
	repo := newRepo()
	ctx := t.Context()

	t.Run("updates status, persists thumbs, and trigger bumps updated_at", func(t *testing.T) {
		truncateAvatars(t)
		a := newAvatar(t, uuid.Must(uuid.NewV7()))
		require.NoError(t, repo.Create(ctx, a))
		initialUpdatedAt := a.UpdatedAt

		// Ensure measurable clock progression for NOW() in the trigger.
		time.Sleep(20 * time.Millisecond)

		thumbs := map[string]string{
			"100x100": "thumbnails/" + a.ID.String() + "/100x100.jpg",
			"300x300": "thumbnails/" + a.ID.String() + "/300x300.jpg",
		}
		require.NoError(t, repo.UpdateProcessingStatus(ctx, a.ID, domain.ProcessingStatusCompleted, thumbs))

		got, err := repo.GetByID(ctx, a.ID)
		require.NoError(t, err)
		require.Equal(t, domain.ProcessingStatusCompleted, got.ProcessingStatus)
		require.Equal(t, thumbs, got.ThumbnailS3Keys)
		require.True(t, got.UpdatedAt.After(initialUpdatedAt),
			"updated_at trigger should bump the timestamp (was %s, now %s)",
			initialUpdatedAt, got.UpdatedAt)
	})

	t.Run("empty map persists as empty JSON object (not NULL)", func(t *testing.T) {
		truncateAvatars(t)
		a := newAvatar(t, uuid.Must(uuid.NewV7()))
		require.NoError(t, repo.Create(ctx, a))

		require.NoError(t, repo.UpdateProcessingStatus(ctx, a.ID, domain.ProcessingStatusFailed, map[string]string{}))

		got, err := repo.GetByID(ctx, a.ID)
		require.NoError(t, err)
		require.NotNil(t, got.ThumbnailS3Keys)
		require.Empty(t, got.ThumbnailS3Keys)
	})

	t.Run("returns ErrNotFound for unknown id", func(t *testing.T) {
		err := repo.UpdateProcessingStatus(ctx, uuid.Must(uuid.NewV7()), domain.ProcessingStatusCompleted, nil)
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})
}
