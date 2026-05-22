package avatar_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	"github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

func TestUpload(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	buildParams := func() avatar.UploadParams {
		const payload = "fake image data"
		return avatar.UploadParams{
			UserID:   userID,
			FileName: "selfie.jpg",
			MIMEType: "image/jpeg",
			Size:     int64(len(payload)),
			Reader:   strings.NewReader(payload),
		}
	}

	t.Run("happy path: storage + repo + publisher all succeed", func(t *testing.T) {
		svc, repo, st, pub := newService(t)
		params := buildParams()

		var capturedKey string
		st.EXPECT().
			Upload(ctx, mock.Anything, params.Reader, params.Size, params.MIMEType).
			Run(func(_ context.Context, key string, _ io.Reader, _ int64, _ string) {
				capturedKey = key
			}).
			Return(nil)

		repo.EXPECT().
			Create(ctx, mock.MatchedBy(func(a *domain.Avatar) bool {
				return a.UserID == userID &&
					a.FileName == "selfie.jpg" &&
					a.MIMEType == "image/jpeg" &&
					a.UploadStatus == domain.UploadStatusCompleted &&
					a.ProcessingStatus == domain.ProcessingStatusPending &&
					strings.HasPrefix(a.S3Key, "originals/") &&
					strings.HasSuffix(a.S3Key, ".jpg")
			})).
			Return(nil)

		pub.EXPECT().
			PublishAvatarUploaded(ctx, mock.MatchedBy(func(e domain.AvatarUploadEvent) bool {
				return e.UserID == userID && strings.HasPrefix(e.S3Key, "originals/")
			})).
			Return(nil)

		got, err := svc.Upload(ctx, params)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, userID, got.UserID)
		require.Equal(t, domain.UploadStatusCompleted, got.UploadStatus)
		require.Equal(t, capturedKey, got.S3Key, "the persisted S3 key must match the one used for upload")
	})

	t.Run("storage upload fails: no repo or publisher calls", func(t *testing.T) {
		svc, _, st, _ := newService(t)
		params := buildParams()

		st.EXPECT().
			Upload(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(errors.New("S3 down"))

		got, err := svc.Upload(ctx, params)
		require.Error(t, err)
		require.Nil(t, got)
		require.Contains(t, err.Error(), "store original")
	})

	t.Run("repo create fails: compensation Delete is called, original error propagated", func(t *testing.T) {
		svc, repo, st, _ := newService(t)
		params := buildParams()

		var capturedKey string
		st.EXPECT().
			Upload(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, key string, _ io.Reader, _ int64, _ string) {
				capturedKey = key
			}).
			Return(nil)

		repoErr := errors.New("PG down")
		repo.EXPECT().Create(ctx, mock.Anything).Return(repoErr)

		st.EXPECT().
			Delete(ctx, mock.MatchedBy(func(k string) bool { return k == capturedKey })).
			Return(nil)

		got, err := svc.Upload(ctx, params)
		require.Error(t, err)
		require.Nil(t, got)
		require.ErrorIs(t, err, repoErr, "original repo error should be the wrapped cause")
	})

	t.Run("repo create fails AND compensation also fails: original error still returned", func(t *testing.T) {
		svc, repo, st, _ := newService(t)
		params := buildParams()

		st.EXPECT().
			Upload(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)

		repoErr := errors.New("PG down")
		repo.EXPECT().Create(ctx, mock.Anything).Return(repoErr)

		st.EXPECT().
			Delete(ctx, mock.Anything).
			Return(errors.New("S3 also down"))

		got, err := svc.Upload(ctx, params)
		require.Error(t, err)
		require.Nil(t, got)
		// Caller sees the original error, not the compensation error.
		require.ErrorIs(t, err, repoErr)
	})

	t.Run("publisher fails after successful create: returns success (avatar exists)", func(t *testing.T) {
		svc, repo, st, pub := newService(t)
		params := buildParams()

		st.EXPECT().
			Upload(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		repo.EXPECT().Create(ctx, mock.Anything).Return(nil)
		pub.EXPECT().
			PublishAvatarUploaded(ctx, mock.Anything).
			Return(errors.New("broker down"))

		got, err := svc.Upload(ctx, params)
		require.NoError(t, err, "publisher failure must not propagate — avatar is persisted")
		require.NotNil(t, got)
		// No compensation call expected; the test would fail at cleanup if Delete
		// had been called without an expectation.
	})
}
