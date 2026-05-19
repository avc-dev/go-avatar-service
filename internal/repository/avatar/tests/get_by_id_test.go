package avatar_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

func TestGetByID(t *testing.T) {
	repo := newRepo()
	ctx := context.Background()

	t.Run("returns avatar when row exists", func(t *testing.T) {
		truncateAvatars(t)
		a := newAvatar(t, uuid.Must(uuid.NewV7()))
		require.NoError(t, repo.Create(ctx, a))

		got, err := repo.GetByID(ctx, a.ID)
		require.NoError(t, err)
		require.Equal(t, a.ID, got.ID)
	})

	t.Run("returns ErrNotFound for unknown id", func(t *testing.T) {
		_, err := repo.GetByID(ctx, uuid.Must(uuid.NewV7()))
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})
}
