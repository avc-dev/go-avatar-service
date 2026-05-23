package avatar

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

// DownloadResult carries the binary body plus the metadata needed by the HTTP
// layer to set Content-Type, Cache-Control and ETag headers. The caller is
// responsible for closing Reader.
type DownloadResult struct {
	Reader      io.ReadCloser
	ContentType string
	Size        int64
	Avatar      *domain.Avatar
}

// DownloadOriginal fetches both the metadata and the original binary for the
// given avatar id. Missing or soft-deleted avatars produce ErrNotFound.
func (s *Service) DownloadOriginal(ctx context.Context, id uuid.UUID) (*DownloadResult, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repoavatar.ErrNotFound) {
			err = ErrNotFound
		}
		return nil, fmt.Errorf("download original for avatar %s: %w", id, err)
	}

	obj, err := s.storage.Download(ctx, a.S3Key)
	if err != nil {
		return nil, fmt.Errorf("download original for avatar %s: %w", id, err)
	}

	return &DownloadResult{
		Reader:      obj.Reader,
		ContentType: obj.ContentType,
		Size:        obj.Size,
		Avatar:      a,
	}, nil
}
