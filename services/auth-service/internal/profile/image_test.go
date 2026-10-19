package profile

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"
)

// A valid 1x1 lossy WebP (x/image/webp can decode but not encode).
const tinyWebPBase64 = "UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA"

func twoToneImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{255, 0, 0, 255}), image.Point{}, draw.Src)
	right := image.Rect(w/2, 0, w, h)
	draw.Draw(img, right, image.NewUniform(color.RGBA{0, 0, 255, 255}), image.Point{}, draw.Src)
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withEXIF splices an APP1 "Exif" segment holding a fake GPS string right
// after the JPEG SOI marker.
func withEXIF(t *testing.T, jpegBytes []byte, secret string) []byte {
	t.Helper()
	payload := append([]byte("Exif\x00\x00"), []byte(secret)...)
	n := len(payload) + 2
	seg := append([]byte{0xFF, 0xE1, byte(n >> 8), byte(n)}, payload...)
	out := append([]byte{}, jpegBytes[:2]...)
	out = append(out, seg...)
	return append(out, jpegBytes[2:]...)
}

func decodeJPEG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("output is not a valid JPEG: %v", err)
	}
	return img
}

func TestProcessAvatarOutputDimensions(t *testing.T) {
	webp, _ := base64.StdEncoding.DecodeString(tinyWebPBase64)
	tests := []struct {
		name  string
		input []byte
	}{
		{"landscape png", encodePNG(t, twoToneImage(400, 200))},
		{"portrait png", encodePNG(t, twoToneImage(100, 300))},
		{"small jpeg is upscaled", encodeJPEG(t, twoToneImage(64, 64))},
		{"webp", webp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ProcessAvatar(tt.input)
			if err != nil {
				t.Fatalf("ProcessAvatar: %v", err)
			}
			b := decodeJPEG(t, out).Bounds()
			if b.Dx() != AvatarSize || b.Dy() != AvatarSize {
				t.Fatalf("got %dx%d, want %dx%d", b.Dx(), b.Dy(), AvatarSize, AvatarSize)
			}
		})
	}
}

func TestProcessAvatarCentersCrop(t *testing.T) {
	// 400x200, red left half / blue right half: the centered 200x200 crop is
	// still half red, half blue.
	out, err := ProcessAvatar(encodePNG(t, twoToneImage(400, 200)))
	if err != nil {
		t.Fatal(err)
	}
	img := decodeJPEG(t, out)
	r, _, b, _ := img.At(10, 128).RGBA()
	if r < b {
		t.Fatalf("left edge should be red, got r=%d b=%d", r, b)
	}
	r, _, b, _ = img.At(245, 128).RGBA()
	if b < r {
		t.Fatalf("right edge should be blue, got r=%d b=%d", r, b)
	}
}

func TestProcessAvatarStripsEXIF(t *testing.T) {
	const secret = "GPS-LAT-32.0853-LON-34.7818"
	input := withEXIF(t, encodeJPEG(t, twoToneImage(300, 300)), secret)
	if !bytes.Contains(input, []byte(secret)) {
		t.Fatal("test setup: secret missing from input")
	}
	out, err := ProcessAvatar(input)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte(secret)) || bytes.Contains(out, []byte("Exif")) {
		t.Fatal("EXIF data survived re-encoding")
	}
}

func TestProcessAvatarFlattensTransparencyOntoWhite(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 50, 50)) // fully transparent
	out, err := ProcessAvatar(encodePNG(t, img))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := decodeJPEG(t, out).At(128, 128).RGBA()
	if r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
		t.Fatalf("expected white background, got %d %d %d", r>>8, g>>8, b>>8)
	}
}

func TestProcessAvatarValidation(t *testing.T) {
	valid := encodePNG(t, twoToneImage(20, 20))
	// PNG header declaring a 30000x30000 canvas (decompression bomb).
	bomb := append([]byte{}, valid...)
	copy(bomb[16:], []byte{0, 0, 0x75, 0x30, 0, 0, 0x75, 0x30})
	tests := []struct {
		name    string
		input   []byte
		wantErr error
	}{
		{"empty", nil, ErrUnsupportedImage},
		{"plain text", []byte("hello, not an image"), ErrUnsupportedImage},
		{"gif", []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), ErrUnsupportedImage},
		{"html pretending to be image", []byte("<html><script>alert(1)</script></html>"), ErrUnsupportedImage},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), ErrUnsupportedImage},
		{"png magic but truncated", valid[:30], ErrInvalidImage},
		{"oversized canvas", bomb, ErrInvalidImage},
		{"over 2 MB", append(append([]byte{}, valid...), make([]byte, MaxAvatarBytes)...), ErrImageTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ProcessAvatar(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSniffImageTypeIgnoresClaimedType(t *testing.T) {
	// A PNG is a PNG no matter what a client's header says; sniffing only
	// looks at bytes.
	if got := SniffImageType(encodePNG(t, twoToneImage(4, 4))); got != "image/png" {
		t.Fatalf("got %q", got)
	}
	if got := SniffImageType([]byte("MZ\x90\x00 executable")); got != "" {
		t.Fatalf("got %q", got)
	}
}
