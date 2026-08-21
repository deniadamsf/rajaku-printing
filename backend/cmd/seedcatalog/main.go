// Command seedcatalog mengisi katalog dengan produk percetakan banner yang
// realistis untuk toko printing di Trenggalek, lengkap dengan pricing rule
// per bahan/ukuran. Meant to be run once, manually, setelah migration
// dijalankan.
//
// ============================================================================
// PERINGATAN — SEMUA ANGKA HARGA DI FILE INI ADALAH PLACEHOLDER.
//
// Pemilik toko BELUM memberikan daftar harga asli (lihat CLAUDE.md §25,
// checklist "Data operasional" belum dikonfirmasi saat dokumen ini ditulis).
// Angka price_per_m2 / price_total di bawah HANYA supaya katalog terisi dan
// flow order/quote bisa didemokan & dikembangkan — BUKAN harga jual
// sebenarnya. WAJIB dikoreksi lewat admin panel /admin/katalog sebelum
// go-live produksi. Sengaja TIDAK dimasukkan ke migration (yang jalan
// otomatis di produksi) — perintah ini harus dijalankan manual oleh operator
// yang sadar angkanya perlu dikoreksi.
// ============================================================================
//
// Idempoten: aman dijalankan berkali-kali — produk yang slug-nya sudah ada
// dilewati (tidak ditimpa), material yang code-nya sudah ada dipakai ulang
// (get-or-create). TIDAK menyentuh produk seed bawaan migration 000002
// (banner-flexi, x-banner).
//
// Usage:
//
//	go run ./cmd/seedcatalog
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rajaku-printing/backend/internal/catalog/model"
	"github.com/rajaku-printing/backend/internal/catalog/repository"
	"github.com/rajaku-printing/backend/internal/config"
	"github.com/rajaku-printing/backend/internal/database"
	"github.com/rajaku-printing/backend/internal/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger.Init(cfg.App.LogLevel, cfg.App.Env == config.EnvDevelopment)

	ctx := context.Background()
	db, err := database.Open(ctx, cfg.DB, cfg.App.Env == config.EnvDevelopment)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	materialRepo := repository.NewMaterialRepository(db)
	productRepo := repository.NewProductRepository(db)

	materialIDs, err := ensureMaterials(ctx, materialRepo, seedMaterials)
	if err != nil {
		return fmt.Errorf("seed materials: %w", err)
	}

	// Validasi SEMUA material code yang dibutuhkan produk di bawah SEBELUM
	// menulis apa pun ke products/product_pricings — kalau ada typo code,
	// run gagal cepat & bersih, bukan meninggalkan sebagian produk
	// setengah jadi (lihat catatan atomisitas di seedProduct).
	if err := validateReferencedMaterialCodes(seedProducts, materialIDs); err != nil {
		return fmt.Errorf("validasi material code: %w", err)
	}

	created := 0
	skipped := 0
	for _, def := range seedProducts {
		wasCreated, err := seedProduct(ctx, db, productRepo, materialIDs, def)
		if err != nil {
			return fmt.Errorf("seed produk %q: %w", def.Slug, err)
		}
		if wasCreated {
			created++
		} else {
			skipped++
		}
	}

	log.Warn().
		Int("products_created", created).
		Int("products_skipped_existing", skipped).
		Msg("seedcatalog selesai — SEMUA HARGA ADALAH PLACEHOLDER, wajib dikoreksi lewat admin panel /admin/katalog sebelum go-live")
	return nil
}

// -------------------------------------------------------------------------
//  Materials — get-or-create by code.
// -------------------------------------------------------------------------

type materialDef struct {
	Code        string
	Name        string
	Description string
}

