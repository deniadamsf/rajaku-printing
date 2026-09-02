# Project Artikel Rajaku Printing (rajakuprinting.com)

Pusat produksi konten edukasi dan optimasi SEO on-page otomatis untuk [rajakuprinting.com](https://rajakuprinting.com). Seluruh artikel yang diproduksi diwajibkan meraih **Skor SEO 100/100** dan mematuhi **Aturan Keamanan Brand**.

---

## 🎯 Standar Skor SEO 100/100 (12 Parameter Wajib)

Kriteria di bawah **tidak ditulis ulang** di dalam script. Sejak 2 September 2026,
`auto_post_article.mjs` meng-import langsung `analyzeSeo()` dari
`frontend/composables/useSeoAnalysis.ts` — file yang sama persis yang dipakai panel
SEO di admin panel. Jadi skor CLI dijamin identik dengan skor yang tampil di layar
admin, dan kalau kriterianya suatu saat diubah, script otomatis ikut berubah
(bukan diam-diam melaporkan 100 berdasarkan aturan lama). Daftar ini hanya
rangkuman untuk dibaca manusia:

1. **Kata Kunci di Judul SEO**: Focus keyword wajib termuat di judul atau meta title.
2. **Kata Kunci di Meta Description**: Focus keyword termuat di meta description.
3. **Kata Kunci di Slug URL**: Slug URL mengandung bentuk slug dari focus keyword.
4. **Kata Kunci di Konten**: Focus keyword muncul di teks artikel.
5. **Kepadatan Kata Kunci (Keyword Density)**: Wajib antara **0.5% – 2.5%**.
6. **Kata Kunci di 10% Awal**: Wajib muncul di awal paragraf pembuka.
7. **Kata Kunci di Subheading**: Wajib muncul di minimal satu heading `## ` atau `### `.
8. **Panjang Konten**: Minimal **600 kata**.
9. **Panjang Judul SEO**: Wajib **40 – 60 karakter**.
10. **Panjang Meta Description**: Wajib **120 – 160 karakter**.
11. **Alt Text Gambar**: Gambar cover wajib memiliki alt text deskriptif.
12. **Kata Kunci Turunan**: Minimal satu *secondary keyword* muncul di dalam konten.

---

## 🚫 Aturan Keamanan Brand (Brand Safety Rule)

- **Dilarang keras menyebut brand lain / pihak ketiga** di dalam gambar cover, gambar konten, alt text, maupun body konten.
- Contoh brand terlarang: *Canva, Photoshop, CorelDraw, Illustrator, Epson, Mimaki, Roland, Snapy, Shopee, Tokopedia, dll.*
- Hanya diperbolehkan menggunakan istilah generik percetakan (*digital printing, banner outdoor, spanduk flexi, eyelet mata ayam, finishing selongsong*) atau brand internal **Rajaku Printing**.

## ✍️ Gaya Penulisan Alami (Anti-AI Pattern)

- **Jangan selalu menebalkan (bold) focus keyword maupun kata kunci turunan**.
- Penempatan kata kunci wajib mengalir secara alami (natural) layaknya tulisan manusia, tanpa pola repetitif seperti `**kata kunci**` di tiap paragraf.
- Gunakan formatting tebal (bold) hanya untuk penekanan informasi penting yang memang perlu ditonjolkan kepada pembaca, bukan untuk sengaja menandai kata kunci SEO.
- Mesin kalkulasi `useSeoAnalysis.ts` membaca teks bersih murni (*stripped text*), sehingga keyword tetap terhitung 100% tanpa perlu dibold.

---

## 🖼️ Aturan Upload Gambar (Cover & Inline Markdown)

1. **Cover Image**: Didefinisikan di `image_path` (mis. `../images/05-banner-outdoor-display.jpg`) dengan `cover_alt_text`.
2. **Gambar Tambahan (Inline Markdown)**: Disisipkan di dalam teks markdown dengan format `![Alt Text](../images/nama-file.jpg)`.
3. **Konversi Otomatis saat Auto-Post**: Script `run_auto_post.mjs` secara otomatis memindai seluruh tag gambar markdown lokal, mengunggahnya ke server `/api/v1/admin/articles/images`, mengonversinya ke WebP, dan memperbarui link markdown lokal menjadi URL server publik (`/api/v1/cms/images/:id`).
4. **Auto-Update**: Jika artikel dengan slug yang sama sudah terbit di server, script akan otomatis memperbarui (*update*) artikel beserta seluruh gambar terkait.

---

## 📁 Struktur Direktori

```
D:\RAJAKU PRINTING\articles\
├── README.md               # Dokumentasi & SOP proyek artikel ini
├── drafts/                 # Artikel siap publish (skor SEO terverifikasi 100)
│   └── 04-panduan-finishing-banner-outdoor.json
├── published/              # Arsip artikel yang sudah live
├── images/                 # Aset cover — TIDAK ikut git (lihat catatan di bawah)
│   └── 04-finishing-banner-outdoor.jpg
└── templates/              # Blueprint template artikel skor 100
    └── article-template.json
```

**Catatan soal git:** naskah artikel (`.json`), README, template, dan script
`.mjs` ikut dilacak git. Folder `images/` **tidak** — file JPG-nya rata-rata
~800 KB dan sekali ter-commit tidak bisa dihapus lagi dari riwayat git tanpa
membongkar seluruh riwayat. Konsekuensinya: kalau repo ini di-clone di komputer
lain, folder gambar akan kosong dan auto-post berjalan tanpa cover image
(script memberi peringatan, tidak diam-diam). Simpan cadangan aset mentahnya di
tempat lain, mis. Google Drive. Gambar yang sudah pernah terbit tetap aman di
server dalam bentuk WebP.

---

## 🚀 Perintah Eksekusi (CLI)

> **Target server wajib disebut.** Tidak ada nilai bawaan. Perintah tanpa `--api`
> dan tanpa `--dry-run` akan berhenti dengan pesan kesalahan, bukan menerbitkan
> ke rajakuprinting.com seperti perilaku lama. Butuh Node **v23.6+**.

### 1. Uji Skor SEO (Dry Run / Validasi Saja)

Tidak menyentuh jaringan sama sekali, jadi `--api` tidak perlu diisi.

```bash
node scripts/run_auto_post.mjs --file=articles/drafts/04-panduan-finishing-banner-outdoor.json --dry-run
```

### 2. Posting ke Server Lokal / Dev

```bash
node scripts/run_auto_post.mjs --file=articles/drafts/04-panduan-finishing-banner-outdoor.json --api=http://localhost:8080 --email=dev@example.com --password=dev
```

### 3. Auto-Post ke Live Website (rajakuprinting.com)

Karena target ini terlihat pelanggan, script menampilkan ringkasan lalu meminta
Anda **mengetik `LIVE`** sebelum mengirim apa pun. Tekan Enter saja untuk batal.

```bash
node scripts/run_auto_post.mjs --file=articles/drafts/04-panduan-finishing-banner-outdoor.json --api=https://rajakuprinting.com
```

Kredensial paling aman diisi lewat mode interaktif seperti di atas (password
tidak tersimpan di riwayat terminal). Untuk otomasi/CI yang tidak bisa
menerima ketikan, tambahkan `--yes-live` — flag itu melewati konfirmasi, jadi
pakai hanya kalau memang disengaja.
