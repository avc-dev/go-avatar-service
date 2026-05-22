package avatar_test

import (
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

// TestHandleUploaded exercises the worker's atomic-claim flow. The legacy
// "GetByID + then UpdateProcessingStatus(processing)" sequence has been
// collapsed into a single ClaimForProcessing call, which is the only
// idempotency gate — these tests reflect that contract.
func TestHandleUploaded(t *testing.T) {
	ctx := t.Context()
	msgID := "msg-test"

	t.Run("happy: claim succeeds → process → mark completed", func(t *testing.T) {
		w, repo, st, resizer := newWorker(t)
		ev, body := makeUploadEvent(t)

		claimed := pendingAvatar(ev.AvatarID)
		// After ClaimForProcessing the row's status is already 'processing'.
		// The worker uses claimed.S3Key (DB source of truth) for the download.
		claimed.ProcessingStatus = domain.ProcessingStatusProcessing
		repo.EXPECT().ClaimForProcessing(ctx, ev.AvatarID).Return(claimed, nil)

		st.EXPECT().Download(ctx, claimed.S3Key).Return(downloadResult("original-bytes"), nil)

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

	t.Run("claim returns ErrNotFound → skip silently (already-claimed or missing)", func(t *testing.T) {
		w, repo, _, _ := newWorker(t)
		ev, body := makeUploadEvent(t)

		// ClaimForProcessing collapses "already claimed" and "row missing"
		// into the same ErrNotFound. withRetry treats ErrNotFound as
		// non-transient and propagates after maxAttempts attempts — so the
		// mock must accept exactly that many calls. With fastRetryDelays
		// (3 delays) maxAttempts is 4.
		repo.EXPECT().ClaimForProcessing(ctx, ev.AvatarID).
			Return(nil, repoavatar.ErrNotFound).Times(4)

		require.NoError(t, w.HandleUploaded(ctx, body, msgID))
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

		claimed := pendingAvatar(ev.AvatarID)
		claimed.ProcessingStatus = domain.ProcessingStatusProcessing
		repo.EXPECT().ClaimForProcessing(ctx, ev.AvatarID).Return(claimed, nil)

		downErr := errors.New("s3 unavailable")
		st.EXPECT().Download(ctx, claimed.S3Key).Return(nil, downErr).Times(4)

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

		claimed := pendingAvatar(ev.AvatarID)
		claimed.ProcessingStatus = domain.ProcessingStatusProcessing
		repo.EXPECT().ClaimForProcessing(ctx, ev.AvatarID).Return(claimed, nil)
		st.EXPECT().Download(ctx, claimed.S3Key).Return(downloadResult("bytes"), nil)

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

		claimed := pendingAvatar(ev.AvatarID)
		claimed.ProcessingStatus = domain.ProcessingStatusProcessing
		repo.EXPECT().ClaimForProcessing(ctx, ev.AvatarID).Return(claimed, nil)
		st.EXPECT().Download(ctx, claimed.S3Key).Return(downloadResult("bytes"), nil)
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

		claimed := pendingAvatar(ev.AvatarID)
		claimed.ProcessingStatus = domain.ProcessingStatusProcessing
		repo.EXPECT().ClaimForProcessing(ctx, ev.AvatarID).Return(claimed, nil)

		// Two failures, then success.
		downErr := errors.New("transient")
		st.EXPECT().Download(ctx, claimed.S3Key).Return(nil, downErr).Times(2)
		st.EXPECT().Download(ctx, claimed.S3Key).Return(downloadResult("bytes"), nil).Once()

		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 100, 100, 85).Return([]byte("t100"), nil)
		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 300, 300, 85).Return([]byte("t300"), nil)
		st.EXPECT().Upload(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)
		repo.EXPECT().UpdateProcessingStatus(ctx, ev.AvatarID, domain.ProcessingStatusCompleted, mock.Anything).Return(nil)

		require.NoError(t, w.HandleUploaded(ctx, body, msgID))
	})

	t.Run("upload of second thumb fails: first thumb is best-effort deleted", func(t *testing.T) {
		// This is the partial-cleanup contract: when the worker has uploaded
		// thumbnail N but thumbnail N+1 fails, markFailed must Delete the
		// already-uploaded keys so we don't leave orphans in S3.
		w, repo, st, resizer := newWorker(t)
		ev, body := makeUploadEvent(t)

		claimed := pendingAvatar(ev.AvatarID)
		claimed.ProcessingStatus = domain.ProcessingStatusProcessing
		repo.EXPECT().ClaimForProcessing(ctx, ev.AvatarID).Return(claimed, nil)
		st.EXPECT().Download(ctx, claimed.S3Key).Return(downloadResult("bytes"), nil)

		// First thumb resizes and uploads cleanly.
		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 100, 100, 85).Return([]byte("t100"), nil)
		firstKey := "thumbnails/" + ev.AvatarID.String() + "/100x100.jpg"
		st.EXPECT().Upload(ctx, firstKey, mock.Anything, int64(4), "image/jpeg").Return(nil)

		// Second thumb's upload fails on every retry.
		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 300, 300, 85).Return([]byte("t300"), nil)
		secondKey := "thumbnails/" + ev.AvatarID.String() + "/300x300.jpg"
		uploadErr := errors.New("s3 hiccup")
		st.EXPECT().Upload(ctx, secondKey, mock.Anything, int64(4), "image/jpeg").Return(uploadErr).Times(4)

		// markFailed must Delete the already-uploaded first thumb. The
		// cleanup context is freshly derived via context.WithoutCancel, so
		// the mock matches with mock.Anything rather than ctx.
		st.EXPECT().Delete(mock.Anything, firstKey).Return(nil)
		repo.EXPECT().UpdateProcessingStatus(mock.Anything, ev.AvatarID, domain.ProcessingStatusFailed, map[string]string(nil)).Return(nil)

		err := w.HandleUploaded(ctx, body, msgID)
		require.Error(t, err)
		require.ErrorIs(t, err, uploadErr)
	})

	t.Run("partial-cleanup is best-effort: Delete failure does not block status mark", func(t *testing.T) {
		// Even when the partial-thumb cleanup itself fails, the status row
		// must still be moved to 'failed' so the UI/operator sees the
		// terminal state.
		w, repo, st, resizer := newWorker(t)
		ev, body := makeUploadEvent(t)

		claimed := pendingAvatar(ev.AvatarID)
		claimed.ProcessingStatus = domain.ProcessingStatusProcessing
		repo.EXPECT().ClaimForProcessing(ctx, ev.AvatarID).Return(claimed, nil)
		st.EXPECT().Download(ctx, claimed.S3Key).Return(downloadResult("bytes"), nil)

		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 100, 100, 85).Return([]byte("t100"), nil)
		firstKey := "thumbnails/" + ev.AvatarID.String() + "/100x100.jpg"
		st.EXPECT().Upload(ctx, firstKey, mock.Anything, int64(4), "image/jpeg").Return(nil)

		resizeErr := errors.New("bad image")
		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, 300, 300, 85).Return(nil, resizeErr)

		// Delete throws — the worker must still attempt the status mark.
		st.EXPECT().Delete(mock.Anything, firstKey).Return(errors.New("s3 down"))
		repo.EXPECT().UpdateProcessingStatus(mock.Anything, ev.AvatarID, domain.ProcessingStatusFailed, map[string]string(nil)).Return(nil)

		err := w.HandleUploaded(ctx, body, msgID)
		require.Error(t, err)
		require.ErrorIs(t, err, resizeErr)
	})

	t.Run("claim transient error then success: flow completes after retry", func(t *testing.T) {
		w, repo, st, resizer := newWorker(t)
		ev, body := makeUploadEvent(t)

		claimErr := errors.New("pg flaky")
		repo.EXPECT().ClaimForProcessing(ctx, ev.AvatarID).Return(nil, claimErr).Times(2)

		claimed := pendingAvatar(ev.AvatarID)
		claimed.ProcessingStatus = domain.ProcessingStatusProcessing
		repo.EXPECT().ClaimForProcessing(ctx, ev.AvatarID).Return(claimed, nil).Once()

		st.EXPECT().Download(ctx, claimed.S3Key).Return(downloadResult("bytes"), nil)
		resizer.EXPECT().ResizeToJPEG(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]byte("t"), nil).Times(2)
		st.EXPECT().Upload(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)
		repo.EXPECT().UpdateProcessingStatus(ctx, ev.AvatarID, domain.ProcessingStatusCompleted, mock.Anything).Return(nil)

		require.NoError(t, w.HandleUploaded(ctx, body, msgID))
	})
}
