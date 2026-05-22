package avatar

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// DeleteByOwner soft-deletes the avatar only if requestingUserID owns it and
// returns the deleted row (already carrying the populated deleted_at). The
// happy-path is a single atomic UPDATE ... RETURNING; an extra SELECT is
// performed only when the UPDATE affects zero rows, to differentiate "not
// found" from "different owner".
//
// Returns:
//   - the deleted avatar and nil error on success
//   - nil and ErrForbidden if the avatar exists but belongs to a different user
//   - nil and ErrNotFound if the avatar does not exist or was already soft-deleted
func (r *PostgresRepository) DeleteByOwner(ctx context.Context, id, requestingUserID uuid.UUID) (*domain.Avatar, error) {
	query := `
		UPDATE avatars
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		RETURNING ` + avatarColumns

	row := r.pool.QueryRow(ctx, query, id, requestingUserID)
	a, err := scanAvatar(row)
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("delete avatar %s by user %s: %w", id, requestingUserID, err)
	}

	// UPDATE affected zero rows — either the avatar does not exist (or is
	// already deleted), or it exists but belongs to a different user.
	const checkQuery = `SELECT user_id FROM avatars WHERE id = $1 AND deleted_at IS NULL`
	var ownerID uuid.UUID
	if scanErr := r.pool.QueryRow(ctx, checkQuery, id).Scan(&ownerID); scanErr != nil {
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return nil, fmt.Errorf("delete avatar %s by user %s: %w", id, requestingUserID, ErrNotFound)
		}
		return nil, fmt.Errorf("delete avatar %s by user %s: check ownership: %w", id, requestingUserID, scanErr)
	}
	if ownerID != requestingUserID {
		return nil, fmt.Errorf("delete avatar %s by user %s: %w", id, requestingUserID, ErrForbidden)
	}
	// Defensive fallback: row exists, owned by requester, but RETURNING
	// missed. Only reachable under a concurrent soft-delete that is itself
	// guarded by the same WHERE clause — effectively impossible.
	return nil, fmt.Errorf("delete avatar %s by user %s: %w", id, requestingUserID, ErrNotFound)
}
