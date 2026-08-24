# Rajaku Printing — Project Spec (v1.0 — Final)

> Dokumen acuan untuk vibe coding di Claude Code. Seluruh keputusan arsitektur & fitur sudah difix — tidak ada lagi item TBD terbuka. Siap dijadikan `CLAUDE.md` di root repo.

## 1. Overview
Company profile + web app manajemen order percetakan banner. Modular monolith. Target pengembangan lanjutan: app Android (Flutter) untuk operasional cetak.

## 2. Tech Stack
- **Frontend**: Nuxt.js 3 (SSR/SSG untuk SEO)
- **Backend**: Golang (Gin + GORM, pola sama seperti proyek lain)
- **DB**: PostgreSQL
- **Animasi 3D**: TresJS (Three.js wrapper Vue) — bukan Three.js mentah
- **Animasi motion**: motion-v (port Framer Motion untuk Vue) — dipilih karena lebih cepat/ringkas untuk development dibanding GSAP
- **Auth**: OAuth (Google minimal) + guest checkout tanpa akun
- **WA Gateway**: Baileys (self-hosted, gratis)
- **PDF invoice**: gofpdf / maroto (Go)
- **Image processing**: auto-convert artikel ke WebP saat upload

> **Konfigurasi URL terpusat (wajib)**: semua base URL (link tracking `wa.me`, link invoice, API base untuk frontend, dll) **hanya boleh dibaca dari satu sumber**: env var `APP_BASE_URL` di backend Go (dibaca sekali saat startup ke config struct, di-inject ke service yang butuh — bukan `os.Getenv()` tersebar di banyak file) dan `runtimeConfig` di Nuxt untuk frontend. Ganti localhost → domain produksi tinggal ubah 1 baris di `.env`, tidak perlu grep-replace ke banyak file. Ini langsung mencegah kelas bug yang pernah ditemukan sebelumnya (base URL notifikasi ke-hardcode salah).

## 3. Arsitektur Modular
Modul terpisah (masing-masing punya service/route sendiri, komunikasi via internal API/event):
1. `auth` — OAuth + guest session (customer) + login karyawan. **Satu form login untuk semua tipe user** — setelah autentikasi, backend baca role lalu redirect otomatis: customer → halaman akun/lacak order, staff → admin panel sesuai menu ter-toggle untuk role-nya. **Akun karyawan tidak punya rute pendaftaran publik** — hanya bisa dibuat oleh super admin dari admin panel. Public hanya bisa daftar sebagai customer.
   - Tabel `customer` dipakai bersama lintas channel: kolom `customer_type` (`guest`/`registered`), nomor WA sebagai **unique matching key** — 1 nomor WA = 1 identitas pelanggan, baik dia order online maupun walk-in (lihat section 11).
2. `catalog` — produk, bahan, harga (customizable dari frontend admin)
3. `order` — order lifecycle, state machine, resi
4. `payment` — manual verification (upload bukti, approve/reject)
5. `shipping` — ongkir manual by admin, pickup vs kirim
6. `design` — upload file desain, request desain, revisi
7. `production` — update status cetak/QC (staff produksi)
8. `notification` — WA gateway (Baileys, Node.js microservice terpisah dipanggil via internal API dari Go), semua trigger status
9. `invoice` — generate struk (thermal) & invoice (PDF/online)
10. `pos` — order walk-in oleh kasir
11. `cms` — artikel SEO, auto-webp
12. `admin` — role & menu toggle per staff
13. `tracking` — public endpoint cek resi (no-login)
14. `discount` — master diskon (persen/nominal), masa berlaku, kuota, soft-delete; nilainya di-*snapshot* ke order saat dipakai (§28)

> Rekap order (§28.5) **tidak** jadi modul sendiri — ia laporan baca-saja di atas tabel `orders`, jadi tinggal di modul `order` (`repository.Recap` → `service.RecapOrders` → `GET /admin/orders/recap`). Bikin modul `report` terpisah malah memaksa cross-module read ke internal `order`, yang dilarang §22.

## 4. Order State Machine
```
order_masuk
→ menunggu_ongkir        (admin set ongkir, kecuali pickup)
→ menunggu_pembayaran    (total fix, notif WA ke pembeli)
→ menunggu_verifikasi    (pembeli upload bukti transfer/QRIS, ATAU cash/QRIS langsung di POS)
→ dibayar / ditolak      (ditolak → balik ke menunggu_verifikasi, minta upload ulang)
→ desain_diverifikasi    (ada revisi jika request desain / upload desain invalid)
   → [jika request desain] desain_dikerjakan → menunggu_approval_desain → revisi/oke
→ proses_cetak
→ qc
→ siap_kirim / siap_ambil
→ dikirim (jika kirim)
→ selesai
→ dibatalkan (bisa terjadi dari status manapun sebelum proses_cetak, mis. timeout bayar)
```
- Order **online** vs **walk-in (POS)** pakai state machine yang **sama**, cuma `menunggu_pembayaran` & `menunggu_verifikasi` bisa auto-skip untuk POS (cash/QRIS langsung dikonfirmasi kasir).
- Field tambahan: `metode_ambil` (pickup/kirim), `metode_bayar` (transfer/qris/cash), `channel` (online/pos).

## 5. Resi & Tracking (Public, No-Login)
- Format resi: **acak**, bukan berurutan. Contoh: `RJK-8F3K2A9X`.
- Endpoint publik `/lacak/:resi` → tampilkan history status saja.
- Data sensitif disensor: alamat & no. HP dipotong (mis. `Jl. Merdek**`, `0812****678`).

## 6. Order Jenis: Upload Desain vs Request Desain
Dua flow beda di modul `design`:
- **Upload desain sendiri**: format CDR/AI/PDF/JPG/PNG → verifikasi file → lanjut cetak.
  - Catatan teknis: CDR/AI **tidak bisa** di-preview di browser, staff harus download & buka manual. PDF/JPG/PNG bisa preview langsung.
- **Request desain**: pembeli upload aset (logo/foto) + brief → biaya jasa desain (harga terpisah) → dikerjakan staff desain → approval/revisi pembeli.

## 7. Payment — Manual
- Tidak pakai payment gateway. Tampilkan no. rekening/QRIS statis.
- Pembeli upload bukti transfer → staff verifikasi (approve/reject) di admin panel.
- Untuk POS: metode `cash`/`qris` di tempat → langsung approve oleh kasir, tanpa upload bukti.

## 8. Shipping
- Ongkir **tidak otomatis** — admin input manual per order setelah order masuk (sebelum pembeli bayar).
- Opsi `metode_ambil`: pickup (skip ongkir) / dikirim (admin isi ongkir).

## 9. Katalog & Pricing
- Fully customizable dari admin frontend: bahan, size, harga.
- **Model harga fleksibel per produk** (bukan hardcode satu model global): saat setup produk di admin, admin pilih tipe kalkulasi per produk — `per_m2` (harga × lebar × tinggi, untuk banner custom size) atau `paket` (harga fix per ukuran umum, mis. 1x2m). Struktur DB: tabel `product` punya field `pricing_type` + tabel `pricing_rule` menyesuaikan tipe yang dipilih.

## 10. Admin Panel & Role
- **Akun karyawan hanya dibuat oleh super admin** dari dalam admin panel (menu "Kelola Staff") — tidak ada halaman pendaftaran publik untuk staff. Super admin input email/nomor HP staff, sistem kirim undangan set password, lalu assign role.
- Role dibuat **modular saat create akun staff** — tiap role punya set menu yang bisa **toggle on/off** oleh admin utama (bukan role hardcoded).
- Login staff & customer pakai **form yang sama** di frontend publik — redirect otomatis ke area masing-masing berdasarkan role setelah autentikasi berhasil (lihat section 3, modul `auth`).
- Role awal yang sudah jelas:
  - Staff verifikasi pembayaran (lihat bukti transfer, approve/reject)
  - Staff produksi (update status cetak/QC)
  - Staff desain (kerjakan request desain)
  - Admin artikel (tulis konten SEO, auto-webp)
  - Kasir/POS (buat order walk-in, terima cash/QRIS, cetak struk)
- Implementasi: tabel `role` + tabel `menu_permission` (many-to-many), bukan role enum statis — supaya toggle-able.
- **Pertimbangan keamanan**: karena akun staff akses data finansial/order, sebaiknya beri proteksi ekstra dibanding akun customer — rate limit percobaan login lebih ketat, opsional 2FA untuk role sensitif (verifikasi pembayaran, super admin).

## 11. POS / Walk-in — Alur Lengkap

Flow dijalankan murni dari layar perangkat kasir (staff), didesain supaya pelanggan **tidak perlu daftar/isi form sendiri** — kasir yang input.

