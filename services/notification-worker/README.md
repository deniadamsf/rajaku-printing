# notification-worker

Microservice Node.js untuk kirim WhatsApp via [Baileys](https://github.com/WhiskeySockets/Baileys). Bagian dari sistem Rajaku Printing — dipanggil oleh backend Go (spec §13).

## Arsitektur

```
[order/payment service] ─insert row──▶ notification_jobs (Postgres)
                                                │
                                                │ HTTP polling (X-Internal-Secret)
                                                ▼
                                     [notification-worker]
                                           │
                                           │ Baileys → WhatsApp Web
                                           ▼
                                        pelanggan
                                           │
                                           │ HTTP callback (sent/failed)
                                           ▼
                                     backend Go (mark job status)
```

- **Backend Go** yang jadi source of truth (schema, retry policy, dedup). Worker cuma _driver_ pengirim.
- **Antrian = tabel Postgres** (`notification_jobs`) — bukan Redis (spec §13: cukup untuk MVP, tambah Redis nanti kalau volume tinggi).
- Worker **polling** endpoint `/internal/notifications/claim` tiap N ms, kirim, lapor balik ke `/internal/notifications/:id/sent` atau `/failed`.

## Setup

```bash
cd services/notification-worker
cp .env.example .env
# Edit .env — WAJIB set INTERNAL_SECRET yang sama dengan backend
# NOTIFICATION_WORKER_SECRET (>= 32 chars).
npm install
npm start
```

**Pairing pertama kali** (§13 — Baileys pakai QR WhatsApp Web):

1. Jalankan `npm start` — di terminal muncul QR ASCII.
2. Buka WhatsApp di HP → **Perangkat Tertaut** → **Tautkan Perangkat** → scan QR.
3. Setelah muncul log `WA connected`, worker siap kirim. Session tersimpan di `./session/` — restart tidak perlu scan ulang.

Alternatif: buka `http://localhost:9090/qr` di browser (kalau tidak enak lihat terminal). Health check di `http://localhost:9090/healthz`.

## Konvensi penting

- **Nomor HP** wajib format `62xxx` (10-15 digit, tanpa `+`/`0`). Backend Go sudah normalize via `pkg/phone` — worker cuma validasi shape lagi.
- **Teks pesan disusun di backend Go** (kolom `notification_jobs.message`). Worker mengirim isinya **apa adanya** — tidak menormalkan, memotong, atau menambah apa pun.
- **Retry** dikontrol backend (`NOTIFICATION_MAX_ATTEMPTS`, exponential backoff, cap 30 menit). Worker cuma lapor sekali per attempt; backend yang decide jadi `failed` (akan retry) atau `dead` (habis).
- **Nomor WA harus terpisah dari WA toko manual** (spec §25 checklist) — nomor Baileys pernah kena rate-limit atau relogin, jangan jadikan yang sama dengan yang dipakai admin manual.

## Perlindungan anti-ban (§13)

Baileys **tidak resmi** — nomor bisa diblokir WhatsApp kalau polanya terlihat seperti bot atau memicu laporan penerima. Worker punya 6 lapis perlindungan, semuanya configurable lewat env (lihat `.env.example` untuk daftar lengkap + default).

| Lapis | File | Ringkasan |
|---|---|---|
| 1. Pacing acak | `src/pacing.js` | Jeda antar pesan diacak `MIN_SEND_INTERVAL_MS`–`MAX_SEND_INTERVAL_MS` (default 3–9 detik, distribusi segitiga, bukan uniform kaku). Tiap `BURST_PAUSE_EVERY` pesan (default 8) worker "istirahat" `BURST_PAUSE_MS`–`BURST_PAUSE_MAX_MS` (default 45–120 detik). |
| 2. Warmup + cap harian | `src/quota.js` | Cap harian menanjak mengikuti umur sesi: `WA_WARMUP_RAMP=20,40,80,150,250` (hari ke-1..ke-5), lalu `WA_DAILY_CAP` (default 500). Cap tercapai → worker berhenti **mengklaim** job sampai lewat tengah malam `TZ` (default `Asia/Jakarta`), dengan log `CAP HARIAN TERCAPAI`. |
| 3. Cap per penerima | `src/quota.js` | Maks `WA_MAX_PER_RECIPIENT_DAY` (default 5) pesan/nomor/hari + jeda `WA_MIN_RECIPIENT_INTERVAL_MS` (default 60 detik) antar pesan ke nomor sama. Laporan penerima = sinyal ban terkuat. |
| 4. Cek nomor terdaftar | `src/wa.js` | `sock.onWhatsApp()` sebelum kirim (`WA_CHECK_REGISTERED`, cache 24 jam). Nomor tanpa WhatsApp **tidak dikirimi**, langsung dilapor gagal permanen. |
| 5. Simulasi mengetik | `src/wa.js` | `presenceSubscribe` → `composing` → jeda proporsional panjang pesan (40–80 ms/karakter, clamp 1–4 detik) → `paused` → kirim. Matikan lewat `WA_SIMULATE_TYPING=false`. |
| 6. Circuit breaker | `src/circuit.js` | `WA_CIRCUIT_FAILURE_THRESHOLD` (default 3) error bahaya berturut-turut (connection closed / rate-limit / 429-like) → pengiriman berhenti dengan backoff eksponensial `WA_CIRCUIT_BASE_BACKOFF_MS` → `WA_CIRCUIT_MAX_BACKOFF_MS`. |

**State cap & warmup di-persist** ke `WA_STATE_FILE` (default `./wa-state.json`, di sebelah `SESSION_DIR` — bukan di dalamnya, karena isi session dir dihapus saat logout WA). File ini **harus ikut persistent** di VPS, kalau hilang hitungan harian mulai dari nol dan warmup dihitung ulang dari hari ke-1.

Umur sesi di-anchor ke nomor yang ter-pairing: kalau nomornya berganti, warmup otomatis mulai lagi dari hari ke-1.

**Alasan gagal berprefix `PERMANENT:`** dipakai untuk kegagalan yang tidak akan sembuh dengan retry (nomor tidak terdaftar, `message` kosong, format nomor salah). Kontrak `/failed` belum punya flag khusus, jadi backend tetap yang menghabiskan attempt sampai `dead` — prefix ini untuk triase operator (dan bisa dijadikan short-circuit di backend nanti).

Status runtime bisa dilihat di `GET /healthz` (`quota`, `circuit`, `pacing`) — kalau worker diam, cek dulu di sini apakah karena cap harian atau circuit terbuka.

## Troubleshooting

- `WA belum siap` di log dispatcher → belum scan QR / koneksi putus. Cek `/qr` atau terminal.
- `INTERNAL_SECRET must be >= 32 chars` → sinkronkan dengan backend `.env`.
- `backend /api/v1/... failed: HTTP 401` → secret salah. Cek header `X-Internal-Secret` sama dengan `NOTIFICATION_WORKER_SECRET` backend.
- Pesan "gagal terkirim, cek manual" di log backend saat approve pembayaran → notif enqueue error tapi transaksi tetap tersimpan (best-effort per §13). Cek `notification_jobs` table untuk row `dead` — bisa re-enqueue manual.
- Worker jalan tapi tidak mengirim apa-apa → cek `GET /healthz`: `quota.remaining_today = 0` (cap harian/warmup habis, tunggu tengah malam atau naikkan `WA_WARMUP_RAMP`/`WA_DAILY_CAP`) atau `circuit.open = true` (WA sedang menahan, tunggu `remaining_open_ms`).
- Job gagal dengan alasan `PERMANENT: nomor tidak terdaftar di WhatsApp` → nomor pelanggan memang tidak punya WhatsApp. Perbaiki datanya di order, jangan naikkan retry.
- Job gagal dengan alasan `cap per penerima tercapai` → satu pelanggan dapat > `WA_MAX_PER_RECIPIENT_DAY` notif dalam sehari. Biasanya tanda ada loop status di backend, bukan tanda cap-nya kekecilan.

## Belum dikerjakan (roadmap)

- Sweeper untuk row stuck di status `sending` > N menit (kalau worker crash mid-send).
- Admin dashboard `/admin/notifications` di backend Go (saat ini query manual via psql).
- Broadcast/manual send endpoint (di luar order flow).