// seedMaterials — mencakup material yang sudah di-seed migration 000002
// (flexi_280, flexi_340, vinyl_solvent, backlite — get-or-create jadi aman
// dipanggil ulang) PLUS material baru khusus produk di bawah.
var seedMaterials = []materialDef{
	{Code: "flexi_280", Name: "Flexi 280 gsm", Description: "Kain flexi outdoor 280 gsm — pilihan populer untuk banner harian."},
	{Code: "flexi_340", Name: "Flexi 340 gsm", Description: "Kain flexi outdoor 340 gsm — lebih tebal, lebih awet."},
	{Code: "flexi_440", Name: "Flexi 440 gsm", Description: "Bahan flexi outdoor paling tebal — untuk baliho/billboard ukuran besar yang kena angin & hujan langsung."},
	{Code: "vinyl_solvent", Name: "Vinyl Solvent", Description: "Vinyl adhesive untuk X-banner / roll-up / stiker outdoor."},
	{Code: "backlite", Name: "Backlite Film", Description: "Bahan tembus cahaya untuk lightbox / display back-lit."},
	{Code: "vinyl_cutting", Name: "Vinyl Cutting (Sticker)", Description: "Vinyl cutting satu warna untuk stiker huruf/logo potong (bukan print), ditempel manual."},
	{Code: "sticker_glossy", Name: "Sticker Vinyl Glossy", Description: "Stiker vinyl print permukaan mengkilap — untuk label produk, stiker kemasan, dan branding kaca."},
}

func ensureMaterials(ctx context.Context, repo *repository.MaterialRepository, defs []materialDef) (map[string]uuid.UUID, error) {
	ids := make(map[string]uuid.UUID, len(defs))
	for _, def := range defs {
		id, err := ensureMaterial(ctx, repo, def)
		if err != nil {
			return nil, err
		}
		ids[def.Code] = id
	}
	return ids, nil
}

func ensureMaterial(ctx context.Context, repo *repository.MaterialRepository, def materialDef) (uuid.UUID, error) {
	existing, err := repo.FindByCode(ctx, def.Code)
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return uuid.Nil, fmt.Errorf("cek material %q: %w", def.Code, err)
	}

	m := &model.Material{Code: def.Code, Name: def.Name, IsActive: true}
	if def.Description != "" {
		description := def.Description
		m.Description = &description
	}
	if err := repo.Create(ctx, m); err != nil {
		return uuid.Nil, fmt.Errorf("buat material %q: %w", def.Code, err)
	}
	log.Info().Str("code", def.Code).Msg("material dibuat")
	return m.ID, nil
}

// -------------------------------------------------------------------------
//  Products — skip kalau slug sudah ada (idempoten, tidak menimpa).
// -------------------------------------------------------------------------

type perM2PricingDef struct {
	MaterialCode string
	PricePerM2   int64
	MinChargeM2  float64 // 0 = tidak ada minimum charge
}

type paketPricingDef struct {
	MaterialCode string
	WidthCm      int
	HeightCm     int
	PackageLabel string
	PriceTotal   int64
}

type productDef struct {
	Slug         string
	Name         string
	Description  string
	Category     string
	PricingType  model.PricingType
	MinWidthCm   *int
	MinHeightCm  *int
	MaxWidthCm   *int
	MaxHeightCm  *int
	DisplayOrder int

	PerM2 []perM2PricingDef // dipakai kalau PricingType == per_m2
	Paket []paketPricingDef // dipakai kalau PricingType == paket
}

func intPtr(v int) *int { return &v }

