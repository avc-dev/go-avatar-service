package domain

import (
	"time"

	"github.com/google/uuid"
)

// Avatar is the aggregate root for user avatar metadata.
// The actual binary content lives in S3-compatible storage; this struct
// only carries the metadata persisted in PostgreSQL.
type Avatar struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	FileName         string
	MIMEType         string
	SizeBytes        int64
	S3Key            string
	ThumbnailS3Keys  map[string]string // e.g. {"100x100": "...", "300x300": "..."}; nil until processed
	UploadStatus     UploadStatus
	ProcessingStatus ProcessingStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

// UploadStatus describes the state of the original-file upload to S3.
type UploadStatus string

const (
	UploadStatusUploading UploadStatus = "uploading"
	UploadStatusCompleted UploadStatus = "completed"
	UploadStatusFailed    UploadStatus = "failed"
)

// ProcessingStatus describes the state of asynchronous thumbnail generation.
type ProcessingStatus string

const (
	ProcessingStatusPending    ProcessingStatus = "pending"
	ProcessingStatusProcessing ProcessingStatus = "processing"
	ProcessingStatusCompleted  ProcessingStatus = "completed"
	ProcessingStatusFailed     ProcessingStatus = "failed"
)
