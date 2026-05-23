package avatar

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// ListByUserID returns all non-deleted avatars for the given user, ordered
// newest first. An empty list is a valid result — not an error.
func (s *Service) ListByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Avatar, error) {
	list, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list avatars for user %s: %w", userID, err)
	}
	return list, nil
}