**1. Input data & buat order**
- Kasir buka "Order Baru (POS)" di admin panel → input Nama + No. WA pelanggan (sistem auto-format ke `62xxx`, validasi sama seperti online) → input detail pesanan (jenis banner, ukuran, bahan → harga terhitung otomatis sesuai `pricing_type` produk, section 9).

**2. Resolusi identitas pelanggan (di backend, transparan buat kasir)**
- Saat tombol "Buat Pesanan" diklik, backend cek nomor WA di tabel `customer` (nomor WA = **matching key**, wajib unique index):
  - **Belum ada** → auto-create customer baru dengan `customer_type = guest` (cuma simpan nama & no. WA, tanpa password/OAuth).
  - **Sudah ada** (baik dari order online sebelumnya maupun walk-in sebelumnya) → order ditempel ke `customer_id` yang sama, sehingga riwayat transaksi pelanggan gabung lintas channel (online + offline) — satu pelanggan, satu riwayat, apapun channel-nya.
- Resi (`RJK-xxxxxxxx`) digenerate saat itu juga — implementasi **retry-on-collision**: generate random string → cek unique constraint di DB → kalau bentrok (kemungkinan kecil tapi harus dihandle), generate ulang. Jangan asumsikan random string selalu unik tanpa dicek.

**3. Pembayaran instan (bypass verifikasi)**
- Kasir pilih metode `Lunas (Cash/QRIS di tempat)` → order langsung masuk status `dibayar`, **skip** status `menunggu_pembayaran`/`menunggu_verifikasi` yang dipakai jalur online (uangnya sudah di tangan kasir, tidak perlu verifikasi async).
- Field `channel: pos` & `metode_bayar: cash/qris_pos` tercatat untuk rekonsiliasi harian (lihat poin rekap di bawah).

**4. Eksekusi: struk & notifikasi (async, tidak boleh blocking response)**
- Sistem cetak struk thermal (section 12) berisi nomor resi — response ke layar kasir harus cepat, jangan nunggu proses lain selesai.
- Pengiriman WA ("Halo [Nama], pesanan Anda sedang diproses. Lacak di: [APP_BASE_URL]/lacak/[resi]") **dilempar ke job queue**, tidak dikirim sinkron dalam request yang sama — supaya kalau WA gateway lambat/gagal, kasir tidak ikut nge-hang nunggu. Link dibangun dari `APP_BASE_URL` terpusat (lihat section 2), bukan hardcode.

**Alur desain untuk walk-in (disederhanakan, tidak ikut loop revisi online)**
- **Skenario A — bawa desain siap cetak**: staff upload file ke order (flow sama seperti "upload desain sendiri", section 6), cek cepat di tempat, langsung lanjut ke `proses_cetak` begitu file oke.
- **Skenario B — minta edit di tempat**: staff/desainer edit pakai software yang biasa dipakai (CorelDraw/Illustrator — tidak perlu editor bawaan di sistem, cukup upload hasil editan ke order). Customer approve **verbal** di tempat → staff klik tombol **"Disetujui Langsung"** di order → sistem **skip** status `menunggu_approval_desain` & notifikasi WA (approval sudah terjadi tatap muka, tidak perlu digital loop).
- Kalau editan butuh waktu lebih lama dari yang customer bisa tunggu → order lanjut ke alur standar `desain_dikerjakan → menunggu_approval_desain` (customer dihubungi via WA nanti, sama seperti online).
- Field tambahan di order: `design_approval_mode` (`instant_walkin` / `async_notify`) — membedakan dua skenario ini di data, berguna untuk rekap (persentase walk-in selesai di tempat vs butuh follow-up).

**Rekonsiliasi**
- Laporan rekonsiliasi harian: total order per kasir, breakdown metode bayar.

## 12. Nota & Invoice
- **Thermal printer** (58mm/80mm): mulai dari `window.print()` + CSS `@media print` layout struk (MVP). ESC/POS native (`node-thermal-printer` atau raw command dari Go) — **tahap 2** kalau volume transaksi tinggi.
- **Invoice online**: generate PDF dari data order, bisa didownload dari halaman lacak resi, dan/atau dikirim via WA.
- Satu struktur data invoice → dua template render (struk ringkas vs PDF A4 lengkap).
- **Auto-send**: invoice PDF otomatis dikirim ke WA pelanggan (link download, bukan attachment langsung — lebih ringan & reliable via Baileys) setiap kali invoice ter-generate/update. Butuh nomor format `62xxx` (lihat section 13) supaya link `wa.me` valid.

## 13. Notifikasi WhatsApp
- **Gateway: Baileys** (library Node.js, protokol WhatsApp Web, self-hosted, gratis tanpa biaya per pesan). Konsekuensi yang perlu disadari: ini **tidak resmi** (bukan WhatsApp Business API), jalan dengan cara "meniru" WhatsApp Web — risikonya nomor bisa kena banned/blokir sementara oleh WhatsApp kalau kirim pesan terlalu cepat/banyak dalam waktu singkat, dan tidak ada SLA/support resmi. Untuk skala UMKM/awal ini umum dipakai & cukup stabil asal tidak spam.
- Karena Baileys berbasis Node.js sedangkan backend utama Golang, ini jadi **microservice terpisah** (`notification-worker`, Node.js kecil) yang dipanggil backend Go via internal API/queue — sejalan dengan arsitektur modular.
- **Format nomor HP wajib distandarkan ke `62xxxxxxxxxx`** (bukan `08xxx`) sejak input di form order/registrasi — divalidasi & dikonversi otomatis (strip `0` di depan, ganti jadi `62`). Ini wajib karena link `wa.me/62xxx` dan API Baileys butuh format internasional tanpa `+`/`0` di depan.
- Trigger notifikasi minimal:
  - Ongkir & total sudah fix (menunggu pembayaran)
  - Pembayaran diverifikasi
  - Desain butuh revisi / disetujui
  - Siap kirim/ambil
  - Resi pengiriman (kalau dikirim ekspedisi)
  - Invoice PDF (lihat section 12 — auto-send)
  - Notif order masuk untuk walk-in (lihat section 11)

**Mekanisme queue — apakah perlu Redis?**

**Tidak wajib untuk MVP.** Yang benar-benar dibutuhkan bukan Redis itu sendiri, tapi *ada mekanisme async job* supaya pengiriman WA tidak blocking response utama (mis. kasir klik "Buat Pesanan" tidak boleh nunggu Baileys selesai kirim pesan). Untuk skala 1 VPS, cukup pakai **tabel Postgres sebagai job queue**:
- Tabel `notification_job` (kolom: `payload`, `status` [`pending`/`sent`/`failed`], `retry_count`, `created_at`) — backend Go insert row saat event terjadi (pembayaran diverifikasi, dll), lalu `notification-worker` poll tabel ini tiap beberapa detik, kirim via Baileys, update status.
- Ini menghindari dependency infra tambahan (selaras dengan preferensi kamu pakai infra hemat biaya, bukan enterprise-grade) — Postgres yang sudah ada cukup rangkap jadi queue sederhana.

**Kapan baru worth pakai Redis** (bukan sekarang, tapi dicatat sebagai trigger di masa depan):
- Volume notifikasi sudah tinggi & polling Postgres mulai berasa lambat/boros query.
- Backend Go di-scale jadi lebih dari 1 instance (perlu rate-limiting/lock yang konsisten lintas instance, mis. cegah 2 instance generate resi collision bersamaan — meski ini juga bisa diatasi unique constraint DB).
- Butuh caching layer buat data yang sering dibaca tapi jarang berubah (mis. katalog produk) untuk kurangi beban DB saat traffic besar.
- Butuh fitur real-time (mis. dashboard admin update live tanpa refresh) via pub/sub.

Kalau salah satu dari itu belum jadi masalah nyata, tambah Redis sekarang cuma menambah kompleksitas ops (butuh maintain service baru di VPS) tanpa manfaat langsung.

## 14. CMS Artikel (SEO)
- Menu tulis artikel di admin (role: Admin Artikel).
- Upload gambar → **auto-convert ke WebP**.
- Meta title/description per artikel, slug custom, schema markup `Article` + `LocalBusiness`.

## 15. SEO Requirements (Web Utama)
- Nuxt 3 SSR/SSG untuk semua halaman publik (landing, katalog, artikel, lacak resi).
- Sitemap otomatis, robots.txt, meta tag dinamis per halaman.
- Schema markup `LocalBusiness` untuk local SEO (Trenggalek/Jawa Timur).
- Core Web Vitals: lazy-load gambar, WebP semua asset visual.

## 16. Frontend 3D/Motion (Landing Page) — FINAL

**Keputusan final (hybrid, 2 teknik berbeda untuk 2 kebutuhan berbeda):**

