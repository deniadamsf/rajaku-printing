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
- **Rate limit**: default min 3 detik antar pesan (`MIN_SEND_INTERVAL_MS`) + batch size kecil. Baileys **tidak resmi** — nomor bisa banned WhatsApp kalau spam. Jangan turunkan tanpa alasan.
- **Retry** dikontrol backend (`NOTIFICATION_MAX_ATTEMPTS`, exponential backoff, cap 30 menit). Worker cuma lapor sekali per attempt; backend yang decide jadi `failed` (akan retry) atau `dead` (habis).
- **Nomor WA harus terpisah dari WA toko manual** (spec §25 checklist) — nomor Baileys pernah kena rate-limit atau relogin, jangan jadikan yang sama dengan yang dipakai admin manual.

## Deploy

Butuh Node.js 20+ (pakai `fetch` bawaan). Jalankan sebagai systemd service atau `pm2` di VPS. Session dir (`./session/`) harus **persistent** — kalau hilang, harus scan QR ulang.

## Troubleshooting

- `WA belum siap` di log dispatcher → belum scan QR / koneksi putus. Cek `/qr` atau terminal.
- `INTERNAL_SECRET must be >= 32 chars` → sinkronkan dengan backend `.env`.
- `backend /api/v1/... failed: HTTP 401` → secret salah. Cek header `X-Internal-Secret` sama dengan `NOTIFICATION_WORKER_SECRET` backend.
- Pesan "gagal terkirim, cek manual" di log backend saat approve pembayaran → notif enqueue error tapi transaksi tetap tersimpan (best-effort per §13). Cek `notification_jobs` table untuk row `dead` — bisa re-enqueue manual.

## Belum dikerjakan (roadmap)

- Sweeper untuk row stuck di status `sending` > N menit (kalau worker crash mid-send).
- Admin dashboard `/admin/notifications` di backend Go (saat ini query manual via psql).
- Broadcast/manual send endpoint (di luar order flow).
