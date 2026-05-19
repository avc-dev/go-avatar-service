package avatar_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/avc-dev/go-avatar-service/internal/services/avatar"
	"github.com/avc-dev/go-avatar-service/internal/services/avatar/mocks"
)

// newService builds a Service wired with fresh mocks for each test. The
// returned mocks register cleanup hooks that assert all expectations were
// met when the test ends.
func newService(t *testing.T) (
	*avatar.Service,
	*mocks.MockAvatarRepository,
	*mocks.MockObjectStorage,
	*mocks.MockEventPublisher,
) {
	t.Helper()
	repo := mocks.NewMockAvatarRepository(t)
	st := mocks.NewMockObjectStorage(t)
	pub := mocks.NewMockEventPublisher(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return avatar.New(repo, st, pub, log), repo, st, pub
}
