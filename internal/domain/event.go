package domain

import "github.com/google/uuid"

// AvatarUploadEvent is emitted after an avatar's original binary has been
// successfully stored in object storage. Workers consume it to generate
// thumbnails and perform any other post-upload processing.
type AvatarUploadEvent struct {
	AvatarID uuid.UUID
	UserID   uuid.UUID
	S3Key    string
}

// AvatarDeleteEvent is emitted when an avatar is removed. S3Keys carries
// every object the worker should evict from object storage — typically the
// original plus any previously-generated thumbnails.
type AvatarDeleteEvent struct {
	AvatarID uuid.UUID
	S3Keys   []string
}
