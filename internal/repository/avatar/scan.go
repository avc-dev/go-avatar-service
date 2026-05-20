package avatar

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// avatarColumns is the canonical column list for SELECT queries against the
// avatars table. Keep in sync with the schema and with the scan order in
// scanAvatar.
const avatarColumns = `id, user_id, file_name, mime_type, size_bytes, s3_key,
	thumbnail_s3_keys, upload_status, processing_status,
	created_at, updated_at, deleted_at`

// rowScanner abstracts pgx.Row and pgx.Rows so scanAvatar works for both
// QueryRow and Query-loop call sites.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanAvatar materialises an Avatar from a single result row, mapping JSONB
// thumbnails and translating pgx.ErrNoRows into ErrNotFound.
//
// scanAvatar deliberately returns sentinels and raw scan errors *unwrapped*
// — every caller already adds operation-level context (e.g. "get avatar by
// id X: ..."), so wrapping here would just produce noisy stacks like
// "get avatar by id X: scan avatar: avatar not found" with no extra signal.
func scanAvatar(row rowScanner) (*domain.Avatar, error) {
	var (
		a          domain.Avatar
		thumbsRaw  []byte
		uploadStr  string
		processStr string
		deletedAt  *time.Time
	)
	err := row.Scan(
		&a.ID, &a.UserID, &a.FileName, &a.MIMEType, &a.SizeBytes, &a.S3Key,
		&thumbsRaw, &uploadStr, &processStr,
		&a.CreatedAt, &a.UpdatedAt, &deletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.UploadStatus = domain.UploadStatus(uploadStr)
	a.ProcessingStatus = domain.ProcessingStatus(processStr)
	a.DeletedAt = deletedAt
	if thumbsRaw != nil {
		if err := json.Unmarshal(thumbsRaw, &a.ThumbnailS3Keys); err != nil {
			return nil, fmt.Errorf("unmarshal thumbnail keys: %w", err)
		}
	}
	return &a, nil
}

// marshalThumbs converts the thumbnail map to a JSONB-friendly representation.
// A nil map becomes SQL NULL; an empty map becomes the JSON object "{}".
func marshalThumbs(thumbs map[string]string) (any, error) {
	if thumbs == nil {
		return nil, nil
	}
	b, err := json.Marshal(thumbs)
	if err != nil {
		return nil, err
	}
	return b, nil
}
