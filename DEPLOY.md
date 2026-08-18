# Runbook Deploy — Rajaku Printing

Panduan langkah demi langkah untuk menaikkan sistem ke VPS produksi. Kerjakan
berurutan; tiap bagian punya cara memastikan langkahnya berhasil sebelum lanjut.

**Prasyarat:** VPS sudah punya PostgreSQL, Go, Node.js 20+, dan domain sudah
mengarah ke server.

---

## Bagian 1 — Google Cloud Console

Ini tidak bisa diotomasi, harus diklik sendiri.

### 1.1 Tambah redirect URI produksi

1. Buka **APIs & Services → Credentials**.
2. Klik OAuth 2.0 Client ID yang dipakai proyek ini.
3. Di **Authorized redirect URIs**, klik **ADD URI**, isi:

   ```
   https://DOMAIN-ANDA/api/v1/auth/google/callback
   ```

4. **Jangan hapus** URI localhost yang sudah ada — satu client boleh punya
   banyak URI, jadi lingkungan dev tetap bisa dipakai.
5. **Save**.

> URI harus **persis sama** dengan `GOOGLE_OAUTH_REDIRECT_URL` di `.env`
> server. Beda `http`/`https`, atau beda garis miring di akhir, akan ditolak
> Google dengan pesan `redirect_uri_mismatch`.

### 1.2 Publish consent screen

1. Buka **APIs & Services → OAuth consent screen**.
2. Lihat **Publishing status**. Kalau tertulis **Testing**, klik
   **PUBLISH APP** lalu konfirmasi.

> Selama status masih "Testing", **hanya akun yang terdaftar di daftar test
> user** yang bisa login. Pelanggan biasa ditolak Google. Ini penyebab paling
> umum keluhan "login Google tidak jalan" setelah deploy.

---

## Bagian 2 — Backend

### 2.1 Siapkan `.env`

```bash
cd backend && cp .env.example .env && nano .env
```

Yang wajib diubah:

| Variabel | Isi |
|---|---|
| `APP_BASE_URL` | `https://DOMAIN-ANDA` (alamat API) |
| `APP_FRONTEND_URL` | alamat Nuxt; kosongkan kalau satu domain |
| `GOOGLE_OAUTH_CLIENT_ID` | dari Console |
| `GOOGLE_OAUTH_CLIENT_SECRET` | dari Console |
| `GOOGLE_OAUTH_REDIRECT_URL` | sama persis dengan langkah 1.1 |
| `NOTIFICATION_WORKER_SECRET` | string acak minimal 32 karakter |
| `DB_*` | kredensial PostgreSQL server |

Membuat secret acak:

```bash
openssl rand -hex 32
```

Simpan hasilnya — dipakai lagi di Bagian 4.

> Config divalidasi saat startup. Kalau ada yang salah, server **menolak
> jalan** dan menyebutkan variabel mana yang bermasalah. Itu perilaku yang
> benar, bukan kerusakan.

### 2.2 Jalankan migration

```bash
make migrate-up
```

Migration baru di rilis ini:

- `000016` — menghapus kode OTP lama yang terlanjur tersimpan sebagai teks
  biasa. **Tidak bisa dibatalkan** (datanya memang sengaja dihapus).
- `000017` — nomor pelanggan jadi opsional.
- `000018` — tabel gambar landing page + izin `sitemedia.manage`.

### 2.3 Jalankan server

```bash
make build && ./bin/api
```

Cek berhasil:

```bash
curl https://DOMAIN-ANDA/api/v1/ping
```

Harus membalas `{"success":true,"data":{"pong":true}}`.

Untuk produksi, jalankan sebagai systemd service atau `pm2` supaya otomatis
hidup lagi setelah server restart.

### 2.4 Buat akun super admin

```bash
make seed-admin email=anda@domain.com phone=62812xxxx name="Nama Anda" password="RahasiaKuat"
```

Aman dijalankan ulang (idempoten).

