package avatar

import (
	"fmt"
	"net/url"
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
// omitted: the schema does not yet store image dimensions and there's no
// pressing use case for them right now.
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
// than `null` for the "no thumbnails yet" case. URLs point back to the Download
// endpoint with the ?size= query selector — clients use the metadata response
// as the directory of available variants.
func toMetadataResponse(a *domain.Avatar) MetadataResponse {
	thumbs := make([]ThumbnailEntry, 0, len(a.ThumbnailS3Keys))
	for size := range a.ThumbnailS3Keys {
		// QueryEscape guards against unexpected characters in the map keys —
		// today they're whitelisted constants but the URL stays well-formed
		// even if that invariant is ever loosened (e.g. user-defined sizes).
		thumbs = append(thumbs, ThumbnailEntry{
			Size: size,
			URL:  fmt.Sprintf("/api/v1/avatars/%s?size=%s", a.ID, url.QueryEscape(size)),
		})
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
