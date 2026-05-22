package avatar_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestListByUserID(t *testing.T) {
	repo := newRepo()
	ctx := t.Context()

	t.Run("returns avatars in DESC order and excludes soft-deleted", func(t *testing.T) {
		truncateAvatars(t)
		userID := uuid.Must(uuid.NewV7())

		a1 := newAvatar(t, userID)
		require.NoError(t, repo.Create(ctx, a1))
		time.Sleep(20 * time.Millisecond)

		a2 := newAvatar(t, userID)
		require.NoError(t, repo.Create(ctx, a2))
		time.Sleep(20 * time.Millisecond)

		a3 := newAvatar(t, userID)
		require.NoError(t, repo.Create(ctx, a3))

		list, err := repo.ListByUserID(ctx, userID)
		require.NoError(t, err)
		require.Len(t, list, 3)
		require.Equal(t, a3.ID, list[0].ID)
		require.Equal(t, a2.ID, list[1].ID)
		require.Equal(t, a1.ID, list[2].ID)

		// Soft-delete the middle one.
		require.NoError(t, repo.SoftDelete(ctx, a2.ID))

		list, err = repo.ListByUserID(ctx, userID)
		require.NoError(t, err)
		require.Len(t, list, 2)
		require.Equal(t, a3.ID, list[0].ID)
		require.Equal(t, a1.ID, list[1].ID)
	})

	t.Run("returns empty slice (no error) when user has no avatars", func(t *testing.T) {
		list, err := repo.ListByUserID(ctx, uuid.Must(uuid.NewV7()))
		require.NoError(t, err)
		require.Empty(t, list)
	})
}
