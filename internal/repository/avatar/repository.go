package avatar

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// ErrNotFound is returned when an avatar does not exist or has already been
// soft-deleted. Use errors.Is to test for it.
var ErrNotFound = errors.New("avatar not found")

// ErrForbidden is returned by ownership-checking methods (e.g. DeleteByOwner)
// when the caller is not the owner of the avatar. Use errors.Is to test for it.
var ErrForbidden = errors.New("not avatar owner")

// Repository is the persistence port for avatar aggregates.
type Repository interface {
	Create(ctx context.Context, a *domain.Avatar) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Avatar, error)
	GetCurrentByUserID(ctx context.Context, userID uuid.UUID) (*domain.Avatar, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Avatar, error)
	ClaimForProcessing(ctx context.Context, id uuid.UUID) (*domain.Avatar, error)
	UpdateProcessingStatus(ctx context.Context, id uuid.UUID, status domain.ProcessingStatus, thumbs map[string]string) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	DeleteByOwner(ctx context.Context, id, requestingUserID uuid.UUID) (*domain.Avatar, error)
}

// PostgresRepository is a pgx-backed implementation of Repository.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository constructs a PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}
