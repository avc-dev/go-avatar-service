package avatar

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/gabriel-vasile/mimetype"
)

// sniffBufferSize is the size of the head buffer used for magic-byte detection.
// mimetype.Detect reads at most 3072 bytes; matching that here means we never
// over-buffer just for sniffing and we hand back exactly that prefix as part
// of the reconstructed reader.
const sniffBufferSize = 3072

// errUnsupportedMIME is the sentinel returned when the detected content type
// is not in allowedImageMIMEs. The caller maps it to HTTP 415.
var errUnsupportedMIME = errors.New("unsupported image type")

// allowedImageMIMEs is the whitelist of accepted upload MIME types. These
// match the formats the imageproc.Resizer can decode — keeping the two in
// sync prevents the case where upload accepts a file the worker can't process.
var allowedImageMIMEs = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/gif":  {},
	"image/webp": {},
}

// sniffImageMIME peeks at the first few KB of r to detect its real MIME type
// via magic bytes, validates it against allowedImageMIMEs, and returns a new
// reader that replays the consumed prefix followed by the rest of the stream.
//
// On success the detected MIME is returned alongside the wrapped reader; the
// caller should prefer this value over any client-supplied Content-Type so a
// renamed `.txt → .png` file is recorded with its true type.
//
// On detection failure (read error) the original reader is returned together
// with the error so the caller can decide whether to retry or abort.
// On whitelist failure errUnsupportedMIME is returned; the partially-read
// reader is not safe to reuse and is not returned.
func sniffImageMIME(r io.Reader) (detectedMIME string, body io.Reader, err error) {
	head := make([]byte, sniffBufferSize)
	n, readErr := io.ReadFull(r, head)
	// io.ReadFull returns ErrUnexpectedEOF when the stream is shorter than the
	// buffer — that's fine for small images (a 200-byte gif is valid). Anything
	// else is a real read error.
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		return "", r, fmt.Errorf("sniff: read head: %w", readErr)
	}
	head = head[:n]

	mtype := mimetype.Detect(head)
	if _, ok := allowedImageMIMEs[mtype.String()]; !ok {
		return mtype.String(), nil, fmt.Errorf("sniff: %w: detected %s", errUnsupportedMIME, mtype.String())
	}

	// Replay the consumed prefix, then continue with the rest of r.
	return mtype.String(), io.MultiReader(bytes.NewReader(head), r), nil
}