1. **Hero cinematic scroll (desktop)** — teknik **video-per-frame** (bukan objek 3D asli): produksi video pendek (real footage proses cetak, atau motion graphics), lalu di-convert jadi sequence frame gambar (60-150 frame) pakai `ffmpeg` (`ffmpeg -i video.mp4 -vf "fps=X" frame_%03d.png`). Frame di-load ke `<canvas>`, digambar sesuai posisi scroll pakai **GSAP ScrollTrigger** (`scrub: true`). Dipilih karena tidak butuh skill 3D modeling, cukup produksi video/motion graphics biasa.
2. **Mockup produk interaktif** — **TresJS** (WebGL asli), tapi geometri **sederhana** (plane/box dengan texture map desain banner di atasnya) — bukan scene 3D kompleks, jadi tetap ringan meski real-time. Khusus untuk preview banner yang bisa diputar user (drag/rotate) sebelum order — fitur bisnis, bukan dekorasi. Wajib lazy-load (`<ClientOnly>` + `IntersectionObserver`), **tidak dipasang di homepage** (lihat section 17 — hanya di halaman Showcase/Galeri).
3. **Microinteraction & transisi umum** (fade, slide, hover) → **motion-v** (final, dipilih karena lebih cepat/ringkas untuk development).
4. **Catatan penting**: GSAP ScrollTrigger di poin 1 di atas **tetap dipakai khusus untuk hero cinematic scroll-scrub**, terlepas dari motion-v dipilih untuk microinteraction umum — motion-v tidak punya utilitas setara untuk scrub canvas image-sequence berbasis posisi scroll presisi piksel. Jadi kedua library ini **hidup berdampingan** dalam project: motion-v untuk transisi/microinteraction biasa, GSAP ScrollTrigger khusus untuk 1 kebutuhan spesifik di hero. Ini keputusan sadar, bukan inkonsistensi.
5. **Klarifikasi istilah**: "Framer" (design tool) ≠ "Framer Motion/Motion" (library JS, basis motion-v). Framer tool tidak dipakai — tidak kompatibel dengan Nuxt custom stack.

Konten sequence gambar/video untuk poin 1 perlu diproduksi (foto/video workshop asli atau CGI render) — ini kerja konten terpisah dari coding, disiapkan sebelum development frontend hero dimulai.

## 17. Navigasi — CTA Order & Login Harus Selalu Terlihat

- **Sticky navbar** di semua halaman publik (desktop & mobile) dengan 2 elemen wajib selalu visible:
  - **"Order Banner"** — CTA utama, paling menonjol secara visual (warna kontras/button solid), posisi kiri-tengah navbar atau sejajar logo.
  - **"Login"** — pola umum di kanan atas navbar, redirect ke form login unified (lihat section 10).
- **Mobile**: sticky top bar tetap pakai pola sama; pertimbangkan tambahan **sticky bottom bar** khusus CTA "Order Banner" di Layar 1 (hero) mobile — pola umum e-commerce mobile (tombol besar selalu terjangkau jempol).
- Order Banner CTA mengarah langsung ke form order (bisa guest checkout, sesuai section 6), bukan ke halaman katalog dulu — biar friksi order minimal.

## 18. Strategi Mobile/Responsive — Pola "Scaffold" 2 Layar

Pola "2 screen (scaffold di mobile)" diimplementasikan sebagai 2 halaman terpisah di mobile (mirip `Scaffold` di Flutter — tiap layar unit mandiri), bukan sekadar CSS resize dari desktop.

Ini bukan cuma responsive (resize CSS), tapi **adaptive** — mobile render pohon komponen yang beda dari desktop, supaya aset berat desktop (scroll-scrub sequence, TresJS) tidak ikut ke-load di HP:

- **Layar 1 — Hero/Landing (mobile)**: hero statis (1 gambar/video pendek loop, bukan scroll-scrub), CTA utama **"Order Banner"** (konsisten dengan section 17), animasi ringan saja (fade/slide via motion-v). Tidak ada scroll-scrub sequence maupun TresJS di sini.
  - **Tinggi hero mobile = satu layar penuh**: `min-h-[calc(100svh-3.5rem)]` (tinggi layar dikurangi navbar), gambar `object-cover` full-bleed. Pakai `svh`, **jangan** `100vh` — di browser HP `100vh` dihitung tanpa bar URL, sehingga kaki hero (tempat tombolnya) tersembunyi saat halaman pertama dibuka.
  - Aset poster mobile idealnya **potret** (9:16, minimal 1080×1920). Foto lanskap yang dipaksa jadi potret akan ter-crop besar dan pecah — bisa diganti kapan saja lewat `/admin/site-media` slot `hero_poster_mobile` tanpa deploy ulang.
  - **Scroll-reveal jangan menyembunyikan konten paruh atas**: `in-view-options` di landing pakai `margin: '-40px'`. Dengan `-100px`, elemen yang sudah setengah terlihat saat halaman dibuka tetap `opacity: 0` sampai user menggulir — yang terlihat sebagai ruang kosong besar, bukan sebagai animasi.
- **Layar 2 — Showcase/Galeri (halaman terpisah)**: diakses dari CTA di Layar 1. Berisi galeri produk pakai swipeable carousel, dan **di halaman inilah** TresJS mockup viewer dimuat (kalau device mendukung) — karena sudah jadi halaman khusus, bukan bagian dari homepage yang harus cepat diakses.
- **Deteksi & rendering**: pakai composable (`useDevice()`/breakpoint check) untuk conditional component rendering di level Nuxt (bukan cuma `display:none` di CSS), supaya JS bundle desktop-only benar-benar tidak terkirim ke mobile.
- Desktop tetap dapat pengalaman penuh: hero scroll-scrub cinematic (section 16) dalam satu halaman kontinu.

## 19. File Storage — Retention Policy (Design Files)

- **File desain** (upload customer: CDR/AI/PDF/JPG/PNG, hasil kerja staff desain) **dihapus otomatis dari storage setelah 30 hari** sejak upload/update terakhir — dijalankan via scheduled job (Go: `robfig/cron`, atau cron VPS yang panggil endpoint cleanup internal).
- **Data tertulis (untuk rekap) TIDAK ikut terhapus**: record di tabel `design_file` tetap ada (nama file asli, ukuran, tanggal upload, order_id terkait, siapa yang upload) — yang dihapus **hanya blob fisik file**. Tandai dengan `is_purged=true` + `purged_at`, kosongkan field `file_url`.
- **Reminder sebelum hapus**: kirim notif ke staff (dashboard/WA internal) H-3 sebelum file dihapus jika order terkait belum berstatus `selesai` — supaya staff sempat download manual kalau butuh reprint.
- Retensi 30 hari sebaiknya **dibuat configurable** (bukan hardcode), disimpan di setting admin — biar bisa diubah tanpa deploy ulang kalau kebijakan berubah.
- **Lokasi storage: disk lokal VPS Hostinger** (final, tidak pakai object storage terpisah). Konsekuensi yang perlu disiapkan:
  - Job cleanup retention (30 hari) jadi krusial untuk jaga kapasitas disk VPS tidak penuh — pastikan job ini benar-benar jalan reliable (monitoring/alert kalau job gagal jalan).
  - Perlu **monitoring sisa disk space** (mis. alert kalau > 80% terpakai), karena tidak ada auto-scaling storage seperti object storage.
  - Sebaiknya ada **backup terpisah** untuk file yang masih dalam masa retensi (mis. rsync berkala ke lokasi lain), karena disk VPS tunggal tidak redundant — kalau VPS bermasalah, file desain yang belum 30 hari bisa ikut hilang.
- Scope saat ini hanya file desain — bukti transfer pembayaran belum termasuk kebijakan ini (bisa disamakan nanti kalau perlu, catat sebagai keputusan terpisah).

## 20. Skill yang Direkomendasikan untuk Claude Code

Selain skill yang sudah kamu punya (`ui-ux-pro-max`, sudah disesuaikan ke Vue), sebaiknya buat skill khusus proyek ini biar konsisten setiap sesi vibe coding (tidak perlu jelaskan ulang aturan tiap kali):

| Skill | Isi/Fungsi |
|---|---|
| `go-modular-conventions` | Encode aturan section 22-23 (layer handler→service→repository, format error wrapping, response envelope, testing minimum) — dipakai tiap bikin modul Go baru |
| `order-state-machine-ref` | Nama status & transisi resmi dari section 4 — cegah Claude Code bikin nama status baru yang tidak konsisten antar modul |
| `nuxt-seo-conventions` | Pola meta tag dinamis, schema markup, sitemap, pipeline auto-webp — dipakai di modul `cms` & halaman publik |
| `scrollytelling-hero` | Pola implementasi GSAP ScrollTrigger + canvas image-sequence (section 16 poin 1), plus pola lazy-load TresJS (`ClientOnly`+`IntersectionObserver`) |
| `baileys-notification-worker` | Konvensi microservice Node.js: queue, rate-limit aman (cegah banned), retry/backoff, format nomor `62xxx` |

