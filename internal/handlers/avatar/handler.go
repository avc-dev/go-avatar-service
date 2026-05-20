// Package avatar contains the HTTP handler layer for the avatar aggregate.
//
// The package defines a consumer-side AvatarService interface that lists
// exactly the service methods the HTTP layer needs. The concrete
// *services/avatar.Service satisfies this interface structurally — handlers
// stay decoupled from extra service methods they don't use and the mock can
// be precisely scoped for handler tests.
//
// Each public Handler method lives in its own file (upload.go, download.go,
// ...) to keep diffs focused and per-endpoint logic easy to find. handler.go
// is the foundation file: interface, struct, constructor.
package avatar

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	svcavatar "github.com/avc-dev/go-avatar-service/internal/services/avatar"
)

// errUUIDNotV7 is returned by parseUserIDPath when the path parameter parsed
// successfully but the UUID is not version 7. It is package-private — the
// caller surfaces it as a 400 with a clear message rather than the bare error.
var errUUIDNotV7 = errors.New("must be UUID v7")

// parseUserIDPath parses a UUID v7 path parameter. The strictness mirrors the
// UserID middleware's policy so any uuid the system stores or accepts has the
// same shape regardless of where it entered the request.
func parseUserIDPath(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, err
	}
	if id.Version() != 7 {
		return uuid.Nil, errUUIDNotV7
	}
	return id, nil
}

// AvatarService is the consumer-side contract the handler depends on. Only
// the methods called by handlers appear here so the generated mock is precise
// and unused service surface area does not leak into HTTP tests.
type AvatarService interface {
	Upload(ctx context.Context, p svcavatar.UploadParams) (*domain.Avatar, error)
	GetMetadata(ctx context.Context, id uuid.UUID) (*domain.Avatar, error)
	DownloadOriginal(ctx context.Context, id uuid.UUID) (*svcavatar.DownloadResult, error)
	DownloadThumbnail(ctx context.Context, id uuid.UUID, size string) (*svcavatar.DownloadResult, error)
	GetCurrentByUserID(ctx context.Context, userID uuid.UUID) (*domain.Avatar, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Avatar, error)
	Delete(ctx context.Context, id, requestingUserID uuid.UUID) error
	DeleteCurrentByUserID(ctx context.Context, userID uuid.UUID) error
}

// Handler serves the /api/v1/avatars and /api/v1/users/{user_id}/avatar(s)
// endpoints. Construct with NewHandler; mount per-method handlers on a chi
// router in cmd/server.
type Handler struct {
	svc            AvatarService
	log            *slog.Logger
	maxUploadBytes int64
}

// NewHandler builds a Handler. maxUploadBytes caps the multipart body size
// for the upload endpoint via http.MaxBytesReader; it is also echoed back to
// the client in the 413 response so the UI can show a meaningful limit.
func NewHandler(svc AvatarService, log *slog.Logger, maxUploadBytes int64) *Handler {
	return &Handler{
		svc:            svc,
		log:            log,
		maxUploadBytes: maxUploadBytes,
	}
}