// seedProducts — 6 produk baru. JANGAN tambahkan banner-flexi / x-banner di
// sini — keduanya sudah di-seed migration 000002 (fixed, produksi).
//
// Catatan: produk "spanduk flexi" SENGAJA tidak ada di daftar ini. Itu adalah
// barang yang sama dengan `banner-flexi` bawaan migration 000002 (sama-sama
// kategori `banner`, sama-sama flexi custom size) — menambahkannya cuma
// menghasilkan dua kartu produk kembar di katalog publik.
//
// SEMUA price_per_m2 / price_total DI BAWAH ADALAH PLACEHOLDER — lihat
// peringatan di kepala file.
var seedProducts = []productDef{
	{
		Slug:         "baliho",
		Name:         "Baliho / Billboard",
		Description:  "Baliho ukuran besar untuk promosi jalan raya, bahan tebal tahan angin & hujan.",
		Category:     "baliho",
		PricingType:  model.PricingTypePerM2,
		MinWidthCm:   intPtr(100),
		MinHeightCm:  intPtr(100),
		MaxWidthCm:   intPtr(500),
		MaxHeightCm:  intPtr(2000),
		DisplayOrder: 40,
		PerM2: []perM2PricingDef{
			{MaterialCode: "flexi_340", PricePerM2: 30_000, MinChargeM2: 2.0},
			{MaterialCode: "flexi_440", PricePerM2: 38_000, MinChargeM2: 2.0},
		},
	},
	{
		Slug:         "roll-up-banner",
		Name:         "Roll-Up Banner",
		Description:  "Banner berdiri portable (sudah termasuk standnya) — cocok untuk pameran/booth/resepsionis.",
		Category:     "roll-up",
		PricingType:  model.PricingTypePaket,
		DisplayOrder: 50,
		Paket: []paketPricingDef{
			{MaterialCode: "vinyl_solvent", WidthCm: 60, HeightCm: 160, PackageLabel: "Standar 60x160", PriceTotal: 150_000},
			{MaterialCode: "vinyl_solvent", WidthCm: 85, HeightCm: 200, PackageLabel: "Jumbo 85x200", PriceTotal: 220_000},
		},
	},
	{
		Slug:         "backdrop-panggung",
		Name:         "Backdrop Panggung & Photo Booth",
		Description:  "Backdrop ukuran besar untuk panggung event, pernikahan, atau photo booth.",
		Category:     "backdrop",
		PricingType:  model.PricingTypePerM2,
		MinWidthCm:   intPtr(100),
		MinHeightCm:  intPtr(100),
		MaxWidthCm:   intPtr(600),
		MaxHeightCm:  intPtr(400),
		DisplayOrder: 60,
		PerM2: []perM2PricingDef{
			{MaterialCode: "flexi_280", PricePerM2: 22_000, MinChargeM2: 2.0},
			{MaterialCode: "flexi_340", PricePerM2: 29_000, MinChargeM2: 2.0},
		},
	},
	{
		Slug:         "stiker-vinyl",
		Name:         "Stiker Vinyl / Cutting",
		Description:  "Stiker cutting satu warna (nama toko, logo, huruf timbul) atau stiker print vinyl outdoor.",
		Category:     "stiker",
		PricingType:  model.PricingTypePerM2,
		MinWidthCm:   intPtr(10),
		MinHeightCm:  intPtr(10),
		MaxWidthCm:   intPtr(122),
		MaxHeightCm:  intPtr(1000),
		DisplayOrder: 70,
		PerM2: []perM2PricingDef{
			{MaterialCode: "vinyl_cutting", PricePerM2: 15_000, MinChargeM2: 0.25},
			{MaterialCode: "vinyl_solvent", PricePerM2: 40_000, MinChargeM2: 0.25},
			{MaterialCode: "sticker_glossy", PricePerM2: 45_000, MinChargeM2: 0.25},
		},
	},
	{
		Slug:         "banner-backlite",
		Name:         "Banner Backlite (Lightbox)",
		Description:  "Bahan tembus cahaya untuk display lightbox/neon box — cocok untuk signage toko malam hari.",
		Category:     "backlite",
		PricingType:  model.PricingTypePerM2,
		MinWidthCm:   intPtr(30),
		MinHeightCm:  intPtr(30),
		MaxWidthCm:   intPtr(200),
		MaxHeightCm:  intPtr(300),
		DisplayOrder: 80,
		PerM2: []perM2PricingDef{
			{MaterialCode: "backlite", PricePerM2: 55_000, MinChargeM2: 1.0},
		},
	},
}

// validateReferencedMaterialCodes memastikan SETIAP material code yang
// dipakai produk manapun di seedProducts sudah punya entri di materialIDs
// (hasil ensureMaterials). Dipanggil SEBELUM produk apa pun ditulis ke DB —
// kalau ada typo code, run gagal cepat & bersih tanpa meninggalkan produk
// setengah jadi (lihat catatan atomisitas di seedProduct).
func validateReferencedMaterialCodes(defs []productDef, materialIDs map[string]uuid.UUID) error {
	for _, def := range defs {
		codes := make([]string, 0, len(def.PerM2)+len(def.Paket))
		for _, pr := range def.PerM2 {
			codes = append(codes, pr.MaterialCode)
		}
		for _, pr := range def.Paket {
			codes = append(codes, pr.MaterialCode)
		}
		for _, code := range codes {
			if _, ok := materialIDs[code]; !ok {
				return fmt.Errorf("produk %q: material code %q tidak dikenal (tidak ada di seedMaterials)", def.Slug, code)
			}
		}
	}
	return nil
}

