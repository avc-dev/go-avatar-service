package avatar_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

// TestDeleteByOwner is table-driven: each case sets up state, runs DeleteByOwner,
// then checks the resulting error and, optionally, the post-condition state.
func TestDeleteByOwner(t *testing.T) {
	repo := newRepo()
	ctx := t.Context()

	tests := []struct {
		name      string
		setup     func(t *testing.T) (id, requestingUserID uuid.UUID)
		wantErr   error
		postCheck func(t *testing.T, id uuid.UUID)
	}{
		{
			name: "owner soft-deletes own avatar",
			setup: func(t *testing.T) (uuid.UUID, uuid.UUID) {
				userID := uuid.Must(uuid.NewV7())
				a := newAvatar(t, userID)
				require.NoError(t, repo.Create(ctx, a))
				return a.ID, userID
			},
			wantErr: nil,
			postCheck: func(t *testing.T, id uuid.UUID) {
				_, err := repo.GetByID(ctx, id)
				require.ErrorIs(t, err, avatar.ErrNotFound)
			},
		},
		{
			name: "non-owner gets ErrForbidden and row is untouched",
			setup: func(t *testing.T) (uuid.UUID, uuid.UUID) {
				ownerID := uuid.Must(uuid.NewV7())
				attackerID := uuid.Must(uuid.NewV7())
				a := newAvatar(t, ownerID)
				require.NoError(t, repo.Create(ctx, a))
				return a.ID, attackerID
			},
			wantErr: avatar.ErrForbidden,
			postCheck: func(t *testing.T, id uuid.UUID) {
				got, err := repo.GetByID(ctx, id)
				require.NoError(t, err, "row must still be alive after a forbidden delete")
				require.NotNil(t, got)
				require.Nil(t, got.DeletedAt)
			},
		},
		{
			name: "unknown id returns ErrNotFound",
			setup: func(t *testing.T) (uuid.UUID, uuid.UUID) {
				return uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
			},
			wantErr: avatar.ErrNotFound,
		},
		{
			name: "already-deleted row returns ErrNotFound",
			setup: func(t *testing.T) (uuid.UUID, uuid.UUID) {
				userID := uuid.Must(uuid.NewV7())
				a := newAvatar(t, userID)
				require.NoError(t, repo.Create(ctx, a))
				require.NoError(t, repo.SoftDelete(ctx, a.ID))
				return a.ID, userID
			},
			wantErr: avatar.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateAvatars(t)
			id, requestingUserID := tt.setup(t)

			got, err := repo.DeleteByOwner(ctx, id, requestingUserID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				require.Equal(t, id, got.ID)
				require.NotNil(t, got.DeletedAt, "returned record should carry the soft-delete timestamp")
			}

			if tt.postCheck != nil {
				tt.postCheck(t, id)
			}
		})
	}
}
