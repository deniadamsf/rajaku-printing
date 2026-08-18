// Package service — business logic modul sitemedia: kelola gambar landing
// page yang bisa diganti admin tanpa deploy ulang (lihat model.Registry).
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/rajaku-printing/backend/internal/pkg/webp"
	"github.com/rajaku-printing/backend/internal/sitemedia/model"
	sitemediarepo "github.com/rajaku-printing/backend/internal/sitemedia/repository"
	"github.com/rajaku-printing/backend/internal/sitemedia/sitemediaapi"
)

// MaxUploadBytes — batas ukuran file mentah (sebelum dikonversi ke WebP)
// untuk satu unggahan sitemedia (§23 poin 3 — validasi input di edge,
// konstanta bernama bukan angka telanjang). Gambar landing page bukan
// dokumen desain besar (CDR/AI) seperti modul design — 5 MB generous untuk
// JPEG/PNG resolusi tinggi.
const MaxUploadBytes = 5 * 1024 * 1024

// FileStore — narrowed filestore contract (sama shape dgn modul lain, mis. cms).
type FileStore interface {
	Save(ctx context.Context, subpath string, r io.Reader) (int64, error)
	Delete(ctx context.Context, subpath string) error
	AbsPath(subpath string) (string, error)
}

// Store — narrowed repository contract untuk testability.
type Store interface {
	Get(ctx context.Context, slot string) (*model.SiteMedia, error)
	List(ctx context.Context) ([]model.SiteMedia, error)
	Upsert(ctx context.Context, row *model.SiteMedia) error
	Delete(ctx context.Context, slot string) error
}

// Config — parameter service.
type Config struct {
	// BaseURL — dari config.App.BaseURL (§2 "satu sumber base URL"). Dipakai
	// membangun URL file publik (GET /api/v1/site-media/file/:slot). WAJIB
	// diisi caller — service tidak fallback ke localhost/hardcode apa pun.
	BaseURL string
}

type Service struct {
	store   Store
	blobs   FileStore
	baseURL string
	nowFn   func() time.Time
}

func New(store Store, blobs FileStore, cfg Config) *Service {
	return &Service{
		store:   store,
		blobs:   blobs,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		nowFn:   time.Now,
	}
}

// ListPublicMap — peta slot->url untuk slot yang SUDAH diisi admin. Dipanggil
// landing page saat SSR (public, no-auth) — harus ringan.
func (s *Service) ListPublicMap(ctx context.Context) (map[string]string, error) {
	rows, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("sitemedia: list public: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Slot] = s.fileURL(row.Slot)
	}
	return out, nil
}

// ListAdmin — SEMUA slot dari registry, digabung dengan nilai terisi. Slot
// kosong tetap muncul dengan field media bernilai nil.
func (s *Service) ListAdmin(ctx context.Context) ([]AdminSlotView, error) {
	rows, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("sitemedia: list admin: %w", err)
	}
	filled := make(map[string]model.SiteMedia, len(rows))
	for _, row := range rows {
		filled[row.Slot] = row
	}

	views := make([]AdminSlotView, 0, len(model.Registry))
	for _, def := range model.Registry {
		views = append(views, s.buildAdminView(def, filled))
	}
	return views, nil
}

func (s *Service) buildAdminView(def model.SlotDef, filled map[string]model.SiteMedia) AdminSlotView {
	view := AdminSlotView{
		Slot:              string(def.Key),
		Label:             def.Label,
		Description:       def.Description,
		SuggestedWidthPx:  def.SuggestedWidthPx,
		SuggestedHeightPx: def.SuggestedHeightPx,
	}
	row, ok := filled[string(def.Key)]
	if !ok {
		return view
	}
	url := s.fileURL(row.Slot)
	view.URL = &url
	view.OriginalName = &row.OriginalName
	view.MimeType = &row.MimeType
	view.SizeBytes = &row.SizeBytes
	view.WidthPx = &row.WidthPx
	view.HeightPx = &row.HeightPx
	view.UploadedBy = row.UploadedBy
	view.UploadedAt = &row.UploadedAt
	view.UpdatedAt = &row.UpdatedAt
	return view
}

