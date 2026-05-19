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

// mountDelete wires the Delete handler through chi+middleware.UserID so URL
// params and the auth-context are both populated as in production.
func mountDelete(h *avatar.Handler) http.Handler {
	r := chi.NewRouter()
	r.With(middleware.UserID).Delete("/api/v1/avatars/{avatar_id}", h.Delete)
	return r
}

func TestDelete(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	t.Run("happy: 204", func(t *testing.T) {
		h, svc := newHandler(t)
		id := uuid.Must(uuid.NewV7())
		svc.EXPECT().Delete(anyCtx(), id, userID).Return(nil)

		req := newRequestWithUser(http.MethodDelete, "/api/v1/avatars/"+id.String(), nil, userID)
		rec := serve(mountDelete(h), req)

		require.Equal(t, http.StatusNoContent, rec.Code)
		require.Empty(t, rec.Body.String())
	})

	t.Run("not found: 404", func(t *testing.T) {
		h, svc := newHandler(t)
		id := uuid.Must(uuid.NewV7())
		svc.EXPECT().Delete(anyCtx(), id, userID).Return(svcavatar.ErrNotFound)

		req := newRequestWithUser(http.MethodDelete, "/api/v1/avatars/"+id.String(), nil, userID)
		rec := serve(mountDelete(h), req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("forbidden: 403", func(t *testing.T) {
		h, svc := newHandler(t)
		id := uuid.Must(uuid.NewV7())
		svc.EXPECT().Delete(anyCtx(), id, userID).Return(svcavatar.ErrForbidden)

		req := newRequestWithUser(http.MethodDelete, "/api/v1/avatars/"+id.String(), nil, userID)
		rec := serve(mountDelete(h), req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("invalid UUID: 400", func(t *testing.T) {
		h, _ := newHandler(t)
		req := newRequestWithUser(http.MethodDelete, "/api/v1/avatars/not-a-uuid", nil, userID)
		rec := serve(mountDelete(h), req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
