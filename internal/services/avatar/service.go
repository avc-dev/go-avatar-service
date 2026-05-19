// Package avatar contains the application-service layer for the avatar
// aggregate: orchestration of object storage, persistence, and event
// publishing into the user-facing operations.
//
// Dependency interfaces (AvatarRepository, ObjectStorage, EventPublisher) are
// declared here on the consumer side: the package owns the contract it needs,
// and concrete implementations from other packages satisfy it via Go's
// structural typing.
package avatar

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	"github.com/avc-dev/go-avatar-service/internal/storage"
)

// Sentinel errors returned by Service. Handlers map these to HTTP status
// codes; other layers should compare via errors.Is.
var (
	ErrNotFound  = errors.New("avatar not found")
	ErrForbidden = errors.New("forbidden")
)

// AvatarRepository is the persistence dependency. The service uses only these
// methods.
type AvatarRepository interface {
	Create(ctx context.Context, a *domain.Avatar) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Avatar, error)
	GetCurrentByUserID(ctx context.Context, userID uuid.UUID) (*domain.Avatar, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Avatar, error)
	DeleteByOwner(ctx context.Context, id, requestingUserID uuid.UUID) (*domain.Avatar, error)
}

// ObjectStorage is the blob-storage dependency.
type ObjectStorage interface {
	Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (*storage.DownloadResult, error)
	Delete(ctx context.Context, key string) error
}

// EventPublisher emits domain events to downstream consumers.
type EventPublisher interface {
	PublishAvatarUploaded(ctx context.Context, event domain.AvatarUploadEvent) error
	PublishAvatarDeleted(ctx context.Context, event domain.AvatarDeleteEvent) error
}

// Service orchestrates the avatar domain.
type Service struct {
	repo      AvatarRepository
	storage   ObjectStorage
	publisher EventPublisher
	log       *slog.Logger
}

// New constructs a Service. All dependencies are required.
func New(repo AvatarRepository, st ObjectStorage, pub EventPublisher, log *slog.Logger) *Service {
	return &Service{
		repo:      repo,
		storage:   st,
		publisher: pub,
		log:       log,
	}
}
