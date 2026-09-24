# Rajaku Printing

Web app manajemen order percetakan banner. Company profile + katalog + order + admin panel.

## Struktur

```
backend/                   # Go (Gin + GORM) — modular monolith
frontend/                  # Nuxt 3 (SSR/SSG)
services/
  └── notification-worker/ # Node.js (Baileys) — WA gateway
docker-compose.yml         # Postgres lokal (dev)
```

## Tech Stack

- **Backend:** Go (Gin + GORM), PostgreSQL, golang-migrate
- **Frontend:** Nuxt 3 (SSR/SSG), TresJS (3D), motion-v
- **Auth:** Google OAuth + guest checkout
- **WA Gateway:** Baileys (self-hosted)
- **PDF Invoice:** gofpdf / maroto
- **Image:** Auto-convert ke WebP saat upload

## Prasyarat

- Go >= 1.22
- Node >= 20
- Docker (untuk Postgres lokal)

## Quick Start

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

WA Notification Worker:
```bash
cd services/notification-worker
cp .env.example .env
npm install
npm start    # scan QR di terminal atau http://localhost:9090/qr
```

### Docker (semua sekaligus)

```bash
cp .env.docker.example .env.docker
docker compose up -d
```

## Arsitektur Backend

Layer `handler → service → repository` dipisah tegas. Semua base URL dibaca dari env `APP_BASE_URL`. Error selalu di-wrap dengan konteks. Config divalidasi saat startup (fail-fast).

## Lisensi

Proprietary — Rajaku Printing.
