# Rencana Pengembangan App Flutter (Customer & Member)

> Turunan dari **§21.1** di `CLAUDE.md` / `AGENTS.md`, ditulis 8 September 2026.
> §21.1 statusnya masih DRAFT dan melarang implementasi tanpa konfirmasi
> pemilik proyek. Dokumen ini **rencana kerjanya**, bukan izin untuk mulai —
> Fase 1 baru boleh dijalankan setelah pemilik proyek bilang mulai.

App ini untuk **customer & member** (akun, pesan banner, lacak, riwayat,
membership). Berbeda dari app Android di §21 yang untuk **operasional
cetak/produksi** — jangan digabung, penggunanya beda dan kebutuhannya beda.

---

## 0. Urutan fase & kenapa tidak boleh dibalik

| Fase | Isi | Estimasi |
|---|---|---|
| 0 | Prasyarat: uji §32, keputusan FCM, kredensial Google Android | 1–2 hari |
| 1 | **Backend**: refresh token + login Google native | 2–3 hari |
| 2 | Kontrak API untuk app (dokumentasi, bukan kode baru) | 0,5–1 hari |
| 3 | Kerangka app Flutter: auth, storage, HTTP client, navigasi | 3–5 hari |
| 4 | Alur order (katalog → quote → multi-item → upload → bayar) | 2–3 minggu |
| 5 | Fitur pendukung: riwayat, lacak, membership, profil | 1–1,5 minggu |
| 6 | Rilis: build, signing, Play Store | 3–7 hari + review toko |

**Total realistis 6–9 minggu** untuk satu orang mengerjakan penuh. Angka ini
untuk MVP yang layak dipakai pelanggan, bukan prototipe.

**Fase 1 wajib selesai sebelum Fase 3 dimulai.** Ini satu-satunya urutan yang
tidak boleh ditawar. Alasannya ditulis di §21.1: menambal auth di tengah
pengerjaan app berarti membongkar ulang seluruh lapisan login, penyimpanan
token, dan interceptor HTTP yang sudah terlanjur dipakai semua layar. Fase 1
juga berguna sendiri untuk web (logout yang benar-benar mencabut sesi), jadi
mengerjakannya duluan tidak ada ruginya walau app-nya nanti ditunda.

Fase 0 boleh jalan paralel dengan Fase 1, kecuali butir 0.1. Fase 4 dan 5
boleh tumpang tindih.

---

## Fase 0 — Prasyarat

### 0.1. Uji manual order multi-item (§32) lewat browser — BLOCKING

§32 sudah live di produksi tapi **belum pernah diuji manual lewat browser**.
Form order di app berdiri persis di atas endpoint yang sama, jadi bug di sana
akan muncul dua kali dan didiagnosis dua kali. Uji dulu di web:

- Buat order online 3 item beda produk, cek `Σ item.subtotal == orders.subtotal`.
- Pakai diskon `applies_to = selected` yang cuma cocok 1 item, cek potongannya
  hanya memotong item itu (§32.3), dan `Σ item.discount_amount ==
  orders.discount_amount`.
- Order campuran: 1 item bawa desain sendiri, 1 item minta dibuatkan, cek
  `orders.design_source` jadi `mixed` (§32.5).
- Cetak struk & invoice, cek satu baris per item dan diskon tetap satu baris
  agregat (§32.7).
- Edit item lewat koreksi super admin (§32.9), cek agregat dihitung ulang.

Kalau ada yang meleset, perbaiki di backend **sebelum** Fase 4.

### 0.2. Keputusan push notification — perlu jawaban, bukan asumsi

Sekarang notifikasi cuma lewat WA (§13). App tanpa push berarti pelanggan
tetap dapat kabar lewat WA — bisa diterima untuk MVP, tapi **harus diputuskan
sadar di depan**, bukan ditemukan di tengah jalan. Kalau nanti diputuskan
pakai FCM, itu menyentuh `notification_job`, butuh tabel token perangkat, dan
menambah kira-kira 1 minggu. Catat keputusannya di §21.1 saat diambil.

### 0.3. Kredensial Google untuk Android

Buat OAuth Client ID **Android** di Google Cloud Console (butuh SHA-1
fingerprint keystore). Client ID Android **berbeda** dari Client ID web yang
sudah dipakai — jangan dipakai ulang, karena klaim `aud` di `id_token` harus
cocok dengan client yang menerbitkannya (§21.1 B).

Catatan lama yang belum beres: `.env.docker` sempat ter-commit berisi
kredensial Google. Rotasi secret lama sekalian di sini.

### 0.4. Base URL

App membaca satu base URL saja, pola sama §2 — satu konstanta build-time
(`--dart-define=API_BASE_URL=...`), bukan nilai yang tersebar di banyak file
Dart. Sediakan nilai dev & produksi.

---

## Fase 1 — Backend: tutup dua gap auth

Kerja Go, tunduk penuh pada §22 (handler → service → repository, error
dibungkus konteks, sentinel error, minimal 1 test happy path + 1 test gagal).

### 1A. Refresh token (1,5–2 hari)

