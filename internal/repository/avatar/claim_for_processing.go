package avatar

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// ClaimForProcessing atomically transitions an avatar from
// processing_status='pending' to 'processing' and returns the now-claimed
// row. The UPDATE has WHERE processing_status='pending' baked in so at most
// one worker can win the race when the same message is delivered (e.g. via
// at-least-once redelivery or two replicas consuming the same queue).
//
// Semantics from the worker's perspective:
//   - row claimed       → (avatar, nil); proceed with thumbnail generation.
//   - already non-pending → ErrNotFound; ack the message and move on.
//   - missing or soft-deleted → ErrNotFound; same handling.
//
// The worker cannot meaningfully distinguish "another worker has it" from
// "the row is gone" — both mean "this message is not actionable" — so the
// repository deliberately collapses the two into one sentinel.
func (r *PostgresRepository) ClaimForProcessing(ctx context.Context, id uuid.UUID) (*domain.Avatar, error) {
	const query = `
		UPDATE avatars
		SET processing_status = $2::processing_status, updated_at = NOW()
		WHERE id = $1
		  AND deleted_at IS NULL
		  AND processing_status = $3::processing_status
		RETURNING ` + avatarColumns

	row := r.pool.QueryRow(ctx, query, id,
		string(domain.ProcessingStatusProcessing),
		string(domain.ProcessingStatusPending),
	)
	a, err := scanAvatar(row)
	if err != nil {
		return nil, fmt.Errorf("claim avatar %s for processing: %w", id, err)
	}
	return a, nil
}
