package avatar

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// ListByUserID returns all non-deleted avatars for the given user, ordered by
// created_at DESC. Returns an empty slice (not an error) when no avatars exist
// for the user.
func (r *PostgresRepository) ListByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Avatar, error) {
	query := `SELECT ` + avatarColumns + `
		FROM avatars
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list avatars for user %s: %w", userID, err)
	}
	defer rows.Close()

	out := make([]*domain.Avatar, 0)
	for rows.Next() {
		a, err := scanAvatar(rows)
		if err != nil {
			return nil, fmt.Errorf("list avatars for user %s: %w", userID, err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list avatars for user %s: %w", userID, err)
	}
	return out, nil
}