**Tambahan non-skill**: sub-agent `code-reviewer` (selain `ui-fixer` yang sudah ada) yang otomatis cek 4 rule inti section 22 (layer separation, error wrapping, no-silent-stub, config fail-fast) sebelum modul dianggap selesai — jadi gate otomatis, bukan manual diingat-ingat.

## 21. Roadmap Lanjutan (Bukan MVP)
- App Android (Flutter) untuk operasional cetak/produksi — konsumsi API Golang yang sama (pastikan backend API-first, token-based auth, bukan session-cookie-only).
- ESC/POS native printing.
- Role granular tambahan jika perlu.

## 22. Aturan Coding — Anti Spaghetti Code (Golang)

**Struktur & separation of concern**
- Layer wajib dipisah tegas: `handler` (HTTP only, no business logic) → `service` (business logic) → `repository` (akses DB only). Handler **tidak boleh** langsung panggil GORM.
- Per modul (lihat section 3) punya folder sendiri: `internal/order/{handler,service,repository,model}.go`. Dilarang modul saling import langsung ke internal package modul lain — komunikasi lewat interface/event, biar dependency antar modul tidak jadi jaring kusut.
- Satu fungsi = satu tanggung jawab. Kalau fungsi service > ~50 baris atau nesting if > 3 level, wajib dipecah.
- Dilarang "god struct/service" yang pegang semua logic (mis. satu `OrderService` yang juga urus payment, shipping, WA notif sekaligus). Delegasikan ke service modul terkait.

**Error handling (biar gampang dilacak)**
- **Setiap** `err != nil` wajib langsung ditangani/dikembalikan — dilarang `_ = err` atau diabaikan diam-diam.
- Bungkus error dengan konteks: `fmt.Errorf("verifikasi pembayaran order %s: %w", orderID, err)` — bukan cuma `return err` polos, biar log/stack trace tahu ini gagal di mana.
- Pakai sentinel/custom error (`errors.Is`/`errors.As`) untuk error yang perlu dibedakan penanganannya (mis. `ErrPaymentAlreadyVerified`, `ErrInvalidReceipt`), bukan compare string error.
- `panic` **hanya** boleh untuk bug fatal saat startup (config invalid dll), tidak untuk alur bisnis normal. Recover middleware di level HTTP server saja sebagai safety net.
- Semua response API pakai format envelope konsisten: `{ "success": bool, "data": ..., "error": { "code": ..., "message": ... } }` — supaya frontend & log gampang parse error di semua endpoint.

**Cegah "kelihatan selesai padahal belum" (kasus stub/JSON parsing kemarin)**
- Dilarang commit fungsi stub yang return dummy/hardcoded value tanpa marker eksplisit `// TODO(nama): belum diimplementasi — alasan`. Stub tanpa marker = bug tersembunyi.
- Config (base URL notifikasi, kredensial WA gateway, dll) **wajib divalidasi saat startup** (fail fast) — app harus gagal jalan kalau env var wajib kosong/salah, bukan gagal diam-diam saat runtime di production.
- Setiap modul yang menyentuh data kritis (payment, sync offline, notifikasi) wajib punya minimal 1 unit test untuk "happy path" + 1 test untuk kasus gagal, sebelum dianggap selesai.

**Tooling wajib (jalan otomatis, bukan manual ingat-ingat)**
- `golangci-lint` + `go vet` jalan di setiap commit/PR (bisa via pre-commit hook atau CI sederhana).
- `gofmt`/`goimports` wajib rapi sebelum commit.
- Struct DB migration pakai tool migration (`golang-migrate`), jangan auto-migrate GORM di production — biar perubahan schema tercatat & bisa di-rollback.

## 23. Kategori Aturan Lain yang Direkomendasikan

1. **Git/commit discipline** — 1 commit/PR = 1 modul/fitur, jangan campur perubahan lintas modul. Commit message jelas (`feat(order): tambah state menunggu_ongkir`), supaya gampang di-audit atau di-revert kalau vibe coding menghasilkan bug.
2. **Definition of Done per modul** — checklist singkat sebelum modul dianggap kelar: lint pass, ada test, config sudah divalidasi, endpoint sudah didokumentasi (godoc/comment).
3. **Validasi input di edge** — semua request dari frontend divalidasi di handler (format, size file upload CDR/AI/PDF max berapa MB, dll) sebelum masuk ke service, jangan percaya data dari client.
4. **Structured logging** — pakai `zerolog`/`zap` dengan request ID & context, bukan `fmt.Println`. Ini krusial buat modul `notification`/`payment` biar gampang lacak kenapa WA gagal terkirim atau kenapa order stuck di status tertentu.
5. **Context propagation** — semua fungsi service/repository terima `context.Context` sebagai parameter pertama, buat timeout/cancel query DB yang macet.
6. **Rate limiting di endpoint publik** — khususnya `/lacak/:resi` (no-login) biar tidak jadi celah brute-force nebak resi.

## 24. Status Keputusan
Tidak ada lagi item TBD terbuka. Seluruh keputusan arsitektur & fitur (section 1-23) sudah final per tanggal dokumen ini disepakati. Spec ini siap dijadikan acuan penuh untuk mulai development.

> **Addendum (2026-08-24):** Section 28 (Modul Diskon & Rekap Order) ditambahkan. Aturan snapshot di §28.2 **binding** — setiap perhitungan uang yang menyentuh diskon wajib membaca kolom snapshot di `orders`, bukan JOIN ke tabel `discounts`.
>
> **Addendum (2026-08-12):** Section 26 (Design Language) ditambahkan setelah spec awal. Semua UI baru **wajib** patuh; UI yang sudah terlanjur dibuat pakai palet Tailwind default (rose/slate + emoji) di-mark sebagai drift dan refactor bertahap.

## 25. Checklist Sebelum Mulai Development

**Blocking (harus kelar dulu)**
- [ ] VPS Hostinger aktif, domain dibeli, DNS mengarah ke VPS
- [ ] PostgreSQL bisa di-provision di VPS
- [ ] `.env` template awal dibuat (`APP_BASE_URL`, kredensial DB, OAuth) — section 2

**Aset & konten (siapkan paralel, tapi jangan nunggu sampai development jalan)**
- [ ] Video/footage pendek untuk hero (bahan dasar frame sequence, section 16)
- [ ] Logo, palet warna, font resmi
- [ ] Data katalog produk asli (bahan, ukuran, harga per m²/paket)
- [ ] 2-3 draft artikel SEO awal
- [ ] Nomor rekening & gambar QRIS statis

**Akun & kredensial**
- [ ] Google OAuth Client ID/Secret
- [ ] Nomor WA khusus untuk Baileys (terpisah dari WA toko yang dipakai manual)

**Data operasional**
- [ ] Daftar staff awal & role masing-masing
- [ ] Konfirmasi kebijakan retensi file desain 30 hari ke tim

**Dev environment**
- [ ] Langganan Claude Code aktif
- [ ] Repo Git dibuat
- [ ] File spec ini ditaruh sebagai `CLAUDE.md` di root repo
- [ ] Skill `go-modular-conventions` & `order-state-machine-ref` dibuat duluan (dipakai sejak modul pertama)

## 26. Design Language — Premium Minimalist Profesional

Aplikasi ini **wajib** tampil premium, minimalis, dan profesional — bukan playful, bukan warna-warni, bukan startup-style. Target kesan: seperti merek B2B/luxury yang matang (Linear, Vercel, Stripe premium tier, Apple product page), bukan e-commerce massal. Panduan ini binding untuk **semua UI baru** — kalau ada tekanan waktu, lebih baik komponen belum sempurna DP daripada kompromi tone yang bikin brand terlihat murah.

### 26.1. Palet Warna — Diambil dari logo (bukan hex logo mentah)

Logo Rajaku Printing pakai merah crimson + emas mahkota + hitam ribbon di atas latar putih. Diserap ke sistem warna UI premium (bukan diaplikasi mentah — warna logo terlalu saturated untuk UI besar):

| Token | Hex | Peran | Contoh pemakaian |
|---|---|---|---|
| `brand.500` (primary) | `#8B1A1A` | Merah crimson deep — CTA utama, active state, brand accent | Tombol "Order Banner", link hover, section highlight |
| `brand.600` | `#6E1414` | Hover/pressed dari primary | Button:hover, `focus:ring-brand-600/40` |
| `brand.50/100` | `#FAF3F3` / `#F5E5E5` | Wash background halus | Alert error subtle, empty state accent |
| `gold.500` (accent) | `#B08D57` | Emas antique — aksen premium/badge VIP/divider ornamental | Badge "Premium", separator hero, ikon rating |
| `gold.400/600` | `#C9A44A` / `#8F7042` | Emas terang/gelap untuk hover atau tulisan on-dark | Text on dark hero |
| `ink.950` | `#0A0A0A` | Foreground utama (heading, body strong) | `<h1>`, `<strong>` |
| `ink.900` | `#171717` | Body default | Paragraf |
| `ink.500` | `#78716C` | Muted / helper text (warm gray) | Label, subtitle, timestamp |
| `ink.200` | `#E7E5E4` | Hairline border | Divider, table row separator, `border-hairline` alias |
| `canvas.DEFAULT` | `#FAFAF9` | Background utama (off-white warm — bukan `#FFFFFF` klinis) | `<body>`, main panel |
| `canvas.alt` | `#F5F5F4` | Section alt / sidebar background | Sidebar admin, striped section |

