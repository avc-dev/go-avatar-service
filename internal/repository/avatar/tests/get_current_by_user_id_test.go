package avatar_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

func TestGetCurrentByUserID(t *testing.T) {
	repo := newRepo()
	ctx := context.Background()

	t.Run("returns most recent avatar for user", func(t *testing.T) {
		truncateAvatars(t)
		userID := uuid.Must(uuid.NewV7())

		a1 := newAvatar(t, userID)
		require.NoError(t, repo.Create(ctx, a1))

		time.Sleep(20 * time.Millisecond) // ensure strictly increasing created_at

		a2 := newAvatar(t, userID)
		require.NoError(t, repo.Create(ctx, a2))

		got, err := repo.GetCurrentByUserID(ctx, userID)
		require.NoError(t, err)
		require.Equal(t, a2.ID, got.ID)
	})

	t.Run("returns ErrNotFound when user has no avatars", func(t *testing.T) {
		_, err := repo.GetCurrentByUserID(ctx, uuid.Must(uuid.NewV7()))
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})
}
