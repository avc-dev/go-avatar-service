package avatar

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// SoftDelete marks the avatar as deleted. Returns ErrNotFound if the row does
// not exist or was already deleted.
func (r *PostgresRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE avatars SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`

	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("soft delete avatar %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("soft delete avatar %s: %w", id, ErrNotFound)
	}
	return nil
}