---

## Bagian 3 — Frontend

```bash
cd frontend && cp .env.example .env && nano .env
```

Isi:

```
NUXT_PUBLIC_API_BASE=https://DOMAIN-ANDA/api/v1
NUXT_PUBLIC_APP_BASE_URL=https://DOMAIN-ANDA
```

Lalu:

```bash
npm ci && npm run build && node .output/server/index.mjs
```

Jalankan juga di bawah systemd/pm2 untuk produksi.

---

## Bagian 4 — Mengaktifkan notifikasi WhatsApp (Baileys)

Bagian ini paling sering membingungkan, jadi dijelaskan pelan-pelan.

### 4.1 Cara kerjanya

1. Backend menyimpan pesan ke tabel antrean `notification_jobs`.
2. Worker bertanya ke backend "ada kiriman?" tiap beberapa detik.
3. Worker mengirim lewat WhatsApp.
4. Worker melapor balik: terkirim atau gagal.

Backend **tidak pernah** bicara langsung ke WhatsApp — dia hanya menulis
antrean ke database. Konsekuensinya dua hal bagus:

- Kalau worker mati, pesanan **tetap tercatat**. Notifikasinya menumpuk di
  antrean dan terkirim begitu worker hidup lagi. Tidak ada yang hilang.
- Kasir tidak pernah menunggu WhatsApp saat menekan "Buat Pesanan".

### 4.2 Siapkan nomor WhatsApp

**Pakai nomor khusus, terpisah dari WA toko yang Anda pakai membalas
pelanggan secara manual.**

Alasannya serius: Baileys bukan API resmi WhatsApp, ia bekerja dengan meniru
WhatsApp Web. Kalau nomor ini sampai dibatasi atau diblokir WhatsApp, Anda
tidak ingin nomor utama toko ikut mati.

Siapkan satu nomor, pasang di HP, aktifkan WhatsApp seperti biasa.

### 4.3 Pasang worker

```bash
cd services/notification-worker && cp .env.example .env && nano .env
```

Yang wajib diisi:

| Variabel | Isi |
|---|---|
| `INTERNAL_SECRET` | **sama persis** dengan `NOTIFICATION_WORKER_SECRET` di `.env` backend |
| `BACKEND_URL` | `https://DOMAIN-ANDA` |

Lalu:

```bash
npm install && npm start
```

### 4.4 Pairing (scan QR) — hanya sekali

Setelah `npm start`, di terminal muncul **QR code berbentuk ASCII**.

1. Buka **WhatsApp di HP** dengan nomor khusus tadi.
2. Ketuk **titik tiga → Perangkat Tertaut**.
3. Ketuk **Tautkan Perangkat**.
4. Arahkan kamera ke QR di layar terminal.

Kalau QR di terminal susah dipindai, buka di browser:

```
http://IP-SERVER:9090/qr
```

**Berhasil kalau** di log muncul `WA connected`.

Sesi tersimpan di folder `session/` — **restart tidak perlu scan ulang**.

### 4.5 Hal penting yang harus dijaga

**Folder `session/` harus permanen.** Kalau terhapus, Anda harus scan QR ulang
dari awal.

**File `wa-state.json`** (di sebelah folder `session/`) menyimpan hitungan
kuota harian dan umur nomor. **Jangan dihapus** — kalau hilang, sistem
menganggap nomor Anda baru lagi dan membatasi kiriman ke 20 pesan per hari.

**Hari-hari pertama sengaja dibatasi:**

| Hari ke- | Batas kirim |
|---|---|
| 1 | 20 pesan |
| 2 | 40 pesan |
| 3 | 80 pesan |
| 4 | 150 pesan |
| 5 | 250 pesan |
| 6 dan seterusnya | 500 pesan |

