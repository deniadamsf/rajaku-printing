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

Cara utama: **lewat admin panel**, tidak perlu SSH.

1. Login sebagai super admin, buka menu **Pairing WhatsApp** (grup Kelola).
2. QR tampil di layar. Buka **WhatsApp di HP** dengan nomor khusus tadi.
3. Ketuk **titik tiga → Perangkat Tertaut → Tautkan Perangkat**.
4. Arahkan kamera ke QR di layar.

Halaman memuat ulang QR otomatis tiap 10 detik (QR WhatsApp cepat
kedaluwarsa), dan berubah jadi "WhatsApp tertaut" beserta nomornya begitu
berhasil.

> **Perlakukan QR seperti password.** Siapa pun yang memindainya menautkan
> WhatsApp miliknya ke nomor toko dan bisa mengirim pesan atas nama toko ke
> seluruh pelanggan. Jangan difoto, jangan share screen selagi QR tampil.

**Ganti nomor?** Di halaman yang sama ada tombol **Putuskan pairing**. Selama
terputus tidak ada notifikasi terkirim ke pelanggan, jadi lakukan saat sepi
dan langsung pindai QR baru yang muncul.

**Kalau admin panel tidak bisa diakses**, QR ASCII tetap tercetak di log
worker saat `npm start` — pakai itu sebagai cadangan.

Endpoint worker `:9090/qr` dan `:9090/pairing` **butuh header
`X-Internal-Secret`**, jadi tidak bisa lagi dibuka langsung dari browser.
Port 9090 sebaiknya tidak dibuka ke internet sama sekali — cukup diakses
backend dari localhost.

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

## Bagian 7 — Menelusuri order yang berpindah pemilik

Kalau pelanggan walk-in kemudian mendaftar dan mengklaim nomor WA-nya, order
lamanya ikut pindah ke akun barunya. Baris pelanggan tamu yang lama tidak
dihapus — ia jadi nisan (nonaktif, nomornya dilepas) supaya kolom audit di
tabel lain tetap punya tujuan.

Setiap perpindahan dicatat permanen di tabel `customer_merges`. Pakai ini
kalau ada pertanyaan "order ini dulu milik siapa":

```sql
SELECT o.resi,
       cm.phone,
       cm.merged_at,
       (SELECT name FROM users WHERE id = cm.from_user_id) AS dulu_milik,
       (SELECT name FROM users WHERE id = cm.to_user_id)   AS sekarang_milik
FROM customer_merges cm
JOIN orders o ON o.id = ANY(cm.order_ids)
WHERE o.resi = 'RJK-XXXXXXXX';
```

Untuk melihat semua yang pernah diserap ke satu akun:

```sql
SELECT * FROM customer_merges WHERE to_user_id = 'UUID-AKUN';
```

Catatan: `orders_moved` bisa `0`. Itu normal — artinya identitas tamunya
diserap tapi dia memang belum pernah punya order.

### Membatalkan penggabungan yang salah

Penggabungan hanya terjadi setelah pemilik nomor memasukkan kode OTP, jadi
salah gabung seharusnya sangat jarang. Kalau tetap terjadi, semuanya bisa
dikembalikan karena baris audit menyimpan daftar order yang berpindah.

Ambil dulu baris auditnya, lalu jalankan **dalam satu transaksi**:

```sql
BEGIN;

-- 1. Kembalikan ordernya ke pemilik lama.
UPDATE orders
SET customer_id = (SELECT from_user_id FROM customer_merges WHERE id = 'UUID-BARIS-AUDIT')
WHERE id = ANY (SELECT unnest(order_ids) FROM customer_merges WHERE id = 'UUID-BARIS-AUDIT');

-- 2. Lepas nomornya dari akun penyerap LEBIH DULU. Urutan ini wajib:
--    satu nomor hanya boleh dipegang satu baris, jadi langkah 3 akan
--    ditolak database kalau nomornya belum dilepas di sini.
UPDATE users SET phone = NULL, phone_verified_at = NULL
WHERE id = (SELECT to_user_id FROM customer_merges WHERE id = 'UUID-BARIS-AUDIT');

-- 3. Hidupkan lagi baris tamunya beserta nomornya.
UPDATE users SET is_active = true,
                 phone = (SELECT phone FROM customer_merges WHERE id = 'UUID-BARIS-AUDIT')
WHERE id = (SELECT from_user_id FROM customer_merges WHERE id = 'UUID-BARIS-AUDIT');

COMMIT;
```

Baris di `customer_merges` sengaja JANGAN dihapus — biarkan sebagai catatan
bahwa penggabungan pernah terjadi dan dibatalkan.

Sengaja tidak dibuatkan tombol di admin panel: endpoint yang bisa memindahkan
kepemilikan order adalah kebalikan dari hal yang justru dijaga ketat di alur
ini, dan menambah permukaan serangan baru demi kejadian yang sangat jarang
tidak sepadan.

---

## Diketahui, sengaja belum dikerjakan

- Belum ada. Semua butir sebelumnya sudah dikerjakan — lihat riwayat commit
  kalau perlu menelusuri kapan.

