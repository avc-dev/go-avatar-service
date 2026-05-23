package avatar_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/handlers/avatar"
	"github.com/avc-dev/go-avatar-service/internal/middleware"
	svcavatar "github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

func mountDeleteUserAvatar(h *avatar.Handler) http.Handler {
	r := chi.NewRouter()
	r.With(middleware.UserID).Delete("/api/v1/users/{user_id}/avatar", h.DeleteUserAvatar)
	return r
}

func TestDeleteUserAvatar(t *testing.T) {
	t.Run("happy (auth user == path user): 204", func(t *testing.T) {
		h, svc := newHandler(t)
		userID := uuid.Must(uuid.NewV7())
		svc.EXPECT().DeleteCurrentByUserID(anyCtx(), userID).Return(nil)

		req := newRequestWithUser(http.MethodDelete, "/api/v1/users/"+userID.String()+"/avatar", nil, userID)
		rec := serve(mountDeleteUserAvatar(h), req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("auth user != path user: 403", func(t *testing.T) {
		h, _ := newHandler(t)
		authUser := uuid.Must(uuid.NewV7())
		pathUser := uuid.Must(uuid.NewV7())

		req := newRequestWithUser(http.MethodDelete, "/api/v1/users/"+pathUser.String()+"/avatar", nil, authUser)
		rec := serve(mountDeleteUserAvatar(h), req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("service ErrNotFound: 404", func(t *testing.T) {
		h, svc := newHandler(t)
		userID := uuid.Must(uuid.NewV7())
		svc.EXPECT().DeleteCurrentByUserID(anyCtx(), userID).Return(svcavatar.ErrNotFound)

		req := newRequestWithUser(http.MethodDelete, "/api/v1/users/"+userID.String()+"/avatar", nil, userID)
		rec := serve(mountDeleteUserAvatar(h), req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})
}
