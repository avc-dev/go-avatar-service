package avatar_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
	"github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

func TestDeleteCurrentByUserID(t *testing.T) {
	ctx := context.Background()
	userID := uuid.Must(uuid.NewV7())

	t.Run("happy: fetches current, deletes via owner-aware path, publishes", func(t *testing.T) {
		svc, repo, _, pub := newService(t)
		current := &domain.Avatar{
			ID:     uuid.Must(uuid.NewV7()),
			UserID: userID,
			S3Key:  "originals/x.jpg",
		}
		repo.EXPECT().GetCurrentByUserID(ctx, userID).Return(current, nil)
		repo.EXPECT().DeleteByOwner(ctx, current.ID, userID).Return(current, nil)
		pub.EXPECT().PublishAvatarDeleted(ctx, mock.Anything).Return(nil)

		err := svc.DeleteCurrentByUserID(ctx, userID)
		require.NoError(t, err)
	})

	t.Run("no current avatar: ErrNotFound, no further calls", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		repo.EXPECT().GetCurrentByUserID(ctx, userID).Return(nil, repoavatar.ErrNotFound)

		err := svc.DeleteCurrentByUserID(ctx, userID)
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})
}
