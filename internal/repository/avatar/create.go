package avatar

import (
	"context"
	"fmt"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// Create inserts a new avatar row. The caller is expected to have already
// populated a.ID (e.g. via uuid.NewV7). If CreatedAt/UpdatedAt are zero, the
// DB defaults to NOW(); otherwise the caller-provided values are persisted.
// After a successful insert, a.CreatedAt and a.UpdatedAt are refreshed from
// the row's RETURNING clause.
func (r *PostgresRepository) Create(ctx context.Context, a *domain.Avatar) error {
	thumbs, err := marshalThumbs(a.ThumbnailS3Keys)
	if err != nil {
		return fmt.Errorf("marshal thumbnail keys: %w", err)
	}

	const query = `
		INSERT INTO avatars (
			id, user_id, file_name, mime_type, size_bytes, s3_key,
			thumbnail_s3_keys, upload_status, processing_status,
			created_at, updated_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9,
			COALESCE($10, NOW()), COALESCE($11, NOW()), $12
		)
		RETURNING created_at, updated_at`

	var createdArg, updatedArg any
	if !a.CreatedAt.IsZero() {
		createdArg = a.CreatedAt
	}
	if !a.UpdatedAt.IsZero() {
		updatedArg = a.UpdatedAt
	}

	err = r.pool.QueryRow(ctx, query,
		a.ID, a.UserID, a.FileName, a.MIMEType, a.SizeBytes, a.S3Key,
		thumbs, string(a.UploadStatus), string(a.ProcessingStatus),
		createdArg, updatedArg, a.DeletedAt,
	).Scan(&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert avatar: %w", err)
	}
	return nil
}
