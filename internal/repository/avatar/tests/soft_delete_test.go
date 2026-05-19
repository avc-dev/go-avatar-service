package avatar_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

// TestSoftDelete uses a table-driven layout: all cases share the same flow
// (setup → SoftDelete → check error), differing only in initial state.
func TestSoftDelete(t *testing.T) {
	repo := newRepo()
	ctx := context.Background()

	tests := []struct {
		name      string
		setup     func(t *testing.T) uuid.UUID // returns the id to delete
		wantErr   error
		verifyGet bool // when wantErr == nil, also verify GetByID returns ErrNotFound after delete
	}{
		{
			name: "soft-deletes live row",
			setup: func(t *testing.T) uuid.UUID {
				a := newAvatar(t, uuid.Must(uuid.NewV7()))
				require.NoError(t, repo.Create(ctx, a))
				return a.ID
			},
			wantErr:   nil,
			verifyGet: true,
		},
		{
			name: "returns ErrNotFound for unknown id",
			setup: func(t *testing.T) uuid.UUID {
				return uuid.Must(uuid.NewV7())
			},
			wantErr: avatar.ErrNotFound,
		},
		{
			name: "returns ErrNotFound for already-deleted row",
			setup: func(t *testing.T) uuid.UUID {
				a := newAvatar(t, uuid.Must(uuid.NewV7()))
				require.NoError(t, repo.Create(ctx, a))
				require.NoError(t, repo.SoftDelete(ctx, a.ID))
				return a.ID
			},
			wantErr: avatar.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateAvatars(t)
			id := tt.setup(t)

			err := repo.SoftDelete(ctx, id)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.verifyGet {
				_, getErr := repo.GetByID(ctx, id)
				require.ErrorIs(t, getErr, avatar.ErrNotFound)
			}
		})
	}
}
