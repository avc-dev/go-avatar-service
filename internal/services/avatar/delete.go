package avatar

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

// Delete soft-deletes the avatar after verifying that requestingUserID owns
// it. On success an AvatarDeleteEvent is published so the worker can evict
// the original and any thumbnails from object storage.
//
// Returns ErrNotFound if the avatar does not exist or is already deleted,
// ErrForbidden if it exists but is owned by another user. A publisher failure
// after a successful soft-delete is logged as a WARN and does not propagate —
// the avatar is gone from the user's POV regardless of broker availability.
func (s *Service) Delete(ctx context.Context, id, requestingUserID uuid.UUID) error {
	a, err := s.repo.DeleteByOwner(ctx, id, requestingUserID)
	if err != nil {
		switch {
		case errors.Is(err, repoavatar.ErrNotFound):
			err = ErrNotFound
		case errors.Is(err, repoavatar.ErrForbidden):
			err = ErrForbidden
		}
		return fmt.Errorf("delete avatar %s by user %s: %w", id, requestingUserID, err)
	}

	if pubErr := s.publisher.PublishAvatarDeleted(ctx, domain.AvatarDeleteEvent{
		AvatarID: a.ID,
		S3Keys:   collectS3Keys(a),
	}); pubErr != nil {
		s.log.Warn("avatar soft-deleted but delete event was not published",
			"avatar_id", a.ID,
			"err", pubErr,
		)
	}

	return nil
}

// collectS3Keys gathers every object-storage key associated with an avatar
// (the original plus all generated thumbnails) so the worker can delete them
// in a single event handler invocation. Order is not significant.
func collectS3Keys(a *domain.Avatar) []string {
	keys := make([]string, 0, len(a.ThumbnailS3Keys)+1)
	keys = append(keys, a.S3Key)
	for _, key := range a.ThumbnailS3Keys {
		keys = append(keys, key)
	}
	return keys
}