Sekarang belum ada sama sekali: `backend/internal/auth/token/jwt.go:7`
menulis "BELUM diimplementasikan", dan `auth_handler.go:130` masih TODO —
logout hanya membuang token di sisi client.

Migration baru `000035_refresh_tokens`:

```
refresh_tokens (
  id           UUID PK,
  user_id      UUID NOT NULL REFERENCES users(id),
  token_hash   TEXT NOT NULL UNIQUE,   -- HASH, bukan token mentah
  device_label TEXT NULL,
  issued_at    TIMESTAMPTZ NOT NULL,
  expires_at   TIMESTAMPTZ NOT NULL,
  revoked_at   TIMESTAMPTZ NULL,
  replaced_by  UUID NULL REFERENCES refresh_tokens(id)
)
```

Aturan yang tidak boleh dilanggar:

- **Token mentah tidak pernah masuk DB** — hanya hash-nya, pola sama dengan
  `password.go`.
- **Rotasi wajib**: setiap kali dipakai, token lama di-revoke dan diganti yang
  baru.
- **Token yang sudah revoked dipakai lagi = sinyal pencurian** → revoke
  **semua** refresh token milik user itu, bukan cuma yang dicoba. Menolak
  diam-diam tanpa revoke massal adalah kegagalan senyap yang dilarang §22.
- TTL 30–60 hari, dibaca dari env, fail-fast saat startup kalau kosong.
- `POST /api/v1/auth/refresh` wajib rate-limited (pola §23.6).
- `POST /api/v1/auth/logout` wajib mencabut token di server, bukan cuma
  menghapus di client. Ini sekalian menutup TODO yang sudah ada.

Batas yang tetap ada dan **wajib ditulis apa adanya**, jangan dijanjikan
lebih: access token tetap stateless (§31.4), jadi mencabut refresh token tidak
membunuh access token yang terlanjur terbit — sesi mati paling lama setelah
TTL access token habis.

### 1B. Login Google native (0,5 hari)

Yang ada sekarang cuma `/google/start` → `/callback` → `/exchange`, murni
navigasi browser. Google Sign-In di HP langsung memberi `id_token`.

- Endpoint baru `POST /api/v1/auth/google/native`, body `{ id_token }`.
- Verifikasi `id_token` pakai public key Google (`google.golang.org/api/idtoken`)
  — jangan pernah percaya isinya mentah dari client.
- Validasi klaim `aud` cocok `GOOGLE_OAUTH_ANDROID_CLIENT_ID` (config baru,
  fail-fast saat startup).
- **Dilarang menduplikasi logika bisnis**: setelah `id_token` terverifikasi,
  hasilnya diserahkan ke method service yang sama dengan flow web
  (`RequestOTP` / `Complete`) — termasuk **tetap wajib OTP WA** untuk user
  baru. Melewati OTP karena "datang dari app" membuka kembali celah keamanan
  yang sudah ditutup untuk web.
- Flow web tidak berubah sama sekali. Ini murni penambahan jalur baru.

### Definition of Done Fase 1

Lint & vet hijau; test happy-path + gagal untuk rotasi, deteksi reuse, dan
verifikasi `id_token`; config baru fail-fast; `code-reviewer` lolos; §21.1
diperbarui dari DRAFT jadi terlaksana.

---

## Fase 2 — Kontrak API untuk app

Tidak menulis endpoint baru. Yang dikerjakan: mendaftar endpoint yang dipakai
app beserta bentuk request/response-nya, supaya sisi Flutter tidak menebak.

Seluruh endpoint yang dibutuhkan **sudah ada**:

| Kebutuhan app | Endpoint |
|---|---|
| Daftar / login | `POST /auth/register`, `POST /auth/login` |
| Login Google | `POST /auth/google/native` *(Fase 1B)* |
| Perpanjang sesi | `POST /auth/refresh` *(Fase 1A)* |
| Profil sendiri | `GET /auth/me` |
| Katalog | `GET /products`, `GET /products/:slug`, `GET /materials` |
| Hitung harga | `POST /quote` |
| Buat pesanan | `POST /orders` |
| Riwayat sendiri | `GET /orders` |
| Detail pesanan | `GET /orders/:resi` |
| Lacak publik | `GET /lacak/:resi` |
| Upload desain | `POST /orders/:resi/design-files` |
| Setujui / revisi desain | `POST /design-drafts/:id/approve`, `/revision` |
| Upload bukti bayar | `POST /orders/:resi/payment-proof` |
| Info rekening / QRIS | `GET /settings/payment-info` |
| Invoice PDF | `GET /invoices/:id/pdf` |
| Membership | `GET /account/membership`, `POST /account/membership/apply` |
| Artikel | `GET /articles`, `GET /articles/:slug` |

Semua respons memakai envelope `{ success, data, error{code,message} }`
(§22) — bikin satu parser di Dart, jangan tiap layar mengurai sendiri.

---

## Fase 3 — Kerangka app

- Proyek Flutter, struktur per fitur (`lib/features/order/...`), bukan per
  jenis file — sejalan dengan modularitas §3.
