package avatar_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	workeravatar "github.com/avc-dev/go-avatar-service/internal/worker/avatar"
	"github.com/avc-dev/go-avatar-service/internal/worker/avatar/mocks"
)

// fastRetryDelays makes retry-driven tests complete in milliseconds rather
// than seconds. Three 1ms entries match the production schedule's length
// (3 retries → 4 attempts total) so retry-exhaustion tests still exercise
// the loop body the right number of times.
var fastRetryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

// newWorker builds a Worker wired with fresh mocks. Tests may pass extra
// options (e.g. avatar.WithThumbnailSizes) on top of the fast-retry default.
func newWorker(t *testing.T, opts ...workeravatar.Option) (
	*workeravatar.Worker,
	*mocks.MockRepository,
	*mocks.MockStorage,
	*mocks.MockImageResizer,
) {
	t.Helper()
	repo := mocks.NewMockRepository(t)
	st := mocks.NewMockStorage(t)
	resizer := mocks.NewMockImageResizer(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	full := append([]workeravatar.Option{workeravatar.WithRetryDelays(fastRetryDelays)}, opts...)
	w := workeravatar.New(repo, st, resizer, log, full...)
	return w, repo, st, resizer
}

// makeUploadEvent returns a fresh AvatarUploadEvent plus its JSON serialisation.
func makeUploadEvent(t *testing.T) (domain.AvatarUploadEvent, []byte) {
	t.Helper()
	avatarID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())
	ev := domain.AvatarUploadEvent{
		AvatarID: avatarID,
		UserID:   userID,
		S3Key:    "originals/" + avatarID.String() + ".jpg",
	}
	body, err := json.Marshal(ev)
	require.NoError(t, err)
	return ev, body
}

// makeDeleteEvent returns a fresh AvatarDeleteEvent with the given keys plus
// its JSON serialisation.
func makeDeleteEvent(t *testing.T, keys ...string) (domain.AvatarDeleteEvent, []byte) {
	t.Helper()
	ev := domain.AvatarDeleteEvent{
		AvatarID: uuid.Must(uuid.NewV7()),
		S3Keys:   keys,
	}
	body, err := json.Marshal(ev)
	require.NoError(t, err)
	return ev, body
}

// pendingAvatar returns a *domain.Avatar with the given id in the pending
// state, suitable for the GetByID mock return.
func pendingAvatar(id uuid.UUID) *domain.Avatar {
	return &domain.Avatar{
		ID:               id,
		UserID:           uuid.Must(uuid.NewV7()),
		FileName:         "photo.jpg",
		MIMEType:         "image/jpeg",
		SizeBytes:        12345,
		S3Key:            "originals/" + id.String() + ".jpg",
		UploadStatus:     domain.UploadStatusCompleted,
		ProcessingStatus: domain.ProcessingStatusPending,
	}
}
