# Rajaku Printing

Web app manajemen order percetakan banner (modular monolith). Company profile + katalog + order + admin panel.

Source of Truth: [`CLAUDE.md`](CLAUDE.md) — jangan menyimpang dari dokumen ini.

## Struktur

```
.
├── CLAUDE.md                  # Spec proyek (Source of Truth)
├── docker-compose.yml         # Postgres lokal (dev)
├── backend/                   # Go (Gin + GORM) — modular monolith
├── frontend/                  # Nuxt 3 (SSR/SSG)
└── services/
    └── notification-worker/   # Node.js (Baileys) — WA gateway
```

## Prasyarat

- Go >= 1.22
- Node >= 20
- Docker (untuk Postgres lokal)
- Git

## Quick Start (Dev Lokal)

```bash
cp .env.example .env
docker compose up -d postgres
```

Backend:

```bash
cd backend
cp .env.example .env
go mod tidy
go run ./cmd/api
```

Frontend:

```bash
cd frontend
cp .env.example .env
npm install
npm run dev
```

Notification worker (WhatsApp via Baileys — spec §13):

```bash
cd services/notification-worker
cp .env.example .env    # sinkronkan INTERNAL_SECRET dgn backend NOTIFICATION_WORKER_SECRET
npm install
npm start               # scan QR di terminal atau di http://localhost:9090/qr
```

Detail pairing + troubleshooting: [`services/notification-worker/README.md`](services/notification-worker/README.md).

## Aturan Kunci (ringkas — detail di `CLAUDE.md`)

- Backend Go: layer `handler → service → repository` wajib dipisah tegas.
- Semua base URL dibaca dari env terpusat (`APP_BASE_URL`), tidak boleh hardcode.
- Nomor WA distandarkan ke format `62xxxxxxxxxx` sejak input.
- Setiap `err != nil` wajib ditangani; error dibungkus konteks (`fmt.Errorf("...: %w", err)`).
- Config divalidasi saat startup (fail-fast).
- Migrasi DB pakai `golang-migrate`, bukan `AutoMigrate` GORM di production.
