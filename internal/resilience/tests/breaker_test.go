package resilience_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	"github.com/avc-dev/go-avatar-service/internal/resilience"
	"github.com/avc-dev/go-avatar-service/internal/storage"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeStorage is a stub ObjectStorage that counts calls and returns a fixed
// error (nil = success), so a test can drive the breaker however it wants.
type fakeStorage struct {
	err   error
	calls int
}

func (f *fakeStorage) Upload(context.Context, string, io.Reader, int64, string) error {
	f.calls++
	return f.err
}

func (f *fakeStorage) Download(context.Context, string) (*storage.DownloadResult, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &storage.DownloadResult{
		Reader:      io.NopCloser(strings.NewReader("abc")),
		ContentType: "image/png",
		Size:        3,
	}, nil
}

func (f *fakeStorage) Delete(context.Context, string) error {
	f.calls++
	return f.err
}

func TestStorage_PassesThroughSuccess(t *testing.T) {
	fake := &fakeStorage{}
	s := resilience.NewStorage(fake, resilience.Settings{}, discardLogger())

	res, err := s.Download(context.Background(), "key")
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, "image/png", res.ContentType)
	require.Equal(t, 1, fake.calls)
}

func TestStorage_OpensAfterSustainedFailures(t *testing.T) {
	fake := &fakeStorage{err: errors.New("backend down")}
	// Long timeout so once it trips it stays open for the whole test — no
	// half-open probe slips through.
	s := resilience.NewStorage(fake, resilience.Settings{
		MinRequests:  5,
		FailureRatio: 0.6,
		Timeout:      time.Minute,
	}, discardLogger())

	// Five failing calls reach the backend; the fifth trips the breaker.
	for i := 0; i < 5; i++ {
		require.Error(t, s.Delete(context.Background(), "key"))
	}
	require.Equal(t, 5, fake.calls)

	// Now open: the next call fails fast without touching the backend.
	err := s.Delete(context.Background(), "key")
	require.ErrorIs(t, err, gobreaker.ErrOpenState)
	require.Equal(t, 5, fake.calls, "inner must not be called while the breaker is open")
}

func TestStorage_CanceledDoesNotTrip(t *testing.T) {
	fake := &fakeStorage{err: context.Canceled}
	s := resilience.NewStorage(fake, resilience.Settings{
		MinRequests:  3,
		FailureRatio: 0.5,
		Timeout:      time.Minute,
	}, discardLogger())

	// Cancellation isn't a backend fault, so the breaker stays closed and every
	// call still gets through.
	for i := 0; i < 10; i++ {
		require.ErrorIs(t, s.Delete(context.Background(), "key"), context.Canceled)
	}
	require.Equal(t, 10, fake.calls)
}

// fakePublisher is a programmable EventPublisher mirroring fakeStorage.
type fakePublisher struct {
	err   error
	calls int
}

func (f *fakePublisher) PublishAvatarUploaded(context.Context, domain.AvatarUploadEvent) error {
	f.calls++
	return f.err
}

func (f *fakePublisher) PublishAvatarDeleted(context.Context, domain.AvatarDeleteEvent) error {
	f.calls++
	return f.err
}

func TestPublisher_PassesThroughAndOpens(t *testing.T) {
	fake := &fakePublisher{err: errors.New("broker down")}
	p := resilience.NewPublisher(fake, resilience.Settings{
		MinRequests:  3,
		FailureRatio: 0.6,
		Timeout:      time.Minute,
	}, discardLogger())

	for i := 0; i < 3; i++ {
		require.Error(t, p.PublishAvatarUploaded(context.Background(), domain.AvatarUploadEvent{}))
	}
	require.Equal(t, 3, fake.calls)

	err := p.PublishAvatarDeleted(context.Background(), domain.AvatarDeleteEvent{})
	require.ErrorIs(t, err, gobreaker.ErrOpenState)
	require.Equal(t, 3, fake.calls)
}