// seedProduct membuat satu produk + pricing rows kalau slug-nya belum ada.
// Return (true, nil) kalau produk baru dibuat, (false, nil) kalau dilewati
// karena sudah ada (idempoten).
//
// Produk + SEMUA baris pricing-nya dibungkus SATU transaksi DB — kalau gagal
// di tengah (mis. satu baris pricing gagal insert), produk yang baru dibuat
// ikut di-rollback, bukan tersisa aktif dengan pricing tidak lengkap.
func seedProduct(
	ctx context.Context,
	db *gorm.DB,
	productRepo *repository.ProductRepository,
	materialIDs map[string]uuid.UUID,
	def productDef,
) (bool, error) {
	_, err := productRepo.FindBySlug(ctx, def.Slug)
	if err == nil {
		log.Info().Str("slug", def.Slug).Msg("produk sudah ada — dilewati (idempoten)")
		return false, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return false, fmt.Errorf("cek produk %q: %w", def.Slug, err)
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txProductRepo := repository.NewProductRepository(tx)
		txPricingRepo := repository.NewPricingRepository(tx)

		p := &model.Product{
			Slug:         def.Slug,
			Name:         def.Name,
			Category:     def.Category,
			PricingType:  def.PricingType,
			MinWidthCm:   def.MinWidthCm,
			MinHeightCm:  def.MinHeightCm,
			MaxWidthCm:   def.MaxWidthCm,
			MaxHeightCm:  def.MaxHeightCm,
			DisplayOrder: def.DisplayOrder,
			IsActive:     true,
		}
		if def.Description != "" {
			description := def.Description
			p.Description = &description
		}
		if err := txProductRepo.Create(ctx, p); err != nil {
			return fmt.Errorf("buat produk %q: %w", def.Slug, err)
		}

		return seedProductPricings(ctx, txPricingRepo, materialIDs, p.ID, def)
	})
	if err != nil {
		return false, err
	}

	log.Info().Str("slug", def.Slug).Msg("produk + pricing dibuat")
	return true, nil
}

func seedProductPricings(
	ctx context.Context,
	pricingRepo *repository.PricingRepository,
	materialIDs map[string]uuid.UUID,
	productID uuid.UUID,
	def productDef,
) error {
	switch def.PricingType {
	case model.PricingTypePerM2:
		for _, pr := range def.PerM2 {
			materialID, ok := materialIDs[pr.MaterialCode]
			if !ok {
				return fmt.Errorf("produk %q: material code %q tidak dikenal", def.Slug, pr.MaterialCode)
			}
			pricePerM2 := pr.PricePerM2
			row := &model.ProductPricing{
				ProductID:  productID,
				MaterialID: materialID,
				PricePerM2: &pricePerM2,
				IsActive:   true,
			}
			if pr.MinChargeM2 > 0 {
				minCharge := pr.MinChargeM2
				row.MinChargeM2 = &minCharge
			}
			if err := pricingRepo.Create(ctx, row); err != nil {
				return fmt.Errorf("buat pricing per_m2 produk %q material %q: %w", def.Slug, pr.MaterialCode, err)
			}
		}
	case model.PricingTypePaket:
		for _, pr := range def.Paket {
			materialID, ok := materialIDs[pr.MaterialCode]
			if !ok {
				return fmt.Errorf("produk %q: material code %q tidak dikenal", def.Slug, pr.MaterialCode)
			}
			widthCm, heightCm, priceTotal := pr.WidthCm, pr.HeightCm, pr.PriceTotal
			label := pr.PackageLabel
			row := &model.ProductPricing{
				ProductID:    productID,
				MaterialID:   materialID,
				WidthCm:      &widthCm,
				HeightCm:     &heightCm,
				PackageLabel: &label,
				PriceTotal:   &priceTotal,
				IsActive:     true,
			}
			if err := pricingRepo.Create(ctx, row); err != nil {
				return fmt.Errorf("buat pricing paket produk %q material %q: %w", def.Slug, pr.MaterialCode, err)
			}
		}
	default:
		return fmt.Errorf("produk %q: pricing_type %q tidak dikenal", def.Slug, def.PricingType)
	}
	return nil
}
