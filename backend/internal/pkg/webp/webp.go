// Package webp — konversi gambar upload (JPEG/PNG/WebP) ke WebP, dipakai
// bersama oleh modul manapun yang perlu auto-convert gambar (§14 CMS artikel,
// sitemedia landing page). Ditaruh di internal/pkg supaya dua modul bisa
// memakainya tanpa saling import internal package satu sama lain (§22).
package webp

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

// ErrDecodeFailed — sentinel yang di-wrap Encode kalau source bukan gambar
// valid / corrupt. Caller di-service map ke error domain masing-masing
// (mis. cmsapi.ErrImageDecodeFailed, sitemediaapi.ErrImageDecodeFailed) lewat
// errors.Is.
var ErrDecodeFailed = errors.New("webp: image decode failed")

// allowedMimes — mime yang diterima upload. Selesai konversi selalu jadi
// image/webp di storage.
var allowedMimes = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/webp": true,
}

// IsAllowedMime — case-insensitive helper untuk validasi mime sebelum decode.
func IsAllowedMime(mime string) bool {
	return allowedMimes[strings.ToLower(strings.TrimSpace(mime))]
}

// Encoded — hasil konversi WebP siap disimpan.
type Encoded struct {
	Data   []byte
	Width  int
	Height int
}

// Encode membaca semua bytes dari r, decode via image.Decode (yang sudah
// tahu format via _-import), lalu encode ke WebP.
//
// Kalau decode gagal (file corrupt / bukan gambar) return ErrDecodeFailed.
// Non-domain error (mis. io.Copy) di-wrap dgn context.
func Encode(r io.Reader) (*Encoded, error) {
	// Baca ke buffer supaya image.Decode bisa peek magic bytes; ukuran sudah
	// dibatasi caller (max upload), jadi aman muat di memory.
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("webp: read image bytes: %w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.Join(err, ErrDecodeFailed)
	}
	var buf bytes.Buffer
	// nativewebp.Encode dgn opts nil = default (extended format, lossless).
	if err := nativewebp.Encode(&buf, img, nil); err != nil {
		return nil, fmt.Errorf("webp: encode: %w", err)
	}
	b := img.Bounds()
	return &Encoded{
		Data:   buf.Bytes(),
		Width:  b.Dx(),
		Height: b.Dy(),
	}, nil
}
