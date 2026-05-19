package avatar

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// GetByID returns the avatar with the given id. Soft-deleted rows are not
// returned and produce ErrNotFound.
func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Avatar, error) {
	query := `SELECT ` + avatarColumns + ` FROM avatars WHERE id = $1 AND deleted_at IS NULL`
	row := r.pool.QueryRow(ctx, query, id)
	a, err := scanAvatar(row)
	if err != nil {
		return nil, fmt.Errorf("get avatar by id %s: %w", id, err)
	}
	return a, nil
}
