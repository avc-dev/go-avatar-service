package imageproc_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/imageproc"
)

// tinyWebPBase64 is a 75x100 lossless WebP. It was lifted from the
// golang.org/x/image module's own test corpus
// (testdata/gopher-doc.1bpp.lossless.webp, 442 bytes) and base64-encoded so
// the test file stays free of binary blobs. Generating WebP from pure Go
// without CGO is impractical, hence the embedded fixture.
const tinyWebPBase64 = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="

// makeRGBA builds a w×h RGBA image with a non-uniform diagonal gradient so
// the encode/decode round-trip carries real information (a flat fill would
// compress to almost nothing and hide bugs in resizing logic).
func makeRGBA(t *testing.T, w, h int) image.Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := uint8((x * 255) / max(w-1, 1))
			g := uint8((y * 255) / max(h-1, 1))
			b := uint8(((x + y) * 255) / max(w+h-2, 1))
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, img image.Image, quality int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}))
	return buf.Bytes()
}

// decodeJPEG asserts the output bytes are a valid JPEG and returns its
// reported format string and bounds.
func decodeJPEG(t *testing.T, b []byte) (string, image.Rectangle) {
	t.Helper()
	img, format, err := image.Decode(bytes.NewReader(b))
	require.NoError(t, err)
	return format, img.Bounds()
}

func TestResizeToJPEG_PNGInput(t *testing.T) {
	r := imageproc.New()
	src := encodePNG(t, makeRGBA(t, 256, 128))

	out, err := r.ResizeToJPEG(t.Context(), bytes.NewReader(src), 100, 100, 85)
	require.NoError(t, err)
	require.NotEmpty(t, out)

	format, bounds := decodeJPEG(t, out)
	require.Equal(t, "jpeg", format)
	require.Equal(t, 100, bounds.Dx())
	require.Equal(t, 100, bounds.Dy())
}

func TestResizeToJPEG_JPEGUpscale(t *testing.T) {
	r := imageproc.New()
	src := encodeJPEG(t, makeRGBA(t, 200, 200), 90)

	out, err := r.ResizeToJPEG(t.Context(), bytes.NewReader(src), 300, 300, 85)
	require.NoError(t, err)
	require.NotEmpty(t, out)

	format, bounds := decodeJPEG(t, out)
	require.Equal(t, "jpeg", format)
	require.Equal(t, 300, bounds.Dx())
	require.Equal(t, 300, bounds.Dy())
}

func TestResizeToJPEG_WebPInput(t *testing.T) {
	r := imageproc.New()
	src, err := base64.StdEncoding.DecodeString(tinyWebPBase64)
	require.NoError(t, err)

	out, err := r.ResizeToJPEG(t.Context(), bytes.NewReader(src), 100, 100, 85)
	require.NoError(t, err)
	require.NotEmpty(t, out)

	format, bounds := decodeJPEG(t, out)
	require.Equal(t, "jpeg", format)
	require.Equal(t, 100, bounds.Dx())
	require.Equal(t, 100, bounds.Dy())
}

func TestResizeToJPEG_SameSizeRoundTrip(t *testing.T) {
	r := imageproc.New()
	src := encodePNG(t, makeRGBA(t, 100, 100))

	out, err := r.ResizeToJPEG(t.Context(), bytes.NewReader(src), 100, 100, 85)
	require.NoError(t, err)

	format, bounds := decodeJPEG(t, out)
	require.Equal(t, "jpeg", format)
	require.Equal(t, 100, bounds.Dx())
	require.Equal(t, 100, bounds.Dy())
}

func TestResizeToJPEG_FillCropAspectRatios(t *testing.T) {
	r := imageproc.New()

	cases := []struct {
		name    string
		srcW    int
		srcH    int
		dstW    int
		dstH    int
		expectW int
		expectH int
	}{
		{name: "wide source to square", srcW: 400, srcH: 100, dstW: 100, dstH: 100, expectW: 100, expectH: 100},
		{name: "tall source to square", srcW: 100, srcH: 400, dstW: 100, dstH: 100, expectW: 100, expectH: 100},
		{name: "square source to square", srcW: 200, srcH: 200, dstW: 100, dstH: 100, expectW: 100, expectH: 100},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := encodePNG(t, makeRGBA(t, tc.srcW, tc.srcH))
			out, err := r.ResizeToJPEG(t.Context(), bytes.NewReader(src), tc.dstW, tc.dstH, 85)
			require.NoError(t, err)

			format, bounds := decodeJPEG(t, out)
			require.Equal(t, "jpeg", format)
			require.Equal(t, tc.expectW, bounds.Dx())
			require.Equal(t, tc.expectH, bounds.Dy())
		})
	}
}

func TestResizeToJPEG_InvalidInputBytes(t *testing.T) {
	r := imageproc.New()
	junk := []byte("this is not an image, just some random bytes that should fail decode")

	_, err := r.ResizeToJPEG(t.Context(), bytes.NewReader(junk), 100, 100, 85)
	require.Error(t, err)
	msg := err.Error()
	require.True(t,
		strings.Contains(msg, "decode") || strings.Contains(msg, "imageproc"),
		"expected error to mention decode or imageproc, got: %s", msg,
	)
}

func TestResizeToJPEG_ValidationErrors(t *testing.T) {
	r := imageproc.New()
	// Use a valid source so we know failures come from validation, not decode.
	src := encodePNG(t, makeRGBA(t, 50, 50))

	cases := []struct {
		name     string
		width    int
		height   int
		quality  int
		mustHave string
	}{
		{name: "zero width", width: 0, height: 100, quality: 85, mustHave: "width"},
		{name: "negative height", width: 100, height: -1, quality: 85, mustHave: "height"},
		{name: "zero quality", width: 100, height: 100, quality: 0, mustHave: "quality"},
		{name: "quality above 100", width: 100, height: 100, quality: 101, mustHave: "quality"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.ResizeToJPEG(t.Context(), bytes.NewReader(src), tc.width, tc.height, tc.quality)
			require.Error(t, err)
			msg := strings.ToLower(err.Error())
			require.True(t,
				strings.Contains(msg, tc.mustHave) || strings.Contains(msg, "invalid"),
				"expected error to mention %q or 'invalid', got: %s", tc.mustHave, err.Error(),
			)
		})
	}
}

func TestResizeToJPEG_CancelledContext(t *testing.T) {
	r := imageproc.New()
	src := encodePNG(t, makeRGBA(t, 100, 100))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := r.ResizeToJPEG(ctx, bytes.NewReader(src), 100, 100, 85)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "expected errors.Is(err, context.Canceled), got: %v", err)
}