Ini bukan kerusakan. Nomor baru yang langsung mengirim ratusan pesan adalah
**penyebab pemblokiran paling umum**. Kalau nomor yang Anda pakai sudah lama
aktif dan wajar, batas ini boleh dinaikkan lewat `WA_WARMUP_RAMP` di `.env`
worker — risiko ditanggung sendiri.

Perlindungan lain yang sudah aktif otomatis: jeda acak antar pesan, istirahat
berkala, maksimal 5 pesan per hari ke satu nomor yang sama, pengecekan apakah
nomor tujuan benar-benar terdaftar di WhatsApp sebelum mengirim, dan
penghentian otomatis kalau WhatsApp mulai menolak.

### 4.6 Memantau

```
http://IP-SERVER:9090/healthz
```

Menampilkan status koneksi, sisa jatah kirim hari ini, dan apakah pengiriman
sedang dihentikan sementara.

### 4.7 Jalankan permanen

```bash
npm install -g pm2
```

```bash
pm2 start src/index.js --name rajaku-wa && pm2 save && pm2 startup
```

### 4.8 Kalau bermasalah

| Gejala | Penyebab dan solusi |
|---|---|
| Log `WA belum siap` | Belum scan QR, atau koneksi putus. Buka `/qr`. |
| Log `INTERNAL_SECRET must be >= 32 chars` | Secret kurang panjang. |
| Log `HTTP 401` dari backend | Secret worker berbeda dengan backend. Samakan. |
| Tidak ada pesan terkirim sama sekali | Cek `/healthz` — mungkin jatah harian sudah habis. |
| Harus scan QR terus-menerus | Folder `session/` tidak permanen. |

---

## Bagian 5 — Isi konten lewat admin panel

Login sebagai super admin, lalu buka **Kelola → Media Landing Page**.

Unggah **Kode QRIS Pembayaran**. Unggah cetakan QRIS apa adanya; jangan
dipotong sampai mengenai pola kotak di sudut, karena kodenya jadi gagal
dipindai. Sebelum diunggah, pembeli hanya melihat pilihan transfer bank.

Slot gambar lain (hero, logo, foto proses) boleh diganti kapan saja tanpa
deploy ulang.

---

## Bagian 6 — Cek akhir

Kerjakan berurutan, semuanya harus lolos:

1. `curl https://DOMAIN-ANDA/api/v1/ping` membalas `success: true`.
2. Buka halaman utama — gambar tampil, tidak ada kotak kosong.
3. Login Google dari browser yang **belum pernah login** (bukan akun test) —
   tidak ada peringatan "unverified".
4. Daftar dengan **melewati** pengisian nomor → akun jadi, tanpa OTP.
5. Daftar dengan nomor baru → akun jadi, **tidak ada WhatsApp terkirim**.
6. Tambah nomor yang sudah dipakai pelanggan lain → muncul tawaran verifikasi
   dan OTP masuk ke WhatsApp. **Ini sekaligus membuktikan Baileys jalan.**
7. Buka halaman pesanan — pastikan tertulis
   **BCA — CV WANSHOU NIAGA UTAMA — 3245070777** dan QRIS tampil. Periksa
   dengan mata sendiri: satu angka keliru berarti uang pelanggan salah alamat.
8. Buka `/lacak/RESI` tanpa login — bisa dibuka.

---

## Diketahui, sengaja belum dikerjakan

- **Order lama milik guest tidak ikut pindah** saat nomornya diklaim lewat
  menu tambah-nomor di halaman akun; hanya kunci pencocokannya yang berpindah.
  Registrasi Google yang langsung menyerap guest tetap menggabungkan riwayat
  seperti biasa.
- **Halaman pairing WhatsApp di admin panel belum ada** — pairing masih lewat
  terminal atau `/qr`.
- **Rekening masih tertulis di kode** (`frontend/utils/payment.ts`);
  menggantinya berarti deploy ulang.
- Unggah gambar lewat admin belum pernah diuji ujung-ke-ujung dengan sesi
  staff sungguhan — Bagian 5 yang akan membuktikannya.
