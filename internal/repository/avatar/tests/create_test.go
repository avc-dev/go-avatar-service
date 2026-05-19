package avatar_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

func TestCreate(t *testing.T) {
	truncateAvatars(t)
	repo := newRepo()
	ctx := context.Background()

	userID := uuid.Must(uuid.NewV7())
	a := newAvatar(t, userID)

	require.NoError(t, repo.Create(ctx, a))
	require.False(t, a.CreatedAt.IsZero(), "Create should populate CreatedAt")
	require.False(t, a.UpdatedAt.IsZero(), "Create should populate UpdatedAt")

	got, err := repo.GetByID(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, a.ID, got.ID)
	require.Equal(t, a.UserID, got.UserID)
	require.Equal(t, a.FileName, got.FileName)
	require.Equal(t, a.MIMEType, got.MIMEType)
	require.Equal(t, a.SizeBytes, got.SizeBytes)
	require.Equal(t, a.S3Key, got.S3Key)
	require.Nil(t, got.ThumbnailS3Keys, "nil thumbnails should round-trip as nil")
	require.Equal(t, domain.UploadStatusUploading, got.UploadStatus)
	require.Equal(t, domain.ProcessingStatusPending, got.ProcessingStatus)
	require.Nil(t, got.DeletedAt)
}
