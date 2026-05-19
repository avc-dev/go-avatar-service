package avatar_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
	"github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

func TestGetCurrentByUserID(t *testing.T) {
	ctx := context.Background()
	userID := uuid.Must(uuid.NewV7())

	t.Run("returns avatar when user has one", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		want := &domain.Avatar{ID: uuid.Must(uuid.NewV7()), UserID: userID}
		repo.EXPECT().GetCurrentByUserID(ctx, userID).Return(want, nil)

		got, err := svc.GetCurrentByUserID(ctx, userID)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("user has no avatars: ErrNotFound", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		repo.EXPECT().GetCurrentByUserID(ctx, userID).Return(nil, repoavatar.ErrNotFound)

		got, err := svc.GetCurrentByUserID(ctx, userID)
		require.Nil(t, got)
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})
}
