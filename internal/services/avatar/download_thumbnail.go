package avatar

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

// DownloadThumbnail fetches a previously-generated thumbnail for the given
// avatar by its size key (e.g. "100x100"). Returns ErrNotFound if either the
// avatar does not exist or that particular thumbnail has not been produced
// yet — from the caller's perspective both cases are "this resource is not
// available", and 404 is the right HTTP mapping.
//
// The DownloadResult carries the same metadata as DownloadOriginal so the HTTP
// layer can set Content-Type and Content-Length headers uniformly across
// original and thumbnail responses.
func (s *Service) DownloadThumbnail(ctx context.Context, id uuid.UUID, size string) (*DownloadResult, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repoavatar.ErrNotFound) {
			err = ErrNotFound
		}
		return nil, fmt.Errorf("download thumbnail %s for avatar %s: %w", size, id, err)
	}

	s3Key, ok := a.ThumbnailS3Keys[size]
	if !ok {
		// Either the size string is not one we generate, or the worker has
		// not produced this variant yet. Caller cannot tell the difference
		// — treat both as not-found so handlers map to 404.
		return nil, fmt.Errorf("download thumbnail %s for avatar %s: %w", size, id, ErrNotFound)
	}

	obj, err := s.storage.Download(ctx, s3Key)
	if err != nil {
		return nil, fmt.Errorf("download thumbnail %s for avatar %s: %w", size, id, err)
	}

	return &DownloadResult{
		Reader:      obj.Reader,
		ContentType: obj.ContentType,
		Size:        obj.Size,
		Avatar:      a,
	}, nil
}
