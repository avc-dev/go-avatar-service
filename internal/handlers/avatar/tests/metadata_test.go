package avatar_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	svcavatar "github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

func mountMetadata(h http.HandlerFunc) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/avatars/{avatar_id}/metadata", h)
	return r
}

func TestMetadata(t *testing.T) {
	t.Run("happy: 200, thumbnails serialised as empty array", func(t *testing.T) {
		h, svc := newHandler(t)
		id := uuid.Must(uuid.NewV7())
		userID := uuid.Must(uuid.NewV7())
		now := time.Now().UTC().Truncate(time.Microsecond)

		svc.EXPECT().
			GetMetadata(anyCtx(), id).
			Return(&domain.Avatar{
				ID:        id,
				UserID:    userID,
				FileName:  "p.png",
				MIMEType:  "image/png",
				SizeBytes: 12345,
				CreatedAt: now,
				UpdatedAt: now,
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+id.String()+"/metadata", nil)
		rec := serve(mountMetadata(h.Metadata), req)

		require.Equal(t, http.StatusOK, rec.Code)
		// Ensure thumbnails is the empty array literal, not null.
		require.Contains(t, rec.Body.String(), `"thumbnails":[]`)

		var got map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Equal(t, id.String(), got["id"])
		require.Equal(t, "p.png", got["file_name"])
		require.Equal(t, "image/png", got["mime_type"])
	})

	t.Run("not found: 404", func(t *testing.T) {
		h, svc := newHandler(t)
		id := uuid.Must(uuid.NewV7())
		svc.EXPECT().GetMetadata(anyCtx(), id).Return(nil, svcavatar.ErrNotFound)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+id.String()+"/metadata", nil)
		rec := serve(mountMetadata(h.Metadata), req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("with thumbnails: URLs point back to Download endpoint", func(t *testing.T) {
		h, svc := newHandler(t)
		id := uuid.Must(uuid.NewV7())
		userID := uuid.Must(uuid.NewV7())
		now := time.Now().UTC().Truncate(time.Microsecond)

		svc.EXPECT().
			GetMetadata(anyCtx(), id).
			Return(&domain.Avatar{
				ID:        id,
				UserID:    userID,
				FileName:  "p.png",
				MIMEType:  "image/png",
				SizeBytes: 12345,
				ThumbnailS3Keys: map[string]string{
					"100x100": "thumbnails/" + id.String() + "/100x100.jpg",
					"300x300": "thumbnails/" + id.String() + "/300x300.jpg",
				},
				CreatedAt: now,
				UpdatedAt: now,
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+id.String()+"/metadata", nil)
		rec := serve(mountMetadata(h.Metadata), req)

		require.Equal(t, http.StatusOK, rec.Code)

		var got struct {
			Thumbnails []struct {
				Size string `json:"size"`
				URL  string `json:"url"`
			} `json:"thumbnails"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Len(t, got.Thumbnails, 2)

		// Order is non-deterministic (Go map iteration). Convert to a map for
		// stable assertions.
		urlBySize := make(map[string]string, 2)
		for _, th := range got.Thumbnails {
			urlBySize[th.Size] = th.URL
		}
		require.Equal(t, "/api/v1/avatars/"+id.String()+"?size=100x100", urlBySize["100x100"])
		require.Equal(t, "/api/v1/avatars/"+id.String()+"?size=300x300", urlBySize["300x300"])
	})
}
