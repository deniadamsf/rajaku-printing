// Package model — entitas domain modul sitemedia: registry slot gambar
// landing page + baris DB `site_media`.
package model

// SlotKey — kunci unik satu slot gambar landing page (mis. "hero_poster_desktop").
// String literal ini yang dipakai sebagai primary key tabel site_media DAN
// sebagai path segment endpoint publik penyaji file
// (GET /api/v1/site-media/file/:slot).
type SlotKey string

// SlotDef — definisi satu slot: label & saran dimensi untuk admin panel.
// Registry didefinisikan di kode (bukan tabel) supaya menambah slot baru =
// tambah satu entri di sini, TIDAK perlu migration.
type SlotDef struct {
	Key         SlotKey `json:"slot"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	// SuggestedWidthPx/HeightPx — dimensi yang disarankan untuk hasil visual
	// terbaik (bukan validasi keras — admin tetap boleh unggah dimensi lain).
	SuggestedWidthPx  int `json:"suggested_width_px"`
	SuggestedHeightPx int `json:"suggested_height_px"`
}

// Daftar resmi kunci slot landing page. Section 16-18 CLAUDE.md menjelaskan
// aset yang dipakai: hero cinematic (poster fallback), logo brand, langkah
// proses cetak, dan OG image untuk share sosial media.
const (
	SlotHeroPosterDesktop SlotKey = "hero_poster_desktop"
	SlotHeroPosterMobile  SlotKey = "hero_poster_mobile"

	SlotBrandLogoFull   SlotKey = "brand_logo_full"
	SlotBrandLogoFullSM SlotKey = "brand_logo_full_sm"
	SlotBrandLogoMark   SlotKey = "brand_logo_mark"

	SlotProses1 SlotKey = "proses_1"
	SlotProses2 SlotKey = "proses_2"
	SlotProses3 SlotKey = "proses_3"
	SlotProses4 SlotKey = "proses_4"
	SlotProses5 SlotKey = "proses_5"
	SlotProses6 SlotKey = "proses_6"
	SlotProses7 SlotKey = "proses_7"
	SlotProses8 SlotKey = "proses_8"

	SlotBahanDetail SlotKey = "bahan_detail"

	SlotTentangWorkshop SlotKey = "tentang_workshop"
	SlotTentangTim      SlotKey = "tentang_tim"

	SlotOGImage SlotKey = "og_image"

	SlotQRIS SlotKey = "qris_code"
)

// Registry — daftar SEMUA slot yang dikenal sistem, urut tampil di admin
// panel. Menambah slot baru: tambah konstanta di atas + satu entri di sini.
var Registry = []SlotDef{
	{
		Key:               SlotHeroPosterDesktop,
		Label:             "Hero Poster (Desktop)",
		Description:       "Gambar statis fallback/poster untuk hero cinematic scroll-scrub desktop (section 16.1) — tampil sebelum frame sequence termuat.",
		SuggestedWidthPx:  1920,
		SuggestedHeightPx: 1080,
	},
	{
		Key:               SlotHeroPosterMobile,
		Label:             "Hero Poster (Mobile)",
		Description:       "Gambar statis hero untuk Layar 1 mobile (section 18) — bukan scroll-scrub, cukup satu gambar/loop pendek.",
		SuggestedWidthPx:  1080,
		SuggestedHeightPx: 1350,
	},
	{
		Key:               SlotBrandLogoFull,
		Label:             "Logo Lengkap (Wordmark)",
		Description:       "Logo Rajaku Printing lengkap dengan wordmark, dipakai navbar/footer desktop.",
		SuggestedWidthPx:  480,
		SuggestedHeightPx: 160,
	},
	{
		Key:               SlotBrandLogoFullSM,
		Label:             "Logo Lengkap (Kecil)",
		Description:       "Varian logo lengkap ukuran kecil untuk navbar mobile / area sempit.",
		SuggestedWidthPx:  240,
		SuggestedHeightPx: 80,
	},
	{
		Key:               SlotBrandLogoMark,
		Label:             "Logo Mark (Ikon)",
		Description:       "Logo mark saja (tanpa wordmark), dipakai favicon-adjacent / avatar.",
		SuggestedWidthPx:  160,
		SuggestedHeightPx: 160,
	},
	{
		Key:               SlotProses1,
		Label:             "Langkah Proses 1",
		Description:       "Ilustrasi langkah pertama alur proses cetak di landing page.",
		SuggestedWidthPx:  800,
		SuggestedHeightPx: 600,
	},
	{
		Key:               SlotProses2,
		Label:             "Langkah Proses 2",
		Description:       "Ilustrasi langkah kedua alur proses cetak di landing page.",
		SuggestedWidthPx:  800,
		SuggestedHeightPx: 600,
	},
	{
		Key:               SlotProses3,
		Label:             "Langkah Proses 3",
		Description:       "Ilustrasi langkah ketiga alur proses cetak di landing page.",
		SuggestedWidthPx:  800,
		SuggestedHeightPx: 600,
	},
	{
		Key:               SlotProses4,
		Label:             "Langkah Proses 4",
		Description:       "Ilustrasi langkah keempat alur proses cetak di landing page.",
		SuggestedWidthPx:  800,
		SuggestedHeightPx: 600,
	},
	{
		Key:               SlotProses5,
		Label:             "Langkah Proses 5",
		Description:       "Ilustrasi langkah kelima alur proses cetak di landing page.",
		SuggestedWidthPx:  800,
		SuggestedHeightPx: 600,
	},
	{
		Key:               SlotProses6,
		Label:             "Langkah Proses 6",
		Description:       "Ilustrasi langkah keenam alur proses cetak di landing page.",
		SuggestedWidthPx:  800,
		SuggestedHeightPx: 600,
	},
	{
		Key:               SlotProses7,
		Label:             "Langkah Proses 7",
		Description:       "Detail panel kontrol & tabung tinta CMYK mesin cetak, dipakai alur proses cetak di landing page.",
		SuggestedWidthPx:  800,
		SuggestedHeightPx: 800,
	},
	{
		Key:               SlotProses8,
		Label:             "Langkah Proses 8",
		Description:       "Detail print-head/carriage saat mencetak, dipakai alur proses cetak di landing page.",
		SuggestedWidthPx:  1000,
		SuggestedHeightPx: 750,
	},
	{
		Key:               SlotBahanDetail,
		Label:             "Foto Bahan & Material",
		Description:       "Foto close-up bahan/roll banner untuk section spesifikasi bahan di landing page.",
		SuggestedWidthPx:  1200,
		SuggestedHeightPx: 900,
	},
	{
		Key:               SlotTentangWorkshop,
		Label:             "Foto Workshop (Tentang Kami)",
		Description:       "Foto tempat/workshop untuk halaman Tentang Kami.",
		SuggestedWidthPx:  1600,
		SuggestedHeightPx: 1000,
	},
	{
		Key:               SlotTentangTim,
		Label:             "Foto Tim (Tentang Kami)",
		Description:       "Foto tim/operator untuk halaman Tentang Kami.",
		SuggestedWidthPx:  1200,
		SuggestedHeightPx: 900,
	},
	{
		Key:               SlotOGImage,
		Label:             "OG Image (Share Sosial Media)",
		Description:       "Gambar preview saat link website dibagikan ke WhatsApp/Facebook/Twitter (meta og:image).",
		SuggestedWidthPx:  1200,
		SuggestedHeightPx: 630,
	},
	{
		Key:               SlotQRIS,
		Label:             "Kode QRIS Pembayaran",
		Description:       "Gambar QRIS statis yang dipindai pembeli (section 7). Unggah cetakan QRIS resmi apa adanya — jangan dipotong sampai mengenai pola sudut, karena kode jadi gagal dipindai.",
		SuggestedWidthPx:  800,
		SuggestedHeightPx: 1130,
	},
}

// registryIndex — lookup cepat by key, dibangun sekali saat init.
var registryIndex = buildRegistryIndex()

func buildRegistryIndex() map[SlotKey]SlotDef {
	idx := make(map[SlotKey]SlotDef, len(Registry))
	for _, def := range Registry {
		idx[def.Key] = def
	}
	return idx
}

// IsValidSlot melaporkan apakah key terdaftar di registry.
func IsValidSlot(key string) bool {
	_, ok := registryIndex[SlotKey(key)]
	return ok
}

// SlotByKey mengembalikan definisi slot, atau false kalau key tidak dikenal.
func SlotByKey(key string) (SlotDef, bool) {
	def, ok := registryIndex[SlotKey(key)]
	return def, ok
}
