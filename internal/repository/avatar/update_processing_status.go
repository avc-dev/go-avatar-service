package avatar

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// UpdateProcessingStatus updates the processing_status and thumbnail keys for
// an avatar. Returns ErrNotFound if no live row matches.
func (r *PostgresRepository) UpdateProcessingStatus(
	ctx context.Context,
	id uuid.UUID,
	status domain.ProcessingStatus,
	thumbs map[string]string,
) error {
	thumbsJSON, err := marshalThumbs(thumbs)
	if err != nil {
		return fmt.Errorf("marshal thumbnail keys: %w", err)
	}

	const query = `
		UPDATE avatars
		SET processing_status = $2, thumbnail_s3_keys = $3
		WHERE id = $1 AND deleted_at IS NULL`

	tag, err := r.pool.Exec(ctx, query, id, string(status), thumbsJSON)
	if err != nil {
		return fmt.Errorf("update processing status for avatar %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update processing status for avatar %s: %w", id, ErrNotFound)
	}
	return nil
}
