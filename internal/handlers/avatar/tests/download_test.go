package avatar_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	svcavatar "github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

// mountDownload wraps the Download handler in a chi router so {avatar_id} is
// populated from the URL the same way it will be in production.
func mountDownload(h http.HandlerFunc) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/avatars/{avatar_id}", h)
	return r
}

func TestDownload(t *testing.T) {
	t.Run("happy: 200 with Content-Type, Content-Length, body", func(t *testing.T) {
		h, svc := newHandler(t)
		id := uuid.Must(uuid.NewV7())
		payload := "binary image data"
		svc.EXPECT().
			DownloadOriginal(anyCtx(), id).
			Return(&svcavatar.DownloadResult{
				Reader:      io.NopCloser(strings.NewReader(payload)),
				ContentType: "image/jpeg",
				Size:        int64(len(payload)),
				Avatar:      &domain.Avatar{ID: id},
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+id.String(), nil)
		rec := serve(mountDownload(h.Download), req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
		require.Equal(t, "17", rec.Header().Get("Content-Length"))
		require.Equal(t, payload, rec.Body.String())
	})

	t.Run("invalid UUID in path: 400", func(t *testing.T) {
		h, _ := newHandler(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/not-a-uuid", nil)
		rec := serve(mountDownload(h.Download), req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("service returns ErrNotFound: 404", func(t *testing.T) {
		h, svc := newHandler(t)
		id := uuid.Must(uuid.NewV7())
		svc.EXPECT().
			DownloadOriginal(anyCtx(), id).
			Return(nil, svcavatar.ErrNotFound)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+id.String(), nil)
		rec := serve(mountDownload(h.Download), req)
		require.Equal(t, http.StatusNotFound, rec.Code)
		require.Contains(t, rec.Body.String(), "Avatar not found")
	})

	t.Run("?size=100x100: dispatches to DownloadThumbnail", func(t *testing.T) {
		h, svc := newHandler(t)
		id := uuid.Must(uuid.NewV7())
		payload := "thumb-100"
		svc.EXPECT().
			DownloadThumbnail(anyCtx(), id, "100x100").
			Return(&svcavatar.DownloadResult{
				Reader:      io.NopCloser(strings.NewReader(payload)),
				ContentType: "image/jpeg",
				Size:        int64(len(payload)),
				Avatar:      &domain.Avatar{ID: id},
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+id.String()+"?size=100x100", nil)
		rec := serve(mountDownload(h.Download), req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, payload, rec.Body.String())
	})

	t.Run("?size=original: still dispatches to DownloadOriginal", func(t *testing.T) {
		h, svc := newHandler(t)
		id := uuid.Must(uuid.NewV7())
		payload := "orig"
		svc.EXPECT().
			DownloadOriginal(anyCtx(), id).
			Return(&svcavatar.DownloadResult{
				Reader:      io.NopCloser(strings.NewReader(payload)),
				ContentType: "image/jpeg",
				Size:        int64(len(payload)),
				Avatar:      &domain.Avatar{ID: id},
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+id.String()+"?size=original", nil)
		rec := serve(mountDownload(h.Download), req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, payload, rec.Body.String())
	})

	t.Run("?size=unknown: thumbnail service returns ErrNotFound → 404", func(t *testing.T) {
		h, svc := newHandler(t)
		id := uuid.Must(uuid.NewV7())
		svc.EXPECT().
			DownloadThumbnail(anyCtx(), id, "99x99").
			Return(nil, svcavatar.ErrNotFound)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+id.String()+"?size=99x99", nil)
		rec := serve(mountDownload(h.Download), req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})
}
