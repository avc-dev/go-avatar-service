package avatar

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// UploadParams carries the inputs of an Upload call.
type UploadParams struct {
	UserID   uuid.UUID
	FileName string
	MIMEType string
	Size     int64
	Reader   io.Reader
}

// Upload orchestrates the upload saga across object storage, persistence, and
// the event broker.
//
// Partial-failure semantics:
//   - Storage put fails:   return the error; nothing else is attempted.
//   - DB create fails:     attempt to delete the orphan object from storage as
//     compensation; return the original error regardless of
//     whether compensation itself succeeded.
//   - Publish fails:       log a WARN and return success — the avatar record
//     exists and is fully usable; only thumbnail processing
//     is delayed until a reconciliation job or outbox
//     pattern is added in a future iteration.
func (s *Service) Upload(ctx context.Context, p UploadParams) (*domain.Avatar, error) {
	avatarID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate avatar id: %w", err)
	}

	s3Key := originalS3Key(avatarID, p.MIMEType)

	if err := s.storage.Upload(ctx, s3Key, p.Reader, p.Size, p.MIMEType); err != nil {
		return nil, fmt.Errorf("store original: %w", err)
	}

	a := &domain.Avatar{
		ID:               avatarID,
		UserID:           p.UserID,
		FileName:         p.FileName,
		MIMEType:         p.MIMEType,
		SizeBytes:        p.Size,
		S3Key:            s3Key,
		UploadStatus:     domain.UploadStatusCompleted,
		ProcessingStatus: domain.ProcessingStatusPending,
	}

	if err := s.repo.Create(ctx, a); err != nil {
		// Compensation: try to delete the orphan object so it does not dangle
		// in storage. The compensation's own failure is logged but does not
		// mask the original error reported to the caller.
		if delErr := s.storage.Delete(ctx, s3Key); delErr != nil {
			s.log.Error("upload compensation failed: orphan in object storage",
				"s3_key", s3Key,
				"delete_err", delErr,
			)
		}
		return nil, fmt.Errorf("create avatar record: %w", err)
	}

	if err := s.publisher.PublishAvatarUploaded(ctx, domain.AvatarUploadEvent{
		AvatarID: avatarID,
		UserID:   p.UserID,
		S3Key:    s3Key,
	}); err != nil {
		s.log.Warn("avatar created but upload event was not published",
			"avatar_id", avatarID,
			"err", err,
		)
	}

	return a, nil
}