**Tegas dilarang** — tidak boleh muncul di UI baru:
- `bg-rose-*`, `text-rose-*` (kecuali `rose-50` untuk error wash — dan sebaiknya pakai `brand.50`). Rose kesan playful/feminin, bukan tone printing shop premium.
- `bg-red-500` / `#EF4444` / warna merah saturated apapun. Pakai `brand.500` yang lebih deep.
- `bg-slate-*` — palet slate ada tint biru, tabrakan sama tone hangat brand kita. Pakai `ink.*` (base warm gray).
- Gradient warna-warni (`from-purple-500 to-pink-500` dll). Kalau perlu gradient: subtle monochrome `from-ink-900 to-ink-800`, atau ornamen aksen `from-gold-400 to-gold-600`.
- Warna emerald/blue/purple untuk decorative — pakai hanya untuk **semantic state** (success/info/warning). Untuk status badge success di modul admin: `bg-emerald-50 text-emerald-800` OK (semantic), tapi jangan pakai emerald sebagai brand accent.

### 26.2. Ikonografi — NO EMOJI di UI production

Emoji (📦 💳 🎨 dst) **dilarang** muncul di UI aplikasi (sidebar, dashboard card, badge). Emoji terlihat playful, inkonsisten lintas OS/font, tidak scale rapi, dan langsung menghancurkan kesan premium.

**Ganti dengan** salah satu:
1. **Lucide** (`lucide-vue-next`) — icon library monoline, stroke 1.5-2px, geometric — cocok untuk premium minimalist. Preferensi utama.
2. **Heroicons outline** — mirip, dari Tailwind team.
3. **Custom SVG inline** — untuk logo, ilustrasi bespoke. Selalu monoline atau flat 2-tone (brand + ink).

Aturan pakai icon:
- Ukuran seragam per konteks (16px sidebar, 20px button, 24px card hero). Jangan campur ukuran dalam satu group.
- Stroke width konsisten (default 1.5 Lucide). Jangan campur stroke thin & thick.
- Warna sesuaikan tone container — di sidebar dark: `text-ink-300`; di card putih: `text-ink-600`; icon active: `text-brand-500`.
- Padding dari label: `gap-2` (8px) default, `gap-3` untuk item besar.

Kalau memang butuh placeholder cepat & belum sempat install icon library, pakai simple monoline SVG inline (path 1-2 line) — **bukan** emoji.

### 26.3. Tipografi — Trio Wajib (Fraunces + Inter + JetBrains Mono)

Aplikasi ini pakai **tiga font** yang sudah di-load di `nuxt.config.ts` (self-hosted via `@nuxtjs/google-fonts`, tidak ada runtime hit ke Google). **Dilarang** pakai font lain — konsistensi tipografi = 60% dari kesan premium.

**1. Fraunces (serif display)** — `font-serif`
Optical-size axis variable font. Kesan editorial regal, sesuai vibe raja di logo. Dipakai untuk:
- Semua `<h1>` page + hero headline (mis. "Dashboard", "Verifikasi Pembayaran", judul landing)
- Judul modal besar & section eyebrow display (mis. quote / testimonial di landing)
- Logo wordmark "Rajaku" di sidebar / navbar (kombinasi dengan aksen gold subtle)

Aturan weight & style Fraunces:
- Display XL (hero landing): `text-5xl md:text-7xl font-serif font-semibold tracking-tight` (weight 600, tracking `-0.02em`)
- Page title admin: `text-2xl md:text-3xl font-serif font-semibold tracking-tight` (weight 600)
- Section title editorial: `text-xl md:text-2xl font-serif font-medium` (weight 500)
- **Jangan** pakai Fraunces untuk button, badge, label, atau body — kelihatan aneh & anti-legibility.

**2. Inter (sans-serif UI + body)** — `font-sans` (default `<html>` font)
Sudah default untuk semua elemen. Dipakai:
- Body paragraph, deskripsi, subtitle, helper text
- Button label, badge, nav item
- Form label & input value
- Card content (kecuali judul yang serif)

Aturan weight Inter:
- Body: 400 (default)
- Label & nav item: 500
- Button primary & judul card kecil: 600
- Section eyebrow (uppercase kecil): 500 + `tracking-[0.14em] uppercase`
- **Kontras weight** minimal 2 step antara headline & body (mis. serif 600 vs body 400) — ini yang bikin editorial feel.

**3. JetBrains Mono (data teknis)** — `font-mono`
Hanya untuk **data non-prosa** yang butuh presisi karakter:
- ID / UUID (dipendekkan `deb76a6e…`)
- Slug (`/artikel/5-tips-pilih-bahan-...`)
- Kode resi (`RJK-8F3K2A9X`)
- Nomor rekening, kode voucher
- Snippet code di dokumentasi/artikel

Selalu kecil: `text-xs` atau `text-[10px]`, warna `text-ink-500` atau `text-ink-700`. **Jangan** untuk body/label/button.

**Skala tipografi resmi** (mobile → desktop responsive):

| Level | Class | Font | Kapan pakai |
|---|---|---|---|
| Display XL | `text-5xl md:text-7xl font-serif font-semibold tracking-tight leading-[1.05]` | Fraunces 600 | Hero landing |
| Display | `text-4xl md:text-5xl font-serif font-semibold tracking-tight` | Fraunces 600 | Hero admin |
| H1 page | `text-2xl md:text-3xl font-serif font-semibold tracking-tight` | Fraunces 600 | Judul halaman admin |
| H2 section | `text-lg md:text-xl font-sans font-semibold` | Inter 600 | Sub-judul dalam page |
| H3 card | `text-sm font-sans font-semibold` | Inter 600 | Judul card |
| Body large | `text-base font-sans font-normal leading-relaxed` | Inter 400 | Paragraf landing/artikel |
| Body | `text-sm font-sans font-normal leading-relaxed` | Inter 400 | Body default UI |
| Small | `text-xs font-sans` | Inter 400/500 | Helper, timestamp, meta |
| Eyebrow | `text-[10px] font-sans font-medium uppercase tracking-[0.14em] text-ink-500` | Inter 500 | Section header sidebar/nav |
| Mono | `text-xs font-mono text-ink-500` | JBM 400 | ID, slug, code |

**Line height** — body `leading-relaxed` (1.625), headline `leading-tight` (1.2) atau `leading-[1.05]` untuk display XL.

**Font loading state** — `display: swap` sudah di-set. Selama font Fraunces belum siap, fallback ke Georgia serif (visual similar). Jangan pakai `font-serif` untuk konten kritis di above-the-fold hero mobile — bisa CLS shift.

### 26.4. Spacing & Layout

Premium = generous whitespace. Default spacing Tailwind sering terlalu padat.

> **Revisi 22 Agustus 2026 (keputusan pemilik proyek):** angka lama (`py-16 md:py-24`)
> membuat landing page terasa "panjang tapi kosong" di HP — jarak antar section
> lebih menonjol daripada isinya. Skala di bawah sudah dipadatkan satu tingkat.
> Jangan dikembalikan ke angka lama tanpa keputusan baru.

- Section vertical padding: `py-12 md:py-20`. Hero: satu layar penuh, lihat §18.
- Jarak judul section → isinya: `mt-8` (bukan `mt-10`).
- Card padding: `p-6 md:p-8` (bukan `p-4`).
- Max content width text-heavy: `max-w-2xl` (untuk artikel), `max-w-6xl` (untuk layout multi-column). Jangan biarkan text panjang sampai tepi viewport.
- Grid gap: `gap-6` default, `gap-8-12` untuk section utama.

### 26.5. Border, Radius, Shadow

- **Border** hairline saja: `border border-hairline` (= `#E7E5E4`), lebih halus dari `border-slate-300`. Alternatif `ring-1 ring-black/5` untuk elevated card.
- **Radius**: `rounded-md` (6px) untuk button/input, `rounded-lg` (8px) untuk card. **Hindari** `rounded-full` di button rectangular (kesan playful) — kecuali badge, avatar, pill status.
- **Shadow** minimal:
  - Default state: `shadow-none` atau `shadow-sm`.
  - Elevated (modal, dropdown): `shadow-lg` atau `shadow-xl`, tapi kombinasi dengan ring `ring-1 ring-black/5`. **Tidak** `shadow-2xl` (gaudy).
  - Jangan pakai colored shadow (`shadow-brand-500/50`) — kesan konsumer.

