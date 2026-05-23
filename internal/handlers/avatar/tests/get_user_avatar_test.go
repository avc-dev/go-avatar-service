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

func mountGetUserAvatar(h http.HandlerFunc) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/users/{user_id}/avatar", h)
	return r
}

func TestGetUserAvatar(t *testing.T) {
	t.Run("happy: 200 with binary body", func(t *testing.T) {
		h, svc := newHandler(t)
		userID := uuid.Must(uuid.NewV7())
		avatarID := uuid.Must(uuid.NewV7())

		svc.EXPECT().
			GetCurrentByUserID(anyCtx(), userID).
			Return(&domain.Avatar{ID: avatarID, UserID: userID, S3Key: "originals/x.jpg"}, nil)

		payload := "binary"
		svc.EXPECT().
			DownloadOriginal(anyCtx(), avatarID).
			Return(&svcavatar.DownloadResult{
				Reader:      io.NopCloser(strings.NewReader(payload)),
				ContentType: "image/jpeg",
				Size:        int64(len(payload)),
				Avatar:      &domain.Avatar{ID: avatarID},
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+userID.String()+"/avatar", nil)
		rec := serve(mountGetUserAvatar(h.GetUserAvatar), req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
		require.Equal(t, payload, rec.Body.String())
	})

	t.Run("no current avatar: 404", func(t *testing.T) {
		h, svc := newHandler(t)
		userID := uuid.Must(uuid.NewV7())
		svc.EXPECT().
			GetCurrentByUserID(anyCtx(), userID).
			Return(nil, svcavatar.ErrNotFound)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+userID.String()+"/avatar", nil)
		rec := serve(mountGetUserAvatar(h.GetUserAvatar), req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("not a uuid v7: 400", func(t *testing.T) {
		h, _ := newHandler(t)
		// google/uuid v4 fixture — parses successfully but Version() != 7.
		uuidV4 := "f81d4fae-7dec-11d0-a765-00a0c91e6bf6"
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+uuidV4+"/avatar", nil)
		rec := serve(mountGetUserAvatar(h.GetUserAvatar), req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
