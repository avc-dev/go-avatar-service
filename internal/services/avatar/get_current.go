package avatar

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

// GetCurrentByUserID returns the user's latest non-deleted avatar, or
// ErrNotFound when the user has none.
func (s *Service) GetCurrentByUserID(ctx context.Context, userID uuid.UUID) (*domain.Avatar, error) {
	a, err := s.repo.GetCurrentByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repoavatar.ErrNotFound) {
			err = ErrNotFound
		}
		return nil, fmt.Errorf("get current avatar for user %s: %w", userID, err)
	}
	return a, nil
}