### 26.6. Interaktif & Motion

- **Transisi** 150-250ms `ease-out` (bukan `ease-in-out` yang lambat, bukan spring bouncy untuk elemen serious).
- **Hover state**: opacity 90% atau background shift 1 shade darker (mis. `hover:bg-brand-600` dari `bg-brand-500`). Jangan scale/rotate/shake elemen bisnis.
- **Focus ring**: `focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas`. Wajib untuk aksesibilitas.
- **Loading state**: spinner monoline halus, warna `ink.400`. Jangan pakai emoji spinner (⏳) atau color loop rainbow.
- **Micro-interaction** (motion-v) — hanya untuk transisi hal yg meaningful (page transition, revealed content). Jangan animasi setiap card hover — kesan noisy.

### 26.7. Komponen — Anchor pattern

Baseline yang harus konsisten:

- **Primary button**: `bg-brand-500 text-canvas hover:bg-brand-600 rounded-md px-4 py-2 text-sm font-semibold`. Radius `md`, bukan full.
- **Secondary button**: `border border-hairline bg-canvas hover:bg-canvas-alt text-ink-900 rounded-md`.
- **Destructive**: `bg-brand-500` (bukan `bg-rose-600`). Confirm modal wajib pakai reason input kalau efek non-trivial.
- **Card**: `bg-canvas border border-hairline rounded-lg p-6 shadow-none`. Hover elevate hanya kalau klikable: `hover:shadow-sm hover:border-ink-300`.
- **Input**: `border-hairline bg-canvas rounded-md focus:border-brand-500 focus:ring-brand-500/20`.
- **Badge**: `rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset`. Warna semantic (green success, amber pending, brand active).

### 26.8. Ilustrasi & Foto

- Foto produk / workshop wajib **desaturasi ringan** (~85% saturation) atau warm-toned filter — menghindari overly commercial look.
- Kalau butuh ilustrasi (mis. hero, error state): pakai monoline vector dua-tone (brand + ink) atau flat editorial style. **Bukan** kartun berwarna-warni (yg kebetulan mirip mascot logo — mascot boleh muncul cuma di footer/tentang-kami, bukan di UI operasional).
- Mascot Rajaku King (dari logo) **jangan** dipakai di admin panel atau checkout flow — konteks profesional, mascot mengganggu. Cukup di landing hero / about page.

### 26.9. Token Naming di Tailwind

`tailwind.config.ts` sudah expose brand tokens:
- `bg-brand-500 text-brand-500 border-brand-500` (crimson primary)
- `bg-gold-500 text-gold-500` (emas aksen)
- `text-ink-500 border-ink-200` (foreground/border warm gray)
- `bg-canvas bg-canvas-alt` (background)
- `border-hairline` (alias `border-ink-200`)

**Kalau pakai `text-slate-*`, `bg-rose-*`, `text-red-*` di file baru → salah.** Reviewer boleh reject.

### 26.10. Drift & migrasi

UI yang sudah dibuat sebelum panduan ini (admin panel foundation, CMS artikel, verifikasi pembayaran) pakai:
- Emoji di sidebar (`useAdminNav.ts`)
- Palet `rose-*` + `slate-*` di banyak class Tailwind
- Beberapa `focus:ring-rose-500/30` di BaseInput

Ini **drift** dan harus di-refactor bertahap. Prioritas migrasi:
1. `tailwind.config.ts` — sudah setup token (baseline enforceable).
2. `useAdminNav.ts` — ganti emoji dengan Lucide icon.
3. Layout `admin.vue` + components `admin/*` — replace `rose/slate` → `brand/ink/hairline`.
4. Halaman CMS artikel + pembayaran — replace class demi class.

Refactor drift bukan blocking untuk fitur baru — fitur baru **wajib** langsung compliant. Drift lama bisa dikerjakan sebagai task cleanup terpisah.

### 26.12. Ritme terang–gelap & token on-dark (22 Agustus 2026)

Landing page **tidak boleh** jadi satu blok putih panjang. Section gelap (`bg-ink-950`) disebar sebagai jeda: hero → (terang) → galeri proses **gelap** → (terang) → kenapa-rajaku **gelap** → (terang) → penutup **gelap** → footer **gelap**. Footer menempel langsung ke section penutup, tanpa jarak terang di antaranya.

Menggelapkan section berarti **membalik seluruh tokennya**, bukan hanya latarnya:

| Peran | Latar terang | Latar gelap |
|---|---|---|
| Latar | `bg-canvas` | `bg-ink-950` |
| Judul | `text-ink-950` | `text-canvas` |
| Body | `text-ink-500` | `text-canvas/70` |
| Eyebrow | `text-ink-500` | `text-canvas/60` |
| Garis | `border-hairline` | `border-white/10` |
| Permukaan kartu | `bg-canvas-alt` | `bg-white/5` |
| Aksen & ikon | `text-brand-500` | `text-gold-400` |
| Offset focus ring | `ring-offset-canvas` | `ring-offset-ink-950` |

Aksen **wajib** pindah ke emas di latar gelap — crimson `brand-500` di atas `ink-950` kontrasnya di bawah ambang keterbacaan. Tombol solid `bg-brand-500` tetap boleh crimson (kontras datang dari isian, bukan dari teks di atas hitam).

**Gerak berbasis scroll** (garis progres baca, hanyutan hero, hanyutan foto galeri) memakai CSS `animation-timeline`, bukan listener scroll di JS. Satu jebakan yang wajib diingat: `view()` mengukur terhadap wadah scroll **terdekat**, dan `overflow-hidden`/`overflow-x-auto` sudah dihitung sebagai wadah scroll — dipasang di dalamnya animasinya diam total tanpa error. Solusinya `view-timeline-name` di elemen luar yang diukur terhadap dokumen, lalu dirujuk dari dalam. Semua dibungkus `@supports` + `prefers-reduced-motion`, dan keadaan diamnya wajib aman (garis progres mulai `scaleX(0)`).

Detail lengkap + contoh hidup: `/admin/design-system` section **Section gelap**, **Sorotan kursor**, dan **Gerak scroll**.

### 26.11. Referensi Interaktif — Living Design System

Aturan tekstual di §26 di atas **dilengkapi** halaman interaktif:

**`/admin/design-system`** — Nuxt page yang render semua token & pattern langsung dari `tailwind.config.ts`. Ini **sumber kebenaran visual** — kalau ragu warna/spacing/pattern, buka page ini dulu sebelum coding.

Isi halaman:
- **Palet** — swatch tiap token (`brand.*`, `gold.*`, `ink.*`, `canvas.*`) + hex + Tailwind classname. Klik swatch untuk copy classname.
- **Tipografi** — live preview semua level skala Fraunces + Inter + JetBrains Mono.
- **Buttons** — primary, secondary, ghost, destructive, dengan state default/hover/focus/disabled.
- **Inputs** — text, textarea, select, checkbox dengan state error/disabled/focus.
- **Badges** — status pill (draft, published, pending, approved, rejected) dengan tone semantic.
- **Cards** — anchor pattern default + hover elevated + interactive link.
- **Motion** — demo focus-ring, hover transition, spinner.

**Update flow:** kalau menambah pattern baru (mis. tab component, tooltip), wajib tambah section di halaman ini dalam PR yang sama. Kalau design-system page dan implementasi drift → design-system yang benar; implementasi refactor.

Halaman dilindungi middleware `staff-only` (internal reference, tidak dibocorkan ke pengunjung). Link ada di sidebar admin grup **Kelola** (item "Design System").


## 27. Subagent — Delegasi Otomatis (Wajib)

Project ini punya subagent di `.claude/agents/`. **Delegasikan otomatis** — jangan tunggu user mengetik `@nama-agent`. Aturan ini override default "jangan panggil Agent tool kecuali diminta": user sudah memberi izin berdiri untuk project ini (14 Agustus 2026). Tujuannya hemat token — kerja detail terjadi di context subagent, main thread cuma terima ringkasannya.

| Agent | Model | Kapan dipanggil (otomatis) |
|---|---|---|
| `go-module-builder` | Sonnet 5 | Semua implementasi backend Go: modul/endpoint/service/repository/migration baru atau diubah |
| `nuxt-ui-builder` | Sonnet 5 | Semua pekerjaan UI frontend: halaman/komponen Nuxt-Vue-Tailwind |
| `code-reviewer` | Opus 5 | Setelah kode Go selesai ditulis/diubah, sebelum modul dianggap kelar atau di-commit. Read-only |
| `design-drift-auditor` | Haiku 4.5 | Setelah file `.vue`/komponen diubah, sebelum commit UI. Scan mekanis §26 |

