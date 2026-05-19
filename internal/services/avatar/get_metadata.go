package avatar

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

// GetMetadata returns the avatar record without fetching the binary. Soft-deleted
// avatars are reported as ErrNotFound.
func (s *Service) GetMetadata(ctx context.Context, id uuid.UUID) (*domain.Avatar, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repoavatar.ErrNotFound) {
			err = ErrNotFound
		}
		return nil, fmt.Errorf("get avatar metadata for %s: %w", id, err)
	}
	return a, nil
}
