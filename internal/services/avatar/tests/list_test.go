package avatar_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

func TestListByUserID(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	t.Run("passes through list from repo", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		want := []*domain.Avatar{
			{ID: uuid.Must(uuid.NewV7()), UserID: userID},
			{ID: uuid.Must(uuid.NewV7()), UserID: userID},
		}
		repo.EXPECT().ListByUserID(ctx, userID).Return(want, nil)

		got, err := svc.ListByUserID(ctx, userID)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("empty list is not an error", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		repo.EXPECT().ListByUserID(ctx, userID).Return([]*domain.Avatar{}, nil)

		got, err := svc.ListByUserID(ctx, userID)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates other errors wrapped", func(t *testing.T) {
		svc, repo, _, _ := newService(t)
		rawErr := errors.New("PG down")
		repo.EXPECT().ListByUserID(ctx, userID).Return(nil, rawErr)

		got, err := svc.ListByUserID(ctx, userID)
		require.Nil(t, got)
		require.ErrorIs(t, err, rawErr)
	})
}