**Pembagian model = strategi biaya**, jangan diubah tanpa alasan:
- **Opus 5** hanya untuk yang butuh nalar dalam (review korektnes, keputusan arsitektur).
- **Sonnet 5** untuk implementasi rutin yang aturannya sudah tertulis jelas di spec ini.
- **Haiku 4.5** untuk kerja mekanis berbasis grep/scan yang tidak butuh penalaran.

**Pola kerja standar per fitur:**
1. Backend → `go-module-builder` implement → `code-reviewer` gate → main thread fix temuan (atau lempar balik ke builder).
2. Frontend → `nuxt-ui-builder` implement → `design-drift-auditor` scan → fix hit yang dilaporkan.
3. Fitur yang menyentuh backend + frontend: dua builder boleh jalan paralel (satu pesan, dua panggilan Agent) karena tidak saling bergantung — reviewer/auditor menyusul setelah keduanya selesai.

**Kapan TIDAK usah delegasi** (overhead spawn lebih mahal dari kerjanya sendiri): edit satu baris, jawab pertanyaan soal kode yang sudah ada di context, baca 1 file, jalankan 1 perintah git/make.

**Batas subagent:** subagent mulai dari context kosong — kirim brief yang berdiri sendiri (path file, nama modul, status yang terlibat, hasil yang diharapkan). Jangan asumsikan dia tahu isi percakapan sebelumnya. Subagent juga tidak bisa memanggil subagent lain.

## 28. Modul Diskon & Rekap Order (24 Agustus 2026)

Ditambahkan setelah spec awal. Keputusan pemilik proyek pada tanggal yang sama:
diskon **dikelola & dipakai internal** (admin bikin master diskon, kasir/admin
memilihnya saat membuat order) — **tidak ada** kolom "kode promo" di form order
publik; dan diskon **hanya memotong subtotal produk**, tidak menyentuh ongkir.

### 28.1. Bentuk diskon

Tabel master `discounts`. Satu baris = satu program diskon:

| Kolom | Isi |
|---|---|
| `code` | Handle pendek unik, huruf besar (`LEBARAN25`) — dipakai kasir untuk mencari cepat, **bukan** kode yang diketik pelanggan |
| `name` | Nama yang dibaca manusia & yang muncul di struk/invoice ("Promo Lebaran 25%") |
| `type` | `percent` \| `nominal` |
| `value_percent` | Diisi hanya kalau `type='percent'`. `NUMERIC(5,2)`, 0 < v <= 100 |
| `value_amount` | Diisi hanya kalau `type='nominal'`. Rupiah bulat, > 0 |
| `max_discount_amount` | Batas atas rupiah untuk tipe persen (mis. "20% maks Rp50.000"). NULL = tanpa batas |
| `min_subtotal` | Subtotal minimum agar diskon boleh dipakai. Default 0 |
| `starts_at` / `ends_at` | Masa berlaku. NULL = tanpa batas di sisi itu |
| `quota` | Maksimum berapa order boleh memakai diskon ini. NULL = tak terbatas |
| `channel_scope` | `all` \| `online` \| `pos` |
| `is_active` | Saklar manual admin, terpisah dari masa berlaku |
| `deleted_at` / `deleted_by` / `delete_reason` | Soft delete — lihat §28.2 |

Selain diskon dari master, kasir boleh memberi **diskon manual** (nominal +
alasan wajib) untuk kasus tawar-menawar di tempat. Diskon manual tersimpan
dengan `discount_id = NULL` dan `discount_type_snapshot = 'manual'`.

**CHECK constraint wajib** di migration: kombinasi `type` dengan kolom nilainya
harus konsisten (`percent` → `value_percent` NOT NULL & `value_amount` NULL, dan
sebaliknya). Jangan andalkan validasi service saja — DB yang jadi penjaga
terakhir.

### 28.2. Aturan inti — diskon di rekap TIDAK BOLEH hilang

Masalahnya: diskon punya masa berlaku dan bisa dinonaktifkan/dihapus admin.
Kalau order cuma menyimpan `discount_id`, begitu diskonnya dihapus, rekap
transaksi bulan lalu ikut rusak — angka diskonnya jadi kosong atau query-nya
error. Tiga lapis pencegahan, ketiganya wajib:

**1. Snapshot nilai ke baris order (bukan cuma foreign key).**
Saat order dibuat, salin nilai diskon yang berlaku *saat itu* ke kolom milik
`orders` sendiri:

```
discount_id              UUID NULL   -- referensi, hanya untuk telusur balik
discount_code_snapshot   VARCHAR(30) NULL
discount_name_snapshot   VARCHAR(150) NULL
discount_type_snapshot   VARCHAR(20) NULL   -- percent | nominal | manual
discount_value_snapshot  NUMERIC(12,2) NULL -- 25.00 (persen) atau 50000 (nominal)
discount_amount          BIGINT NOT NULL DEFAULT 0  -- rupiah yang BENAR-BENAR dipotong
discount_note            TEXT NULL          -- wajib diisi untuk diskon manual
```

`discount_amount` adalah angka yang dipakai semua perhitungan uang. Snapshot
lain hanya untuk menjelaskan angka itu ke manusia.

