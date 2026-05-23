package avatar_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"github.com/avc-dev/go-avatar-service/internal/handlers/avatar"
	"github.com/avc-dev/go-avatar-service/internal/handlers/avatar/mocks"
)

// anyCtx is a small wrapper around mock.Anything used everywhere we don't
// need to assert on the exact context value (which is all the handler tests).
func anyCtx() interface{} { return mock.Anything }

// testMaxUploadBytes is small on purpose so the 413 case is easy to trigger
// without allocating MBs of fake payload.
const testMaxUploadBytes int64 = 1024

// newHandler builds a Handler wired with a fresh MockService. The mock
// registers a cleanup hook asserting all expectations were met.
func newHandler(t *testing.T) (*avatar.Handler, *mocks.MockService) {
	t.Helper()
	svc := mocks.NewMockService(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return avatar.NewHandler(svc, log, testMaxUploadBytes), svc
}

// newRequestWithUser builds an *http.Request carrying the X-User-ID header so
// a middleware.UserID-wrapped handler admits it.
func newRequestWithUser(method, target string, body io.Reader, userID uuid.UUID) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.Header.Set("X-User-ID", userID.String())
	return r
}

// serve drives a handler with the given request and returns the recorder for
// assertion. Tests that need URL params should wrap the handler in a
// chi.Router via chi.NewRouter — see download_test.go for the canonical
// pattern.
func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// jpegHeader is the JPEG SOI + APP0/JFIF marker. Twenty bytes is enough for
// gabriel-vasile/mimetype to classify the buffer as image/jpeg; tests append
// arbitrary payload to it to simulate a "real" upload.
var jpegHeader = []byte{
	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10,
	'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00,
}

// fakeJPEG returns bytes that start with the JPEG magic sequence and end with
// the supplied trailer. Magic-byte sniffing only inspects the head, so the
// trailer can be anything — tests use it to assert exact lengths or to add
// uniqueness across cases.
func fakeJPEG(trailer ...byte) []byte {
	out := make([]byte, 0, len(jpegHeader)+len(trailer))
	out = append(out, jpegHeader...)
	out = append(out, trailer...)
	return out
}
