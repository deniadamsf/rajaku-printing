// Package service — gambar produk katalog (§9 landing page card image).
//
// Pola sama dengan modul sitemedia (internal/sitemedia/service): terima
// JPEG/PNG/WebP, convert ke WebP, simpan lewat FileStore (§19 disk lokal
// VPS), URL publik dihitung dari BaseURL config terpusat (§2) SAAT BACA —
// DB hanya menyimpan path penyimpanan relatif (products.image_path), bukan
// URL absolut (§22, cegah data lama menunjuk host lama setelah APP_BASE_URL
// berubah). Path fisik per unggahan memakai UUID acak (pola sama dgn
// sitemedia) — bukan deterministik dari product_id — supaya unggahan baru
// ditulis ke file BARU dulu, baris DB diperbarui, baru file lama dihapus;
// kalau update DB gagal, gambar publik yang sudah tersaji tidak pernah
// berubah dari sudut pandang pembaca.
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/catalog/catalogapi"
	"github.com/rajaku-printing/backend/internal/catalog/model"
	"github.com/rajaku-printing/backend/internal/catalog/repository"
	"github.com/rajaku-printing/backend/internal/pkg/webp"
)

// MaxProductImageUploadBytes — batas ukuran file mentah (sebelum konversi
// WebP) untuk satu unggahan gambar produk. 4 MB per spesifikasi tugas.
const MaxProductImageUploadBytes = 4 * 1024 * 1024

// Sentinel errors — dibedakan penanganannya di handler (§22, errors.Is bukan
// compare string error).
var (
	ErrImageEmpty        = errors.New("catalog admin: file gambar kosong")
	ErrImageTooLarge     = errors.New("catalog admin: file gambar melebihi batas ukuran 4 MB")
	ErrImageInvalidType  = errors.New("catalog admin: tipe file tidak didukung (hanya jpeg/png/webp)")
	ErrImageDecodeFailed = errors.New("catalog admin: file gambar tidak valid/corrupt")
	ErrProductImageEmpty = errors.New("catalog admin: produk belum punya gambar")
)

// ProductImageInput — parameter unggah gambar satu produk.
type ProductImageInput struct {
	FileReader io.Reader
	FileSize   int64
	MimeType   string
}

// ProductImageHandle — hasil resolve gambar produk ke path fisik, dipakai
// handler publik untuk menyajikan file (c.File / header Content-Type).
type ProductImageHandle struct {
	AbsPath  string
	MimeType string
}

// AdminUploadProductImage mengganti gambar satu produk: decode → convert
// WebP → simpan blob BARU (UUID acak) → perbarui kolom image_path → baru
// hapus blob lama (urutan wajib, sama seperti sitemedia.Upload — kalau
// update DB gagal, blob baru yang terlanjur ditulis dibersihkan dan gambar
// lama tetap utuh/tersaji).
func (s *Service) AdminUploadProductImage(ctx context.Context, productID uuid.UUID, in ProductImageInput) (*model.Product, error) {
	if err := validateProductImageInput(in); err != nil {
		return nil, err
	}
	product, err := s.products.FindByIDWithPricings(ctx, productID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrProductNotFound
		}
		return nil, fmt.Errorf("catalog: load product %s for image upload: %w", productID, err)
	}

	// Batasi read agar tidak overshoot (defense terhadap MIME/size lie).
	limitedR := io.LimitReader(in.FileReader, MaxProductImageUploadBytes+1)
	encoded, err := webp.Encode(limitedR)
	if err != nil {
		if errors.Is(err, webp.ErrDecodeFailed) {
			return nil, ErrImageDecodeFailed
		}
		return nil, fmt.Errorf("catalog: encode webp for product %s: %w", productID, err)
	}

	newSubpath := newProductImageSubpath(productID)
	if _, err := s.blobs.Save(ctx, newSubpath, bytes.NewReader(encoded.Data)); err != nil {
		return nil, fmt.Errorf("catalog: save product image blob %s: %w", productID, err)
	}

	oldPath := product.ImagePath
	if err := s.products.Update(ctx, productID, map[string]any{"image_path": newSubpath}); err != nil {
		// DB gagal setelah blob baru tersimpan — bersihkan supaya tidak jadi
		// sampah orphan; gambar lama (kalau ada) tetap tersaji apa adanya.
		if delErr := s.blobs.Delete(ctx, newSubpath); delErr != nil {
			log.Ctx(ctx).Warn().Err(delErr).Str("product_id", productID.String()).Str("subpath", newSubpath).
				Msg("catalog: gagal bersihkan blob baru setelah update image_path gagal")
		}
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrProductNotFound
		}
		return nil, fmt.Errorf("catalog: set image_path for product %s: %w", productID, err)
	}

	// File lama dibersihkan TERAKHIR, setelah DB sukses — kegagalan hapus
	// tidak menggagalkan request, cukup log (sama pola dgn sitemedia.Upload).
	if oldPath != nil && *oldPath != "" && *oldPath != newSubpath {
		if err := s.blobs.Delete(ctx, *oldPath); err != nil {
			log.Ctx(ctx).Warn().Err(err).Str("product_id", productID.String()).Str("old_path", *oldPath).
				Msg("catalog: gagal hapus blob gambar produk lama setelah ditimpa — cleanup manual mungkin diperlukan")
		}
	}

	product.ImagePath = &newSubpath
	return product, nil
}

