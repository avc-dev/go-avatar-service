package avatar

import (
	"time"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// UploadResponse is the 201 body for POST /api/v1/avatars.
type UploadResponse struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	URL       string    `json:"url"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// MetadataResponse is the 200 body for GET /api/v1/avatars/{id}/metadata and
// the per-item shape inside ListResponse. Width/height are intentionally
// omitted for MVP: the schema does not yet store image dimensions; the
// worker will populate them in Iteration 4.
type MetadataResponse struct {
	ID         uuid.UUID        `json:"id"`
	UserID     uuid.UUID        `json:"user_id"`
	FileName   string           `json:"file_name"`
	MIMEType   string           `json:"mime_type"`
	Size       int64            `json:"size"`
	Thumbnails []ThumbnailEntry `json:"thumbnails"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

// ThumbnailEntry describes a single rendered thumbnail size.
type ThumbnailEntry struct {
	Size string `json:"size"`
	URL  string `json:"url"`
}

// ListResponse is the 200 body for GET /api/v1/users/{user_id}/avatars.
// Avatars is intentionally not omitempty: callers should see an empty array
// rather than a missing key when the user has no avatars yet.
type ListResponse struct {
	Avatars []MetadataResponse `json:"avatars"`
}

// toMetadataResponse converts a domain Avatar into the wire shape. Thumbnails
// is always a non-nil slice (possibly empty) so json.Marshal emits `[]` rather
// than `null` for the "no thumbnails yet" case.
func toMetadataResponse(a *domain.Avatar) MetadataResponse {
	thumbs := make([]ThumbnailEntry, 0, len(a.ThumbnailS3Keys))
	for size := range a.ThumbnailS3Keys {
		// TODO(iter-4): once a dedicated thumbnail download endpoint exists,
		// populate URL with e.g. "/api/v1/avatars/{id}?size={size}".
		thumbs = append(thumbs, ThumbnailEntry{Size: size, URL: ""})
	}
	return MetadataResponse{
		ID:         a.ID,
		UserID:     a.UserID,
		FileName:   a.FileName,
		MIMEType:   a.MIMEType,
		Size:       a.SizeBytes,
		Thumbnails: thumbs,
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
	}
}
