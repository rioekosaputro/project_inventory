# Rental ERP-Lite

Sistem inventory & rental peralatan kantor B2B (printer, PC, monitor, sparepart, consumable, maintenance teknisi).

- Acuan lengkap: [docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md)
- Aturan kerja untuk Claude Code: [CLAUDE.md](CLAUDE.md)

## Stack

| Bagian   | Teknologi                                                                              |
| -------- | -------------------------------------------------------------------------------------- |
| Backend  | Go 1.23+, Gin, pgx/v5, golang-migrate, shopspring/decimal, golang-jwt v5, slog         |
| Database | PostgreSQL 15+ (staging: Supabase, production: Postgres sendiri)                       |
| Frontend | Vue 3 (`<script setup lang="ts">`), Vite, Pinia, Vue Router, Tailwind v4, PrimeVue 4   |
| PDF      | Gotenberg 8                                                                            |
| Storage  | S3-compatible (Supabase Storage / MinIO) atau lokal                                    |

## Struktur

```
backend/   cmd/{api,worker,migrate,seed} config/ migrations/ templates/ pkg/ internal/
frontend/  src/{lib,types,stores,router,layouts,composables,components,services,views}
deploy/    docker-compose.dev.yml, docker-compose.prod.yml, Caddyfile
docs/      IMPLEMENTATION_PLAN.md, blueprint.pdf, adr/
```

## Prasyarat

- Go 1.23+
- Node.js 20+ dan npm
- PostgreSQL 15+ (lokal) **atau** Docker (untuk `deploy/docker-compose.dev.yml`)
- `make` (di Windows: Git Bash / WSL / `choco install make`)

## Menjalankan secara lokal

### 1. Database

Opsi A — PostgreSQL lokal: buat database kosong, lalu isi `DATABASE_URL` di `backend/.env`.

```sql
CREATE DATABASE project_inventory;
```

Opsi B — Docker:

```bash
docker compose -f deploy/docker-compose.dev.yml up -d
```

### 2. Backend (dari folder `backend/`)

```bash
cp .env.example .env     # lalu isi DATABASE_URL, JWT_ACCESS_SECRET (acak >= 32 karakter), SEED_ADMIN_PASSWORD
make migrate-up
make seed
make dev
```

Perintah lain: `make test`, `make test-integration`, `make lint`, `make migrate-down`, `make migrate-new name=xxx`.

### 3. Frontend (dari folder `frontend/`)

```bash
cp .env.example .env
npm install
npm run dev
```

Perintah lain: `npm run build`, `npm run test`, `npm run lint`, `npm run type-check`.

> Catatan: backend dan frontend diisi bertahap sesuai task di Bagian 13 plan
> (T0.2 untuk backend, T0.8 untuk frontend). Perintah di atas baru berfungsi setelah task tersebut selesai.

## Keamanan

- Jangan commit file `.env`. Hanya `.env.example` (tanpa nilai rahasia asli) yang masuk repo.
- Access token disimpan di memori (Pinia), refresh token lewat cookie HttpOnly.
