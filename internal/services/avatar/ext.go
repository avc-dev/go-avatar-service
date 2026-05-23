package avatar

import (
	"fmt"

	"github.com/google/uuid"
)

// originalS3Key builds the object-storage key for an avatar's original file.
// The extension is derived from the MIME type so the MinIO web console shows
// usable filenames; unknown types fall through to ".bin".
func originalS3Key(id uuid.UUID, mimeType string) string {
	return fmt.Sprintf("originals/%s%s", id, extFromMIME(mimeType))
}

// extFromMIME maps known image MIME types to a canonical file extension.
// Handlers are responsible for rejecting unsupported MIME types before they
// reach the service, so unknown values here indicate a bug rather than a
// user input the service must accommodate.
func extFromMIME(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".bin"
	}
}