// Upload mengganti isi satu slot: decode → convert WebP → simpan blob baru →
// perbarui DB → baru hapus blob lama (urutan wajib — kegagalan hapus lama
// tidak menggagalkan request, cukup log).
func (s *Service) Upload(ctx context.Context, in UploadInput) (*model.SiteMedia, error) {
	slot := strings.TrimSpace(in.Slot)
	if !model.IsValidSlot(slot) {
		return nil, sitemediaapi.ErrUnknownSlot
	}
	if in.FileSize <= 0 {
		return nil, sitemediaapi.ErrImageEmpty
	}
	if in.FileSize > MaxUploadBytes {
		return nil, sitemediaapi.ErrImageTooLarge
	}
	if !webp.IsAllowedMime(in.MimeType) {
		return nil, sitemediaapi.ErrImageInvalidType
	}

	// Batasi read agar tidak overshoot (defense terhadap MIME/size lie).
	limitedR := io.LimitReader(in.FileReader, MaxUploadBytes+1)
	encoded, err := webp.Encode(limitedR)
	if err != nil {
		if errors.Is(err, webp.ErrDecodeFailed) {
			return nil, sitemediaapi.ErrImageDecodeFailed
		}
		return nil, fmt.Errorf("sitemedia: encode webp slot %q: %w", slot, err)
	}

	now := s.nowFn().UTC()
	subpath := path.Join("site_media", slot, uuid.New().String()+".webp")
	written, err := s.blobs.Save(ctx, subpath, bytes.NewReader(encoded.Data))
	if err != nil {
		return nil, fmt.Errorf("sitemedia: save webp blob slot %q: %w", slot, err)
	}

	oldPath := s.lookupOldPath(ctx, slot)

	uploader := in.UploaderID
	row := &model.SiteMedia{
		Slot:         slot,
		StoragePath:  subpath,
		OriginalName: safeOriginalName(in.OriginalName),
		MimeType:     "image/webp",
		SizeBytes:    written,
		WidthPx:      encoded.Width,
		HeightPx:     encoded.Height,
		UploadedBy:   &uploader,
		UploadedAt:   now,
		UpdatedAt:    now,
	}
	if err := s.store.Upsert(ctx, row); err != nil {
		// DB gagal setelah blob baru tersimpan — bersihkan supaya tidak jadi
		// sampah orphan. Kegagalan cleanup di sini best-effort (log saja),
		// error utama tetap dari Upsert.
		if delErr := s.blobs.Delete(ctx, subpath); delErr != nil {
			log.Ctx(ctx).Warn().Err(delErr).Str("subpath", subpath).
				Msg("sitemedia: gagal bersihkan blob baru setelah upsert gagal")
		}
		return nil, fmt.Errorf("sitemedia: upsert slot %q: %w", slot, err)
	}

	// File lama dibersihkan TERAKHIR, setelah DB sukses — sesuai instruksi:
	// jangan hapus lama sebelum baru tersimpan, dan kegagalan hapus tidak
	// boleh menggagalkan request.
	if oldPath != "" && oldPath != subpath {
		if err := s.blobs.Delete(ctx, oldPath); err != nil {
			log.Ctx(ctx).Warn().Err(err).Str("slot", slot).Str("old_path", oldPath).
				Msg("sitemedia: gagal hapus blob lama setelah slot ditimpa — cleanup manual mungkin diperlukan")
		}
	}

	return row, nil
}

// lookupOldPath — cek baris lama SEBELUM upsert supaya tahu file mana yang
// perlu dibersihkan setelah DB diperbarui. Lookup gagal (non-not-found)
// di-log tapi tidak menghentikan alur — blob baru sudah aman tersimpan.
func (s *Service) lookupOldPath(ctx context.Context, slot string) string {
	existing, err := s.store.Get(ctx, slot)
	if err != nil {
		if !errors.Is(err, sitemediarepo.ErrNotFound) {
			log.Ctx(ctx).Warn().Err(err).Str("slot", slot).
				Msg("sitemedia: lookup baris lama gagal, lanjut upsert (file lama mungkin tidak terhapus)")
		}
		return ""
	}
	return existing.StoragePath
}

// Delete mengosongkan slot — frontend kembali memakai file statis bawaan.
func (s *Service) Delete(ctx context.Context, slot string) error {
	slot = strings.TrimSpace(slot)
	if !model.IsValidSlot(slot) {
		return sitemediaapi.ErrUnknownSlot
	}
	existing, err := s.store.Get(ctx, slot)
	if err != nil {
		if errors.Is(err, sitemediarepo.ErrNotFound) {
			return sitemediaapi.ErrSlotEmpty
		}
		return fmt.Errorf("sitemedia: lookup slot %q before delete: %w", slot, err)
	}
	if err := s.store.Delete(ctx, slot); err != nil {
		if errors.Is(err, sitemediarepo.ErrNotFound) {
			return sitemediaapi.ErrSlotEmpty
		}
		return fmt.Errorf("sitemedia: delete slot %q: %w", slot, err)
	}
	if err := s.blobs.Delete(ctx, existing.StoragePath); err != nil {
		log.Ctx(ctx).Warn().Err(err).Str("slot", slot).Str("path", existing.StoragePath).
			Msg("sitemedia: gagal hapus blob setelah slot dikosongkan")
	}
	return nil
}

// GetFile — resolve slot ke path fisik untuk disajikan handler.
func (s *Service) GetFile(ctx context.Context, slot string) (*FileHandle, error) {
	if !model.IsValidSlot(slot) {
		return nil, sitemediaapi.ErrUnknownSlot
	}
	row, err := s.store.Get(ctx, slot)
	if err != nil {
		if errors.Is(err, sitemediarepo.ErrNotFound) {
			return nil, sitemediaapi.ErrSlotEmpty
		}
		return nil, fmt.Errorf("sitemedia: get file slot %q: %w", slot, err)
	}
	abs, err := s.blobs.AbsPath(row.StoragePath)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Str("slot", slot).Str("subpath", row.StoragePath).
			Msg("sitemedia: path ditolak filestore — cek DB")
		return nil, fmt.Errorf("sitemedia: resolve path slot %q: %w", slot, err)
	}
	return &FileHandle{AbsPath: abs, MimeType: row.MimeType, OriginalName: row.OriginalName}, nil
}

func (s *Service) fileURL(slot string) string {
	return s.baseURL + "/api/v1/site-media/file/" + slot
}

// safeOriginalName — buang path separator supaya nama file yg disimpan aman
// untuk display (bukan untuk fs path — storage_path digenerate service).
func safeOriginalName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "file"
	}
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}