**2. Master diskon tidak pernah di-hard-delete.**
Tombol "Hapus" di admin = soft delete (`deleted_at` terisi), pola yang sama
dengan `orders` di migration `000025`. Barisnya tetap ada, jadi foreign key
`orders.discount_id` tidak pernah menggantung dan telusur balik ("order mana
saja yang pakai promo ini?") tetap jalan selamanya. Kalau nanti ada yang
tergoda menambah `ON DELETE CASCADE` di FK ini: jangan — itu persis jalan yang
membuat rekap hilang.

**3. Semua query uang membaca kolom snapshot, tidak pernah JOIN ke `discounts`.**
Rekap, invoice, struk, dan laporan rekonsiliasi mengambil
`orders.discount_amount` / `discount_name_snapshot`. `discounts` hanya
di-JOIN di layar **kelola diskon**, tidak di layar uang.

Konsekuensi yang harus disadari & memang diinginkan: kalau admin mengubah nilai
sebuah diskon dari 20% jadi 25%, order lama **tetap** tercatat 20% — karena yang
dicatat adalah apa yang benar-benar terjadi saat transaksi, bukan isi master
hari ini. Nonaktif/kadaluarsa/hapus hanya memblokir pemakaian **baru**.

### 28.3. Rumus total

Diskon memotong **subtotal produk saja**. Ongkir tidak pernah didiskon.

```
discount_amount = min(hitungan_diskon, subtotal)     -- dijepit, tidak boleh > subtotal
total           = subtotal - discount_amount + COALESCE(shipping_cost, 0)
```

Untuk `type='percent'`:
`hitungan = round(subtotal × value_percent / 100)`, lalu dipotong
`max_discount_amount` kalau ada. Pembulatan **HALF-UP ke rupiah**, konsisten
dengan `calcPerM2` di `catalog/service/pricing.go`.

`total` **tidak boleh negatif** — penjepitan `min(..., subtotal)` di atas sudah
menjaminnya, dan wajib ada CHECK `discount_amount >= 0` di DB.

Titik kode yang WAJIB ikut memakai rumus ini (kalau salah satu terlewat, total
jadi tidak konsisten antar jalur):
- `order/service/order_service.go` — `CreateOrder` (online) dan `CreatePOSOrder`
- `SetShippingCost` — `newTotal` sekarang `Subtotal - DiscountAmount + ongkir`
- `ConfirmPickupTotal`
- `order/service/admin_override.go` — edit data pesanan super admin

### 28.4. Validasi saat diskon dipakai

Dicek di `discount` service sebelum order dibuat; gagal salah satu = tolak
dengan sentinel error masing-masing (§22 — `errors.Is`, bukan bandingkan string):

- `ErrDiscountNotFound` — id tidak ada, atau `deleted_at` terisi
- `ErrDiscountInactive` — `is_active = false`
- `ErrDiscountNotStarted` / `ErrDiscountExpired` — di luar `starts_at`/`ends_at`
- `ErrDiscountChannelMismatch` — `channel_scope` tidak cocok channel order
- `ErrDiscountMinSubtotal` — subtotal di bawah `min_subtotal`
- `ErrDiscountQuotaExhausted` — kuota habis

**Pemakaian kuota dihitung, bukan disimpan.** Tidak ada kolom `used_count` yang
di-increment — jumlah pemakaian = `COUNT(*) FROM orders WHERE discount_id = ?
AND deleted_at IS NULL`. Alasannya: kolom counter berarti dual-write, dan
begitu ada satu jalur yang lupa meng-update-nya, angkanya melenceng diam-diam
dan tidak ada cara mendeteksinya. Menghitung dari `orders` membuat pesanan
sebagai satu-satunya sumber kebenaran.

Konsekuensi yang diterima sadar: dua kasir yang menekan "Buat Pesanan"
bersamaan pada diskon yang tersisa 1 kuota bisa sama-sama lolos, jadi kuota
terlampaui paling banyak sejumlah request yang benar-benar bersamaan. Untuk
volume satu toko ini tidak masalah, dan konsekuensinya (memberi diskon satu
kali lebih banyak) jauh lebih ringan daripada risiko counter yang melenceng.
Kalau suatu saat perlu ketat, kuncinya `SELECT ... FOR UPDATE` pada baris
`discounts` di dalam transaksi pembuatan order — bukan menambah counter.

### 28.5. Rekap Order (admin panel)

Halaman `/admin/rekap`, permission `report.view`. Laporan baca-saja di atas
tabel `orders` (`deleted_at IS NULL`), tinggal di modul `order` — lihat catatan
di §3.

Route-nya `GET /api/v1/admin/order-recap` (+ `/export` untuk CSV), **bukan**
sub-path `/admin/orders/...`. Alasannya: grup itu sudah punya `GET
/admin/orders/:resi`, dan menaruh path statis bersebelahan dengan wildcard di
level yang sama mengundang bentrok routing — pola grup terpisah ini sama dengan
`/admin/audit-log` yang sudah ada di `order/handler/routes.go`.

**Filter**: rentang tanggal (wajib, zona WIB seperti rekonsiliasi POS §11),
channel (`semua`/`online`/`pos`), status order, kasir/pembuat, dan diskon
(termasuk pilihan "hanya order berdiskon").

**Kartu ringkasan**: jumlah order, omzet kotor (Σ `subtotal`), total diskon
(Σ `discount_amount`), total ongkir, omzet bersih (Σ `total`), dan rata-rata
nilai order.

**Tabel per order**: tanggal, resi, pelanggan, produk, channel, status,
subtotal, **diskon (nama snapshot + nominal)**, ongkir, total, metode bayar.
Baris diskon menampilkan `discount_name_snapshot` — jadi tetap terbaca lengkap
walau diskonnya sudah dihapus dari master.

**Ekspor CSV**: `GET /admin/order-recap/export` dengan filter yang sama, kolom
sama dengan tabel. Header `Content-Disposition: attachment`. Angka rupiah
ditulis polos tanpa pemisah ribuan supaya langsung terbaca sebagai angka di
spreadsheet.

Rekap **tidak** menghitung ulang apa pun dari master diskon — semuanya
penjumlahan kolom di `orders` (§28.2 lapis 3).

**Batas rentang tanggal: maksimum 366 hari**, berlaku untuk ketiga endpoint
rekap. Ini bukan pembatasan hemat-hematan: ekspor CSV tanpa batas berarti satu
staff yang mengetik `from=2000-01-01&to=2099-12-31` menarik seluruh tabel order
ke memori sekaligus, dan di VPS Hostinger tunggal (§19, tanpa autoscale) itu
cukup untuk mematikan proses backend. Ekspornya juga wajib **ditulis
mengalir ke `c.Writer` per batch**, bukan dirakit jadi satu `[]byte` lalu
dikirim — kalau tidak, batas 366 hari cuma menggeser masalahnya, tidak
menghilangkannya.

**Isi dropdown filter**: `GET /api/v1/admin/order-recap/filters?from=&to=`
(permission `report.view`) mengembalikan daftar kasir & diskon yang
**benar-benar muncul** pada rentang tanggal itu, jadi dropdown-nya ikut berubah
saat rentangnya diubah. Daftar diskonnya dibaca dari kolom **snapshot** di
`orders`, bukan dari tabel `discounts` — konsekuensinya persis yang diinginkan
§28.2: diskon yang sudah dihapus dari master tetap bisa dipilih sebagai filter,
karena order yang memakainya masih ada. Order berdiskon manual dikelompokkan
jadi satu entri `id: null` berlabel "Diskon manual".

### 28.6. Permission baru

| Kode | Untuk apa | Default |
|---|---|---|
| `discount.manage` | Buat/ubah/nonaktifkan/hapus master diskon | `super_admin` |
| `discount.apply` | Memakai diskon (master atau manual) saat membuat order | `super_admin`, `cashier` |
| `report.view` | Buka halaman Rekap Order & ekspor CSV | `super_admin` |

Seperti permission lain (§10), ketiganya tetap bisa di-toggle ke role manapun
lewat "Kelola Role". Yang perlu disadari sebelum memberikan `discount.apply`
ke role lain: pemegangnya bisa memotong harga jual, termasuk lewat diskon
manual tanpa batas nominal — beri hanya ke role yang memang berwenang
menentukan harga.

### 28.7. Tampilan diskon di dokumen

Kalau `discount_amount > 0`, struk thermal (§12) dan invoice PDF wajib
menampilkan barisnya di antara subtotal dan ongkir:

```
Subtotal                 Rp 250.000
Diskon (Promo Lebaran)  -Rp  50.000
Ongkir                   Rp  15.000
TOTAL                    Rp 215.000
```

Kalau `discount_amount = 0`, barisnya **tidak** ditampilkan sama sekali —
jangan cetak "Diskon Rp 0". Nama diskon diambil dari
`discount_name_snapshot`; untuk diskon manual pakai teks "Diskon" saja
(nama snapshot-nya kosong).

### 28.8. UI (§26 berlaku penuh)

- `/admin/diskon` — daftar + form buat/ubah, badge status (`Aktif`, `Terjadwal`,
  `Kadaluarsa`, `Nonaktif`, `Kuota habis`) memakai warna semantic §26.7, ikon
  Lucide `TicketPercent`. Hapus = modal konfirmasi dengan input alasan (§26.7
  "destructive"), dan modalnya wajib menyebut bahwa order lama tetap mencatat
  diskon ini.
- `/admin/rekap` — ikon Lucide `ClipboardList`, grup sidebar `kelola`.
- Form POS (`/admin/pos`) — pemilih diskon + ringkasan harga yang menunjukkan
  potongan sebelum kasir menekan "Buat Pesanan".

### 28.9. Cakupan diskon per produk (24 Agustus 2026)

Awalnya diskon berlaku global — disaring hanya oleh `channel_scope`,
`min_subtotal`, dan masa berlaku. Ditambahkan kemampuan membatasi sebuah
diskon hanya untuk produk tertentu.

Yang membuat ini murah: **satu order = satu produk** di sistem ini. `orders`
menyimpan satu `product_id`, satu bahan, satu ukuran, satu `subtotal` — tidak
ada tabel item baris. Jadi "diskon per produk" tidak menuntut pembongkaran
struktur order, cukup penyaringan saat diskon dipakai.

**Struktur**
- `discounts.applies_to` — `all` (default) atau `selected`. Default `all`
  membuat semua diskon yang sudah ada berperilaku persis seperti sebelumnya.
- Tabel `discount_products` (`discount_id`, `product_id`, unique berpasangan,
  FK ke `products`). Produk **tidak pernah di-hard-delete** di modul catalog
  (hanya `is_active=false`), jadi FK ini aman dan relasinya tidak perlu ikut
  soft-delete.

**Validasi** — sentinel `ErrDiscountProductMismatch` di `validateForUse`:
kalau `applies_to='selected'` dan `product_id` order tidak ada di daftar,
tolak.

**Aturan yang tidak boleh dilanggar:** diskon `applies_to='selected'` yang
daftar produknya kosong (atau seluruh produknya sudah nonaktif) **tidak boleh
diperlakukan sebagai berlaku-untuk-semua**. Daftar kosong = tidak ada order
yang cocok = diskon tidak bisa dipakai. Menganggap "kosong berarti semua"
adalah kegagalan senyap yang memberi potongan ke seluruh katalog, dan itu
kerugian uang nyata. Jalur create/update wajib menolak `applies_to='selected'`
tanpa satu pun produk, dan `validateForUse` wajib menolak lagi saat dipakai —
dua-duanya, karena produk bisa dinonaktifkan setelah diskon dibuat.

**Endpoint `/applicable`** — tambah parameter `product_id`, supaya daftar
diskon yang muncul di layar kasir sudah tersaring sejak awal dan kasir tidak
pernah melihat promo yang akan ditolak saat disimpan.

**Yang TIDAK berubah:** aturan snapshot §28.2. `discount_amount` tetap
disalin ke order saat transaksi. Cakupan produk hanya menentukan **boleh atau
tidaknya sebuah diskon dipakai**, bukan cara pencatatannya — jadi jaminan
"diskon tetap tercatat di rekap walau sudah dihapus" berdiri tanpa disentuh.

Cakupan per **bahan** (`material_id`) sengaja belum dibuat — kalau nanti
dibutuhkan, polanya sama persis (`discount_materials` + satu sentinel lagi).
