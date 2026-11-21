# CLAUDE.md — Rental ERP-Lite

Sistem inventory & rental peralatan kantor B2B (printer, PC, monitor, sparepart, consumable, maintenance teknisi).
**Acuan lengkap: `docs/IMPLEMENTATION_PLAN.md`** — baca bagian yang relevan sebelum mengerjakan task. Jika ada konflik antara kode lama, blueprint PDF, dan plan → **plan yang menang**.

## Cara kerja
- User akan meminta per Task ID, mis. "Kerjakan T1.5". Cari task di Bagian 13 plan, baca bagian yang dirujuk, kerjakan sampai DoD terpenuhi, lalu centang `[x]` task tersebut di plan.
- Kerjakan satu task dalam satu waktu. Jangan mengerjakan fitur fase berikutnya tanpa diminta.
- Sebelum menulis kode yang mengubah stok/status, baca Bagian 5 (aturan bisnis) dan 6 (algoritma).
- Jika aturan bisnis tidak jelas atau tidak ada di plan, **tanya dulu** — jangan mengarang aturan.
- Setelah selesai: jalankan lint + test, ringkas file yang diubah dan cara mengujinya.

## Stack
- Backend: Go 1.23+, Gin, pgx/v5 (pgxpool), golang-migrate, shopspring/decimal, golang-jwt v5, slog. Module: `rental-inventory-api`.
- DB: PostgreSQL 15+ (staging: Supabase; production: Postgres sendiri).
- PDF: Gotenberg 8. Storage: S3-compatible (Supabase Storage / MinIO) atau lokal.
- Frontend: Vue 3 `<script setup lang="ts">`, Vite, Pinia, Vue Router, Tailwind v4, PrimeVue 4, vee-validate + zod, dayjs, Leaflet + OSM.

## Struktur
```
backend/   cmd/{api,worker,migrate,seed} config/ migrations/ templates/ pkg/ internal/{domain,repository/postgres,usecase,jobs,delivery/http}
frontend/  src/{lib,types,stores,router,layouts,composables,components,services,views}
deploy/    docker-compose.dev.yml, docker-compose.prod.yml, Caddyfile
docs/      IMPLEMENTATION_PLAN.md, blueprint.pdf
```

## Perintah
```bash
# infra lokal
docker compose -f deploy/docker-compose.dev.yml up -d
# backend (dari folder backend/)
make dev | make test | make test-integration | make lint
make migrate-up | make migrate-down | make migrate-new name=xxx | make seed
# frontend (dari folder frontend/)
npm run dev | npm run build | npm run test | npm run lint | npm run type-check
npx openapi-typescript ../backend/api/openapi.yaml -o src/types/api.generated.ts
```

## Invarian bisnis — JANGAN DILANGGAR
1. Stok (`stock_balances`, status/lokasi `product_serials`) **hanya berubah** di dalam transaksi usecase: approve LPB, reservasi/release DO, POD/pickup driver, finalisasi opname. Tidak pernah dari handler atau saat dokumen masih DRAFT.
2. Setiap mutasi stok menulis `stock_ledgers`; setiap perubahan status SN menulis `product_serial_histories`. Kedua tabel + `audit_logs` **append-only** (dijaga trigger DB). Koreksi = dokumen baru, bukan UPDATE.
3. Urutan lock: header dokumen `FOR UPDATE` → `stock_balances` urut (product_id, rack_id) → `product_serials` urut id. Validasi dilakukan setelah lock.
4. Uang: `NUMERIC(15,2)` di DB, `decimal.Decimal` di Go, string di JSON. **Dilarang float64 untuk uang.** PPN: tarif 12% × DPP Nilai Lain (11/12), tarif dari `tax_settings` — jangan hardcode 11%.
5. Tanggal bisnis (periode tagihan, tanggal POD, nomor dokumen) dihitung dalam zona **Asia/Jakarta** lewat `pkg/clock`.
6. Nomor dokumen hanya lewat `pkg/docnum` (tabel `document_sequences`), di dalam transaksi yang sama.
7. Otorisasi wajib di backend (`RequirePermission` + cek kepemilikan/warehouse scope di usecase). Guard FE hanya kenyamanan UI.
8. Migrasi yang sudah di-apply ke staging **tidak boleh diedit** — buat migrasi baru. Setiap `.up.sql` punya `.down.sql`.
9. Koneksi Supabase transaction pooler (6543) tidak mendukung prepared statement → hormati `DB_POOLER_MODE`. Jangan pakai advisory lock session, `SET` session, atau `LISTEN/NOTIFY`.
10. Jangan simpan state di memori proses (API bisa jalan serverless di Vercel). Job terjadwal = fungsi usecase idempoten (`job_runs`).

## Konvensi Go
- Layer: handler (tipis) → usecase (aturan bisnis, TxManager, tanpa Gin/pgx) → repository (SQL saja, `database.GetExecutor(ctx, pool)`).
- Interface repository didefinisikan di `internal/domain`; sentinel error di `internal/domain/errors.go`; mapping HTTP hanya di `delivery/http/httperr`.
- SQL selalu parameterized; kolom sort dari whitelist.
- Nama identifier kode dalam bahasa Inggris; pesan error/teks untuk user dalam Bahasa Indonesia.
- Respons JSON: `{success, message, data, meta}` / `{success:false, message, code, errors, request_id}`.
- Test: unit untuk usecase (mock repo), integrasi dengan testcontainers Postgres untuk semua alur stok; panggil `assertInvariants` di akhir test integrasi. Jalankan dengan `-race`.

## Konvensi Vue
- Composition API + TS strict. API hanya via `src/services/*`; tipe dari `api.generated.ts`.
- Access token di memori (Pinia), refresh token cookie HttpOnly (`/api/v1/auth/refresh`). Jangan simpan token di localStorage.
- Tabel list pakai `useServerTable` (pagination/filter server-side, sinkron ke query URL).
- Angka uang & pajak final dari backend; FE hanya menampilkan (`formatRupiah`).
- Teks UI Bahasa Indonesia, tombol diawali kata kerja. Status pakai `StatusBadge`.
- Aksi kritis (approve LPB, POD, pembayaran) kirim header `Idempotency-Key`.

## Role
`ADMIN, MANAGEMENT, BUSINESS_ADMIN, STAFF_WAREHOUSE, HEAD_WAREHOUSE, DRIVER, TECHNICIAN` — matriks permission di plan Bagian 5.1, dikodekan di `internal/domain/permission.go`.
