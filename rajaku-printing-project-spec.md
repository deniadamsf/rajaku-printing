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
