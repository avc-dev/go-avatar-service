// Package imageproc provides pure-compute image processing primitives used by
// the avatar worker. The package wraps github.com/disintegration/imaging and
// exposes a single Resizer type so the implementation can be swapped later
// (e.g. for a libvips-backed variant) without touching call sites.
//
// The package is intentionally free of I/O, logging and broker concerns: it
// takes bytes in via io.Reader, returns bytes out via []byte, and lets the
// caller decide what to do with them. context.Context is accepted on the
// public method so call sites stay uniform with the rest of the project and
// so cancelled contexts can short-circuit before CPU-bound work begins.
package imageproc

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"

	// Register stdlib decoders we accept on input.
	_ "image/jpeg"
	_ "image/png"

	// Register the WebP decoder; we never encode WebP, only consume it.
	_ "golang.org/x/image/webp"

	"github.com/disintegration/imaging"
)

// Resizer performs decode → resize+crop → JPEG-encode pipelines. The struct
// is currently empty; it exists so future variants (configurable resampler,
// memory pools, libvips backend) can be added without breaking callers.
type Resizer struct{}

// New constructs a Resizer.
func New() *Resizer {
	return &Resizer{}
}

// ResizeToJPEG decodes src (any registered format: JPEG, PNG, WebP), resizes
// and centre-crops the result to exactly width×height using Lanczos
// resampling, then encodes as JPEG with the given quality (1..100). The
// returned slice is the encoded JPEG body.
//
// Validation:
//   - width and height must both be > 0.
//   - quality must be in [1, 100].
//   - If ctx is already cancelled at entry, ctx.Err() is returned (wrapped).
//
// The underlying imaging library is CPU-bound and does not consult ctx
// during work; ctx is only checked at entry so callers can cheaply abort
// already-cancelled jobs.
func (r *Resizer) ResizeToJPEG(ctx context.Context, src io.Reader, width, height, quality int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("imageproc: context cancelled before resize width=%dx%d: %w", width, height, err)
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("imageproc: invalid dimensions width=%d height=%d: must be > 0", width, height)
	}
	if quality < 1 || quality > 100 {
		return nil, fmt.Errorf("imageproc: invalid quality=%d: must be in [1,100]", quality)
	}

	img, _, err := image.Decode(src)
	if err != nil {
		return nil, fmt.Errorf("imageproc: decode width=%dx%d: %w", width, height, err)
	}

	resized := imaging.Fill(img, width, height, imaging.Center, imaging.Lanczos)

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, resized, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
		return nil, fmt.Errorf("imageproc: encode jpeg width=%dx%d: %w", width, height, err)
	}
	return buf.Bytes(), nil
}
