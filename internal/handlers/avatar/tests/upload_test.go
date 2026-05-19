package avatar_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	"github.com/avc-dev/go-avatar-service/internal/middleware"
	svcavatar "github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

// buildMultipart returns a body and the corresponding Content-Type header.
// fieldName lets tests assert that the handler looks at the canonical "file"
// field and rejects bodies that lack it.
func buildMultipart(t *testing.T, fieldName, filename, contentType string, payload []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)

	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+filename+`"`)
	if contentType != "" {
		hdr.Set("Content-Type", contentType)
	}
	part, err := mw.CreatePart(hdr)
	require.NoError(t, err)
	_, err = part.Write(payload)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	return body, mw.FormDataContentType()
}

func TestUpload(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	t.Run("happy path: 201 with UploadResponse", func(t *testing.T) {
		h, svc := newHandler(t)

		payload := []byte("fake image")
		body, ct := buildMultipart(t, "file", "selfie.jpg", "image/jpeg", payload)

		now := time.Now().UTC().Truncate(time.Microsecond)
		avatarID := uuid.Must(uuid.NewV7())
		svc.EXPECT().
			Upload(mock.Anything, mock.MatchedBy(func(p svcavatar.UploadParams) bool {
				return p.UserID == userID &&
					p.FileName == "selfie.jpg" &&
					p.MIMEType == "image/jpeg" &&
					p.Size == int64(len(payload))
			})).
			Return(&domain.Avatar{
				ID:        avatarID,
				UserID:    userID,
				CreatedAt: now,
			}, nil)

		req := newRequestWithUser(http.MethodPost, "/api/v1/avatars", body, userID)
		req.Header.Set("Content-Type", ct)

		rec := serve(middleware.UserID(http.HandlerFunc(h.Upload)), req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var got map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Equal(t, avatarID.String(), got["id"])
		require.Equal(t, userID.String(), got["user_id"])
		require.Equal(t, "/api/v1/avatars/"+avatarID.String(), got["url"])
		require.Equal(t, "processing", got["status"])
		require.NotEmpty(t, got["created_at"])
	})

	t.Run("missing X-User-ID context: 500 (handler is defensive against route misconfig)", func(t *testing.T) {
		h, _ := newHandler(t)

		// Call the handler directly — no UserID middleware in front of it.
		body, ct := buildMultipart(t, "file", "x.jpg", "image/jpeg", []byte("x"))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", body)
		req.Header.Set("Content-Type", ct)

		rec := serve(http.HandlerFunc(h.Upload), req)
		require.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("missing file field: 400", func(t *testing.T) {
		h, _ := newHandler(t)

		// Send a multipart body with a field named other than "file".
		body, ct := buildMultipart(t, "not_file", "x.jpg", "image/jpeg", []byte("x"))
		req := newRequestWithUser(http.MethodPost, "/api/v1/avatars", body, userID)
		req.Header.Set("Content-Type", ct)

		rec := serve(middleware.UserID(http.HandlerFunc(h.Upload)), req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), "file field is required")
	})

	t.Run("body exceeds limit: 413 with max_size", func(t *testing.T) {
		h, _ := newHandler(t)

		oversized := bytes.Repeat([]byte("A"), int(testMaxUploadBytes)+1024)
		body, ct := buildMultipart(t, "file", "big.bin", "application/octet-stream", oversized)

		req := newRequestWithUser(http.MethodPost, "/api/v1/avatars", body, userID)
		req.Header.Set("Content-Type", ct)

		rec := serve(middleware.UserID(http.HandlerFunc(h.Upload)), req)
		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)

		var got map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Equal(t, "File too large", got["error"])
		require.NotNil(t, got["max_size"], "413 body must include max_size for the client UI")
	})

	t.Run("service.Upload returns generic error: 500", func(t *testing.T) {
		h, svc := newHandler(t)

		body, ct := buildMultipart(t, "file", "x.jpg", "image/jpeg", []byte("x"))
		svc.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(nil, errors.New("boom"))

		req := newRequestWithUser(http.MethodPost, "/api/v1/avatars", body, userID)
		req.Header.Set("Content-Type", ct)

		rec := serve(middleware.UserID(http.HandlerFunc(h.Upload)), req)
		require.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	// Silence unused-import warning when the file is trimmed.
	_ = io.Discard
	_ = strings.NewReader
}
