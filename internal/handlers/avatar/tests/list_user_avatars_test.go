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
)

func mountList(h http.HandlerFunc) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/users/{user_id}/avatars", h)
	return r
}

func TestListUserAvatars(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	t.Run("happy with items: 200 with populated list", func(t *testing.T) {
		h, svc := newHandler(t)
		now := time.Now().UTC().Truncate(time.Microsecond)
		a1 := &domain.Avatar{
			ID: uuid.Must(uuid.NewV7()), UserID: userID,
			FileName: "a.jpg", MIMEType: "image/jpeg", SizeBytes: 1,
			CreatedAt: now, UpdatedAt: now,
		}
		a2 := &domain.Avatar{
			ID: uuid.Must(uuid.NewV7()), UserID: userID,
			FileName: "b.png", MIMEType: "image/png", SizeBytes: 2,
			CreatedAt: now, UpdatedAt: now,
		}
		svc.EXPECT().ListByUserID(anyCtx(), userID).Return([]*domain.Avatar{a1, a2}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+userID.String()+"/avatars", nil)
		rec := serve(mountList(h.ListUserAvatars), req)

		require.Equal(t, http.StatusOK, rec.Code)
		var got struct {
			Avatars []map[string]any `json:"avatars"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Len(t, got.Avatars, 2)
		require.Equal(t, a1.ID.String(), got.Avatars[0]["id"])
		require.Equal(t, a2.ID.String(), got.Avatars[1]["id"])
	})

	t.Run("happy empty: 200 with avatars:[]", func(t *testing.T) {
		h, svc := newHandler(t)
		svc.EXPECT().ListByUserID(anyCtx(), userID).Return(nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+userID.String()+"/avatars", nil)
		rec := serve(mountList(h.ListUserAvatars), req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), `"avatars":[]`)
	})

	t.Run("not a uuid v7 in path: 400", func(t *testing.T) {
		h, _ := newHandler(t)
		uuidV4 := "f81d4fae-7dec-11d0-a765-00a0c91e6bf6"
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+uuidV4+"/avatars", nil)
		rec := serve(mountList(h.ListUserAvatars), req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
