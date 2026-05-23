package avatar

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// GetCurrentByUserID returns the most-recently-created non-deleted avatar for
// the given user, or ErrNotFound.
func (r *PostgresRepository) GetCurrentByUserID(ctx context.Context, userID uuid.UUID) (*domain.Avatar, error) {
	query := `SELECT ` + avatarColumns + `
		FROM avatars
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1`
	row := r.pool.QueryRow(ctx, query, userID)
	a, err := scanAvatar(row)
	if err != nil {
		return nil, fmt.Errorf("get current avatar for user %s: %w", userID, err)
	}
	return a, nil
}
