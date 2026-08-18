# Runbook Deploy — Rajaku Printing

Urutan ini mengasumsikan VPS sudah punya PostgreSQL, Node 20+, Go, dan domain
sudah mengarah ke server.

## 1. Google Cloud Console (tidak bisa diotomasi)

1. **Tambah redirect URI produksi.** APIs & Services → Credentials → OAuth 2.0
   Client ID yang dipakai → Authorized redirect URIs → tambahkan:

   ```
   https://DOMAIN-ANDA/api/v1/auth/google/callback
   ```

   Biarkan URI localhost tetap ada — satu client boleh punya banyak URI, jadi
   dev tetap jalan. URI harus **persis sama** dengan `GOOGLE_OAUTH_REDIRECT_URL`
   di `.env` server, termasuk http/https dan ada-tidaknya slash di akhir.

2. **Publish OAuth consent screen.** OAuth consent screen → Publishing status →
   **Publish app**. Selama masih "Testing", HANYA akun yang terdaftar di daftar
   test user yang bisa login — pelanggan biasa akan ditolak Google.

## 2. Backend `.env` di server

Salin dari `backend/.env.example` lalu ubah minimal ini:

| Var | Nilai produksi |
|---|---|
| `APP_BASE_URL` | `https://DOMAIN-ANDA` (API) |
| `APP_FRONTEND_URL` | URL Nuxt; kosongkan kalau satu domain |
| `GOOGLE_OAUTH_CLIENT_ID` / `_SECRET` | dari Console |
| `GOOGLE_OAUTH_REDIRECT_URL` | sama persis dengan langkah 1 |
| `NOTIFICATION_WORKER_SECRET` | acak, minimal 32 karakter |
| kredensial DB | sesuai server |

Config divalidasi saat startup (fail-fast §22) — kalau ada yang salah, server
menolak jalan dan menyebutkan var mana. Itu perilaku yang benar, bukan bug.

## 3. Migration

```bash
cd backend && make migrate-up
```

Tiga migration baru di rilis ini:
- `000016` — meredaksi kode OTP plaintext yang terlanjur tersimpan di
  `notification_jobs`. Tidak bisa di-rollback (data memang dihapus).
- `000017` — nomor pelanggan jadi nullable + partial unique index
  `WHERE phone IS NOT NULL`, dan tabel OTP kini melayani dua jalur.
- `000018` — tabel `site_media` + permission `sitemedia.manage` untuk
  super admin (pengelola gambar landing page).

Ketiganya sudah diuji apply ke Postgres lokal, bukan cuma dibaca.

## 4. Frontend

`frontend/.env`: `NUXT_PUBLIC_API_BASE=https://DOMAIN-ANDA/api/v1` dan
`NUXT_PUBLIC_APP_BASE_URL=https://DOMAIN-ANDA`. Lalu `npm ci && npm run build`,
jalankan `node .output/server/index.mjs` di bawah systemd/pm2.

## 5. Notification worker (Baileys)

```bash
cd services/notification-worker
cp .env.example .env    # INTERNAL_SECRET harus SAMA dengan NOTIFICATION_WORKER_SECRET backend
npm install && npm start
```

QR ASCII muncul di terminal, atau buka `http://localhost:9090/qr`. Scan dari
WhatsApp → Perangkat Tertaut → Tautkan Perangkat.

**Wajib diperhatikan:**
- Pakai nomor **terpisah** dari WA toko yang dipakai manual (§25).
- Folder `session/` harus persistent. Kalau hilang, scan QR ulang.
- File `wa-state.json` di sebelah `session/` menyimpan hitungan kuota &
  warmup. Jangan dihapus — kalau hilang, warmup mulai dari hari ke-1 lagi.
- **Hari pertama hanya 20 pesan/hari**, naik 40/80/150/250 lalu 500. Ini
  sengaja: nomor baru yang langsung menyemprot adalah penyebab ban paling
  umum. Kalau nomor yang dipakai sudah lama aktif, ramp boleh dinaikkan lewat
  `WA_WARMUP_RAMP` — dengan risiko ditanggung sendiri.
- Sisa jatah hari ini bisa dilihat di `http://localhost:9090/healthz`.

## 6. Cek cepat setelah naik

1. `GET /api/v1/ping` balas `{"success":true,...}`.
2. Login Google dari browser bersih (bukan akun test) — pastikan consent
   screen tidak lagi bilang "unverified/testing".
3. Registrasi dengan **melewati** nomor → akun jadi tanpa OTP.
4. Registrasi dengan nomor baru → akun jadi, **tidak ada WA terkirim**.
5. Tambah nomor yang sudah dipakai pelanggan lain → muncul tawaran verifikasi
   dan OTP masuk ke WA.
6. `/lacak/RESI` publik bisa dibuka tanpa login.
7. Login sebagai super admin -> **Kelola -> Media Landing Page**. Unggah satu
   gambar ke slot mana pun, buka landing, pastikan gambarnya berganti. Lalu
   tekan "kembalikan ke bawaan" dan pastikan gambar statis kembali muncul.
8. Buka halaman pesanan pelanggan dan pastikan rekening yang tampil
   **BCA - CV WANSHOU NIAGA UTAMA - 3245070777**. Salah satu nilai keliru di
   sini berarti uang pelanggan salah alamat, jadi periksa dengan mata sendiri.

## Diketahui, belum dikerjakan

- Order lama milik guest **tidak ikut pindah** saat nomornya diklaim lewat
  menu tambah-nomor di akun; hanya matching key-nya yang berpindah. Registrasi
  Google yang langsung menyerap guest tetap menggabungkan riwayat seperti
  biasa. Perlu dirapikan agar sesuai janji §11 (satu pelanggan, satu riwayat).
- Halaman pairing WhatsApp di admin panel belum dibuat — pairing masih lewat
  QR di terminal / `http://localhost:9090/qr`.
- Rekening masih hardcode di `frontend/utils/payment.ts`; menggantinya berarti
  deploy ulang. Belum dipindah ke modul settings.
- QRIS belum tersedia. Catatan ke pembeli sudah berbunyi "segera hadir", TAPI
  tombol pilihan metode "QRIS" di form unggah bukti bayar MASIH bisa dipilih.
  Pertimbangkan menonaktifkannya sampai QRIS benar-benar ada.
- Scan `design-drift-auditor` untuk UI baru belum dijalankan (kosmetik).
- Unggah gambar via admin belum pernah dicoba ujung-ke-ujung dengan sesi staff
  sungguhan (butuh login admin). Endpoint & UI-nya lolos build dan cadangan
  statisnya terverifikasi; langkah 7 di atas yang membuktikan sisanya.
