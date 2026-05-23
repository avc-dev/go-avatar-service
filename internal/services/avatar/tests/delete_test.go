package avatar_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
	"github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

func TestDelete(t *testing.T) {
	ctx := t.Context()
	id := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())

	buildDeleted := func() *domain.Avatar {
		return &domain.Avatar{
			ID:     id,
			UserID: userID,
			S3Key:  "originals/x.jpg",
			ThumbnailS3Keys: map[string]string{
				"100x100": "thumbnails/x/100.jpg",
				"300x300": "thumbnails/x/300.jpg",
			},
		}
	}

	t.Run("happy: deletes, publishes event with original and all thumbnails", func(t *testing.T) {
		svc, repo, _, pub := newService(t)
		a := buildDeleted()
		repo.EXPECT().DeleteByOwner(ctx, id, userID).Return(a, nil)

		pub.EXPECT().PublishAvatarDeleted(ctx, mock.MatchedBy(func(e domain.AvatarDeleteEvent) bool {
			if e.AvatarID != id || len(e.S3Keys) != 3 {
				return false
			}
			seen := make(map[string]struct{}, len(e.S3Keys))
			for _, k := range e.S3Keys {
				seen[k] = struct{}{}
			}
			_, hasOrig := seen["originals/x.jpg"]
			_, hasT1 := seen["thumbnails/x/100.jpg"]
			_, hasT2 := seen["thumbnails/x/300.jpg"]
			return hasOrig && hasT1 && hasT2
		})).Return(nil)

		err := svc.Delete(ctx, id, userID)
		require.NoError(t, err)
	})

	t.Run("not found: no publish, ErrNotFound returned", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		repo.EXPECT().DeleteByOwner(ctx, id, userID).Return(nil, repoavatar.ErrNotFound)

		err := svc.Delete(ctx, id, userID)
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})

	t.Run("forbidden: no publish, ErrForbidden returned", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		repo.EXPECT().DeleteByOwner(ctx, id, userID).Return(nil, repoavatar.ErrForbidden)

		err := svc.Delete(ctx, id, userID)
		require.ErrorIs(t, err, avatar.ErrForbidden)
	})

	t.Run("publisher failure after delete: success returned", func(t *testing.T) {
		svc, repo, _, pub := newService(t)
		a := buildDeleted()
		repo.EXPECT().DeleteByOwner(ctx, id, userID).Return(a, nil)
		pub.EXPECT().PublishAvatarDeleted(ctx, mock.Anything).Return(errors.New("broker down"))

		err := svc.Delete(ctx, id, userID)
		require.NoError(t, err, "publisher failure must not propagate — avatar is already soft-deleted")
	})

	t.Run("avatar with no thumbnails: event carries only the original key", func(t *testing.T) {
		svc, repo, _, pub := newService(t)
		a := &domain.Avatar{ID: id, UserID: userID, S3Key: "originals/x.jpg"}
		repo.EXPECT().DeleteByOwner(ctx, id, userID).Return(a, nil)

		pub.EXPECT().PublishAvatarDeleted(ctx, mock.MatchedBy(func(e domain.AvatarDeleteEvent) bool {
			return len(e.S3Keys) == 1 && e.S3Keys[0] == "originals/x.jpg"
		})).Return(nil)

		err := svc.Delete(ctx, id, userID)
		require.NoError(t, err)
	})
}