- HTTP client satu pintu dengan interceptor: menempel `Authorization: Bearer`,
  dan saat kena 401 **otomatis menukar refresh token lalu mengulang request
  sekali**. Kalau refresh gagal, paksa login ulang. Ini satu-satunya tempat
  yang boleh tahu soal refresh; layar tidak boleh mengurusnya sendiri.
- **Refresh token disimpan di secure storage** (`flutter_secure_storage`,
  yaitu Keychain/Keystore), **bukan** `SharedPreferences` polos.
- Navigasi + guard rute untuk halaman yang butuh login.
- Tema ikut Design Language §26: crimson `#8B1A1A`, emas `#B08D57`, ink warm
  gray, canvas off-white; Fraunces untuk judul, Inter untuk body. **Tanpa
  emoji** di UI.

---

## Fase 4 — Alur order (bagian terberat)

Urutan layar: katalog → detail produk → penyusun item → keranjang → data
pengiriman → ringkasan → buat pesanan → upload desain → upload bukti bayar.

Yang membuat fase ini berat bukan jumlah layarnya, tapi **penyusun item
multi-baris** (§32): tiap baris punya produk, bahan, ukuran, jumlah, dan
sumber desain sendiri, maksimal 20 baris (§32.4).

### Aturan keras untuk sisi Dart

1. **Harga tidak boleh dihitung di app.** Selalu `POST /quote` ke backend.
   Menyalin rumus `per_m2` / `paket` (§9) ke Dart berarti dua tempat yang bisa
   melenceng diam-diam, dan yang melenceng itu uang.
2. **Diskon tidak boleh dihitung atau dialokasikan di app.** Basis hitung
   (`eligible_subtotal`) dan alokasi sisa-terbesar (§32.3) milik backend. App
   hanya menampilkan angka yang dikembalikan.
3. **Uang pakai `int` rupiah bulat, jangan `double`.** Rupiah tidak punya sen;
   `double` memperkenalkan galat pecahan yang tidak akan pernah cocok dengan
   `BIGINT` di Postgres.
4. **Nama status disalin persis dari §4.** Jangan mengarang varian
   (`waiting_payment`, `menunggu_bayar`) — bikin satu enum Dart yang isinya
   sama huruf per huruf dengan status backend.
5. **Nomor WA diformat `62xxx`** sejak input (§13), sama seperti web.
6. Upload file CDR/AI/PDF/JPG/PNG dengan batas ukuran yang sama dengan web,
   divalidasi di app **dan** tetap divalidasi backend (§23.3 — jangan percaya
   client).

---

## Fase 5 — Fitur pendukung

- Riwayat pesanan (`GET /orders`), detail, dan riwayat status.
- Lacak resi tanpa login — data sensitif sudah disensor backend (§5), app
  cukup menampilkan apa adanya.
- Membership: kartu status + tombol ajukan (§30.4). Sembunyikan seluruh
  bagian ini kalau `membership_enabled = false`.
- Profil, ubah data dasar, logout yang benar-benar mencabut token.
- Artikel SEO sebagai konten baca dalam app (opsional, murah karena
  endpoint-nya sudah ada).

---

## Fase 6 — Rilis

- Keystore rilis + SHA-1 didaftarkan ke Google Cloud Console (butir 0.3).
- Ikon, splash, nama app, versi.
- Kebijakan privasi (wajib Play Store — app mengumpulkan nama, nomor WA,
  email, dan file unggahan).
- Build App Bundle → internal testing → produksi.
- Sediakan waktu untuk review Play Store: biasanya beberapa hari, bisa lebih
  lama untuk akun developer baru.

---

## Yang sengaja TIDAK masuk MVP

- **Push notification (FCM)** — sampai butir 0.2 diputuskan.
- **iOS** — butuh akun Apple Developer berbayar dan mesin macOS. Kodenya
  Flutter jadi bisa menyusul, tapi jangan dijanjikan sekarang.
- **UI kelola perangkat login** — tabel `refresh_tokens` sudah menampungnya
  lewat `device_label`, tapi layarnya keputusan terpisah.
- **App operasional produksi** (§21) — pengguna dan kebutuhannya berbeda.
- **Pembayaran otomatis** — tetap manual (§7), app cuma mengunggah bukti.

---

## Catatan untuk agent yang mengerjakan ini

Kalau dikerjakan Gemini Antigravity: baca `AGENTS.md` lebih dulu, terutama §4
(nama status), §22 (aturan coding Go), §26 (design language), §28.2 (snapshot
diskon), dan §32 (order multi-item). Fase 1 seluruhnya kerja Go dan tunduk
pada §22 — bukan kode gaya bebas.

Pemilihan model per jenis kerja ada di catatan portabilitas §27 `AGENTS.md`.
Khusus **Fase 1**, review-nya wajib memakai model tier atas (Gemini 3.1 Pro
High atau Claude Opus 4.6) walau implementasinya memakai Flash — fase itu
menyentuh auth dan penyimpanan token, dan kesalahan di sana tidak terlihat
sampai ada yang memanfaatkannya.
