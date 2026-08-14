// image_processor — decode source image (JPG/PNG/WebP) lalu re-encode ke WebP
// via HugoSmits86/nativewebp (pure Go, no cgo). Dipanggil sekali per upload.
package service

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	// Register decoders for the formats we accept.
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"

	nativewebp "github.com/HugoSmits86/nativewebp"
	// WebP decoder (untuk input yang sudah WebP) — supaya image.Decode kenal
	// format WebP.
	_ "golang.org/x/image/webp"
)

// allowedImageMimes — mime yang diterima upload. Selesai konversi selalu
// jadi image/webp di storage.
var allowedImageMimes = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/webp": true,
}

// isAllowedImageMime — case-insensitive helper untuk validasi mime.
func isAllowedImageMime(mime string) bool {
	return allowedImageMimes[strings.ToLower(strings.TrimSpace(mime))]
}

// encodedImage — hasil konversi WebP siap disimpan.
type encodedImage struct {
	Data   []byte
	Width  int
	Height int
}

// encodeToWebP membaca semua bytes dari r, decode via image.Decode (yang
// sudah tahu format via _-import), lalu encode ke WebP.
//
// Kalau decode gagal (file corrupt / bukan gambar) return ErrImageDecodeFailed.
// Non-domain error (mis. io.Copy) di-wrap dgn context.
func encodeToWebP(r io.Reader) (*encodedImage, error) {
	// Baca ke buffer supaya image.Decode bisa peek magic bytes; ukuran sudah
	// dibatasi caller (MaxUploadMB), jadi aman muat di memory.
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read image bytes: %w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.Join(err, errImageDecode)
	}
	var buf bytes.Buffer
	// nativewebp.Encode dgn opts nil = default (extended format, lossless).
	if err := nativewebp.Encode(&buf, img, nil); err != nil {
		return nil, fmt.Errorf("webp encode: %w", err)
	}
	b := img.Bounds()
	return &encodedImage{
		Data:   buf.Bytes(),
		Width:  b.Dx(),
		Height: b.Dy(),
	}, nil
}

// errImageDecode — sentinel internal untuk di-wrap encodeToWebP; service
// layer akan errors.Is-check dan map ke cmsapi.ErrImageDecodeFailed.
var errImageDecode = errors.New("image decode failed")