// AdminDeleteProductImage mengosongkan gambar produk — kolom image_path
// diset NULL lebih dulu, baru file fisik dihapus best-effort (kegagalan
// hapus blob tidak menggagalkan request, cukup di-log, sama dgn pola
// sitemedia.Delete).
func (s *Service) AdminDeleteProductImage(ctx context.Context, productID uuid.UUID) error {
	product, err := s.products.FindByID(ctx, productID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return catalogapi.ErrProductNotFound
		}
		return fmt.Errorf("catalog: load product %s for image delete: %w", productID, err)
	}
	if product.ImagePath == nil {
		return ErrProductImageEmpty
	}
	oldPath := *product.ImagePath
	if err := s.products.Update(ctx, productID, map[string]any{"image_path": nil}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return catalogapi.ErrProductNotFound
		}
		return fmt.Errorf("catalog: clear image_path for product %s: %w", productID, err)
	}
	if err := s.blobs.Delete(ctx, oldPath); err != nil {
		log.Ctx(ctx).Warn().Err(err).Str("product_id", productID.String()).
			Msg("catalog: gagal hapus blob gambar produk setelah image_path dikosongkan")
	}
	return nil
}

// GetProductImage — resolve gambar produk ke path fisik untuk disajikan
// handler publik (GET /catalog/product-images/:id).
func (s *Service) GetProductImage(ctx context.Context, productID uuid.UUID) (*ProductImageHandle, error) {
	product, err := s.products.FindByID(ctx, productID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, catalogapi.ErrProductNotFound
		}
		return nil, fmt.Errorf("catalog: load product %s for image serve: %w", productID, err)
	}
	if product.ImagePath == nil {
		return nil, ErrProductImageEmpty
	}
	abs, err := s.blobs.AbsPath(*product.ImagePath)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Str("product_id", productID.String()).
			Msg("catalog: path gambar produk ditolak filestore — cek konsistensi data")
		return nil, fmt.Errorf("catalog: resolve product image path %s: %w", productID, err)
	}
	return &ProductImageHandle{AbsPath: abs, MimeType: "image/webp"}, nil
}

func validateProductImageInput(in ProductImageInput) error {
	if in.FileSize <= 0 {
		return ErrImageEmpty
	}
	if in.FileSize > MaxProductImageUploadBytes {
		return ErrImageTooLarge
	}
	if !webp.IsAllowedMime(in.MimeType) {
		return ErrImageInvalidType
	}
	return nil
}

// newProductImageSubpath — path fisik BARU per unggahan, UUID acak (pola
// sama dgn internal/sitemedia) — bukan deterministik dari product_id, supaya
// unggahan baru tidak pernah menimpa file yang sedang disajikan sebelum
// baris DB berhasil diperbarui (lihat AdminUploadProductImage).
func newProductImageSubpath(productID uuid.UUID) string {
	return path.Join("product_images", productID.String(), uuid.New().String()+".webp")
}

// ProductImageURL — URL publik gambar produk, dihitung dari BaseURL config
// terpusat (§2) + product_id. DIPANGGIL SAAT SERIALISASI RESPONSE (handler
// dto.go), bukan disimpan di DB — supaya ganti APP_BASE_URL tidak perlu UPDATE
// massal ke baris lama.
func (s *Service) ProductImageURL(productID uuid.UUID) string {
	return s.baseURL + "/api/v1/catalog/product-images/" + productID.String()
}
