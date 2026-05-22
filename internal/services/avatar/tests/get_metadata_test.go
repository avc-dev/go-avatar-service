package avatar_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
	"github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

func TestGetMetadata(t *testing.T) {
	ctx := t.Context()
	id := uuid.Must(uuid.NewV7())

	t.Run("returns avatar when repo finds it", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		want := &domain.Avatar{ID: id, UserID: uuid.Must(uuid.NewV7())}
		repo.EXPECT().GetByID(ctx, id).Return(want, nil)

		got, err := svc.GetMetadata(ctx, id)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("translates repo.ErrNotFound to service.ErrNotFound", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		repo.EXPECT().GetByID(ctx, id).Return(nil, repoavatar.ErrNotFound)

		got, err := svc.GetMetadata(ctx, id)
		require.Nil(t, got)
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})

	t.Run("wraps other repo errors", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		rawErr := errors.New("PG down")
		repo.EXPECT().GetByID(ctx, id).Return(nil, rawErr)

		got, err := svc.GetMetadata(ctx, id)
		require.Nil(t, got)
		require.Error(t, err)
		require.NotErrorIs(t, err, avatar.ErrNotFound)
		require.ErrorIs(t, err, rawErr)
	})
}
