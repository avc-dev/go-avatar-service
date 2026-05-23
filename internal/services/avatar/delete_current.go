package avatar

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

// DeleteCurrentByUserID looks up the user's current (most recently created)
// avatar and soft-deletes it. Ownership is implicit: only avatars owned by
// userID can be the current one, so the underlying Delete with userID as the
// requester always passes the ownership check (barring concurrent operations
// that the caller is expected to tolerate).
//
// Returns ErrNotFound if the user has no current avatar.
func (s *Service) DeleteCurrentByUserID(ctx context.Context, userID uuid.UUID) error {
	current, err := s.repo.GetCurrentByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repoavatar.ErrNotFound) {
			err = ErrNotFound
		}
		return fmt.Errorf("delete current avatar for user %s: %w", userID, err)
	}
	return s.Delete(ctx, current.ID, userID)
}
