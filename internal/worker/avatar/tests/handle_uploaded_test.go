package avatar_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
	"github.com/avc-dev/go-avatar-service/internal/storage"
)

// downloadResult is a small helper that builds a fresh DownloadResult with
// an in-memory body. Each call returns a new ReadCloser so retries that
// re-Download work in tests.
func downloadResult(body string) *storage.DownloadResult {
	return &storage.DownloadResult{
		Reader:      io.NopCloser(strings.NewReader(body)),
		ContentType: "image/jpeg",
		Size:        int64(len(body)),
	}
}

func TestHandleUploaded(t *testing.T) {
	ctx := context.Background()
	msgID := "msg-test"

	t.Run("happy: pending → processing → completed with all thumbs", func(t *testing.T) {
		w, repo, st, resizer := newWorker(t)
		ev, body := makeUploadEvent(t)

		repo.EXPECT().GetByID(ctx, ev.AvatarID).Return(pendingAvatar(ev.AvatarID), nil)
		repo.EXPECT().UpdateProcessingStatus(ctx, ev.AvatarID, domain.ProcessingStatusProcessing, map[string]string(nil)).Return(nil)
		st.EXPECT().Download(ctx, ev.S3Key).Return(downloadResult("original-bytes"), nil)

		thumb100 := []byte("thumb-100x100")
		thumb300 := []byte("thumb-300x300")
		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 100, 100, 85).Return(thumb100, nil)
		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 300, 300, 85).Return(thumb300, nil)

		st.EXPECT().Upload(ctx,
			"thumbnails/"+ev.AvatarID.String()+"/100x100.jpg",
			mock.Anything, int64(len(thumb100)), "image/jpeg",
		).Return(nil)
		st.EXPECT().Upload(ctx,
			"thumbnails/"+ev.AvatarID.String()+"/300x300.jpg",
			mock.Anything, int64(len(thumb300)), "image/jpeg",
		).Return(nil)

		repo.EXPECT().UpdateProcessingStatus(ctx, ev.AvatarID, domain.ProcessingStatusCompleted,
			mock.MatchedBy(func(thumbs map[string]string) bool {
				return len(thumbs) == 2 &&
					thumbs["100x100"] == "thumbnails/"+ev.AvatarID.String()+"/100x100.jpg" &&
					thumbs["300x300"] == "thumbnails/"+ev.AvatarID.String()+"/300x300.jpg"
			}),
		).Return(nil)

		require.NoError(t, w.HandleUploaded(ctx, body, msgID))
	})

	t.Run("avatar missing: ErrNotFound on GetByID → skip, no further calls", func(t *testing.T) {
		w, repo, _, _ := newWorker(t)
		ev, body := makeUploadEvent(t)

		// Idempotency: GetByID is retried before we decide it's truly missing.
		// withRetry calls fn maxAttempts times when it always errors, so the
		// mock must accept the same number of calls. With fastRetryDelays
		// (3 delays) that's 4 attempts.
		repo.EXPECT().GetByID(ctx, ev.AvatarID).
			Return(nil, repoavatar.ErrNotFound).Times(4)

		require.NoError(t, w.HandleUploaded(ctx, body, msgID))
	})

	t.Run("status gate skips already-handled states", func(t *testing.T) {
		cases := []struct {
			name   string
			status domain.ProcessingStatus
		}{
			{"completed", domain.ProcessingStatusCompleted},
			{"failed", domain.ProcessingStatusFailed},
			{"processing", domain.ProcessingStatusProcessing},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				w, repo, _, _ := newWorker(t)
				ev, body := makeUploadEvent(t)

				avatar := pendingAvatar(ev.AvatarID)
				avatar.ProcessingStatus = tc.status
				repo.EXPECT().GetByID(ctx, ev.AvatarID).Return(avatar, nil)

				require.NoError(t, w.HandleUploaded(ctx, body, msgID))
			})
		}
	})

	t.Run("malformed JSON body: error returned, no mock calls", func(t *testing.T) {
		w, _, _, _ := newWorker(t)
		err := w.HandleUploaded(ctx, []byte("not json"), msgID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unmarshal upload event")
	})

	t.Run("download fails after all retries: mark failed + error", func(t *testing.T) {
		w, repo, st, _ := newWorker(t)
		ev, body := makeUploadEvent(t)

		repo.EXPECT().GetByID(ctx, ev.AvatarID).Return(pendingAvatar(ev.AvatarID), nil)
		repo.EXPECT().UpdateProcessingStatus(ctx, ev.AvatarID, domain.ProcessingStatusProcessing, map[string]string(nil)).Return(nil)

		downErr := errors.New("s3 unavailable")
		st.EXPECT().Download(ctx, ev.S3Key).Return(nil, downErr).Times(4)

		// markFailed uses a context.WithoutCancel + 5s timeout, so the mock
		// is called with a different ctx than the test's ctx. Match on the
		// other args precisely.
		repo.EXPECT().UpdateProcessingStatus(mock.Anything, ev.AvatarID, domain.ProcessingStatusFailed, map[string]string(nil)).Return(nil)

		err := w.HandleUploaded(ctx, body, msgID)
		require.Error(t, err)
		require.ErrorIs(t, err, downErr)
	})

	t.Run("resize fails: mark failed + error, no thumb upload attempts", func(t *testing.T) {
		w, repo, st, resizer := newWorker(t)
		ev, body := makeUploadEvent(t)

		repo.EXPECT().GetByID(ctx, ev.AvatarID).Return(pendingAvatar(ev.AvatarID), nil)
		repo.EXPECT().UpdateProcessingStatus(ctx, ev.AvatarID, domain.ProcessingStatusProcessing, map[string]string(nil)).Return(nil)
		st.EXPECT().Download(ctx, ev.S3Key).Return(downloadResult("bytes"), nil)

		resizeErr := errors.New("invalid image data")
		// First iteration is 100x100; it fails immediately, no retry on resize.
		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 100, 100, 85).Return(nil, resizeErr)

		repo.EXPECT().UpdateProcessingStatus(mock.Anything, ev.AvatarID, domain.ProcessingStatusFailed, map[string]string(nil)).Return(nil)

		err := w.HandleUploaded(ctx, body, msgID)
		require.Error(t, err)
		require.ErrorIs(t, err, resizeErr)
	})

	t.Run("mark completed fails: error returned, no second mark-failed", func(t *testing.T) {
		w, repo, st, resizer := newWorker(t)
		ev, body := makeUploadEvent(t)

		repo.EXPECT().GetByID(ctx, ev.AvatarID).Return(pendingAvatar(ev.AvatarID), nil)
		repo.EXPECT().UpdateProcessingStatus(ctx, ev.AvatarID, domain.ProcessingStatusProcessing, map[string]string(nil)).Return(nil)
		st.EXPECT().Download(ctx, ev.S3Key).Return(downloadResult("bytes"), nil)
		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 100, 100, 85).Return([]byte("t100"), nil)
		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 300, 300, 85).Return([]byte("t300"), nil)
		st.EXPECT().Upload(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

		finalErr := errors.New("pg ouch")
		repo.EXPECT().UpdateProcessingStatus(ctx, ev.AvatarID, domain.ProcessingStatusCompleted, mock.Anything).
			Return(finalErr).Times(4)

		err := w.HandleUploaded(ctx, body, msgID)
		require.Error(t, err)
		require.ErrorIs(t, err, finalErr)
		// No mark-failed expectation set: mock would fail the test if it were called.
	})

	t.Run("download succeeds on retry: full flow completes", func(t *testing.T) {
		w, repo, st, resizer := newWorker(t)
		ev, body := makeUploadEvent(t)

		repo.EXPECT().GetByID(ctx, ev.AvatarID).Return(pendingAvatar(ev.AvatarID), nil)
		repo.EXPECT().UpdateProcessingStatus(ctx, ev.AvatarID, domain.ProcessingStatusProcessing, map[string]string(nil)).Return(nil)

		// Two failures, then success.
		downErr := errors.New("transient")
		st.EXPECT().Download(ctx, ev.S3Key).Return(nil, downErr).Times(2)
		st.EXPECT().Download(ctx, ev.S3Key).Return(downloadResult("bytes"), nil).Once()

		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 100, 100, 85).Return([]byte("t100"), nil)
		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 300, 300, 85).Return([]byte("t300"), nil)
		st.EXPECT().Upload(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)
		repo.EXPECT().UpdateProcessingStatus(ctx, ev.AvatarID, domain.ProcessingStatusCompleted, mock.Anything).Return(nil)

		require.NoError(t, w.HandleUploaded(ctx, body, msgID))
	})
}
