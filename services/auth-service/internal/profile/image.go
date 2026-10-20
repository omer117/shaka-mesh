package profile

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"net/http"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register WebP decoder
)

const (
	MaxAvatarBytes = 2 << 20
	AvatarSize     = 256
	// maxSourcePixels guards against decompression bombs: a tiny file can
	// declare a gigantic canvas. 40 MP covers any phone camera.
	maxSourcePixels = 40_000_000
	jpegQuality     = 85
)

// SniffImageType returns the real MIME type from the file's magic bytes. The
// client-supplied Content-Type header is never consulted.
func SniffImageType(data []byte) string {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	switch t := http.DetectContentType(head); t {
	case "image/jpeg", "image/png", "image/webp":
		return t
	}
	return ""
}

// ProcessAvatar validates an upload and returns a 256x256 JPEG.
//
// Decoding to pixels and encoding again drops every metadata segment (EXIF,
// GPS, ICC, comments). The image is center-cropped to a square, scaled, and
// flattened onto white because JPEG has no alpha channel.
//
// Output is JPEG rather than WebP because the Go ecosystem has no pure-Go WebP
// encoder (x/image/webp only decodes); a cgo encoder would complicate the
// static Alpine build.
func ProcessAvatar(data []byte) ([]byte, error) {
	if len(data) > MaxAvatarBytes {
		return nil, ErrImageTooLarge
	}
	if SniffImageType(data) == "" {
		return nil, ErrUnsupportedImage
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, ErrInvalidImage
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxSourcePixels {
		return nil, ErrInvalidImage
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalidImage
	}

	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	minPt := image.Pt(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2)
	crop := image.Rectangle{Min: minPt, Max: minPt.Add(image.Pt(side, side))}

	dst := image.NewRGBA(image.Rect(0, 0, AvatarSize, AvatarSize))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, xdraw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
