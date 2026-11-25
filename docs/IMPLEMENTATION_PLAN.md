# IMPLEMENTATION PLAN — Rental ERP-Lite (Inventory & Rental Peralatan Kantor B2B)

> **Dokumen acuan utama untuk development (manusia & Claude Code di VS Code).**
> Sumber: *Blueprint Sistem Inventory & Rental ERP-Lite* (PDF, 38 hlm) + pelengkap & koreksi di dokumen ini.
> Stack: **Go 1.23+ (Gin, pgx/v5)** · **PostgreSQL 15+** · **Gotenberg 8** · **Vue 3 + TypeScript (Vite, Pinia, Tailwind, PrimeVue)** · **Leaflet + OpenStreetMap**
> Target deploy: **Testing/Staging** → Vercel (frontend + API Go) + Supabase (Postgres + Storage) · **Production** → VPS/hosting + domain sendiri.

---

**Daftar Isi:** 0 Cara Pakai · 1 Ringkasan · 2 Review Blueprint · 3 Arsitektur & Deploy · 4 Database (SQL lengkap) · 5 Aturan Bisnis & State Machine · 6 Algoritma Transaksi Inti · 7 Backend Go · 8 REST API · 9 Frontend Vue · 10 Keamanan · 11 Testing · 12 Runbook Deploy · 13 Roadmap & Task · 14 Backlog · 15 Glosarium

---

## 0. Cara Memakai Dokumen Ini

1. Simpan file ini di root repo sebagai `docs/IMPLEMENTATION_PLAN.md`, dan `CLAUDE.md` di root repo (Claude Code membaca `CLAUDE.md` otomatis).
2. Kerjakan **per Task ID** (contoh `T1.3`) sesuai urutan di **Bagian 13 (Roadmap)**. Jangan lompat fase: Fase 2 bergantung pada tabel & service Fase 1.
3. Setiap task punya **Definition of Done (DoD)**. Centang `[x]` di file ini setelah DoD terpenuhi (termasuk test).
4. Saat memberi perintah ke Claude Code, cukup tulis: *"Kerjakan T1.3 sesuai docs/IMPLEMENTATION_PLAN.md"*.
5. Bagian yang bertanda **⚠️ KOREKSI** adalah perbaikan terhadap blueprint PDF — ikuti versi dokumen ini.
6. Bagian yang bertanda **💡 MASUKAN** adalah rekomendasi tambahan — boleh ditunda, tapi skemanya sudah disiapkan agar tidak perlu migrasi besar nanti.

---

## 1. Ringkasan Proyek

Platform **Rental ERP-Lite** untuk perusahaan sewa peralatan kantor B2B (printer, PC, monitor, sparepart, consumable, maintenance teknisi) yang menggantikan pencatatan gudang manual.

**Kapabilitas inti:**

| Area | Isi |
|---|---|
| Gudang | Multi-gudang, rak (Zona-Rak-Level), stok **hybrid** (unit ber-Serial Number + barang non-SN), scanner barcode POS (USB HID), Stock Opname |
| Dokumen | LPB Masuk/Keluar (Lembar Penerimaan/Pengeluaran Barang) dengan approval Kepala Warehouse → satu-satunya pemicu mutasi stok (ACID) |
| Audit | `stock_ledgers` append-only (buku besar mutasi) + `audit_logs` aksi user |
| Komersial | Pelanggan + cabang (alamat, kode pos, koordinat), kontrak sewa bulanan, SO (Kirim/Tarik/Swap/Consumable), DO, tagihan bulanan otomatis (PPN / Non-PPN) |
| Lapangan | Mobile web driver (checkpoint GPS, foto POD), peta sebaran aset, tiket maintenance teknisi + permintaan sparepart + eskalasi swap |
| Output | PDF Surat Jalan ber-QR verifikasi (Gotenberg), PDF Invoice, export Excel |
| Akses | JWT access + refresh, RBAC 7 role (ADMIN, MANAGEMENT, BUSINESS_ADMIN, STAFF_WAREHOUSE, HEAD_WAREHOUSE, DRIVER, TECHNICIAN) |

**Fase bisnis (dari blueprint):**
- **Fase 1 — Inti Gudang:** master produk/rak, stok hybrid, scanner POS, LPB + approval, stock opname, PDF Surat Jalan.
- **Fase 2 — Komersial:** pelanggan, kontrak, SO, DO + reservasi, tagihan bulanan.
- **Fase 3 — Lapangan:** driver mobile (GPS + POD), peta aset, tiket maintenance.
- **Fase 4 (tambahan plan ini) — Hardening & Production:** keamanan, observability, migrasi data awal, cutover ke hosting production.

---

## 2. Catatan Review Blueprint (Koreksi & Celah)

| # | Temuan | Tindakan di plan ini |
|---|---|---|
| R1 | **Blok kode di PDF terpotong di batas halaman** (skema `product_serials`, LPB, ledger, SO/DO, invoice, tracking, tiket tidak lengkap; beberapa fungsi Go/Vue terpotong). | Skema SQL **lengkap** ditulis ulang di Bagian 4. Logika fungsi yang terpotong ditulis sebagai algoritma di Bagian 6. |
| R2 | ⚠️ **PPN di-hardcode 11%.** Sejak PMK 131/2024 tarif PPN 12%, tapi untuk barang/jasa non-mewah dihitung dengan **DPP Nilai Lain = 11/12 × harga jual**, sehingga efektif tetap 11%. Faktur pajak memakai kode transaksi 04. | Invoice menyimpan `dpp`, `dpp_other_value`, `tax_rate` (12.00), `tax_amount`. Rumus & tarif dari tabel `tax_settings` (bisa berubah tanpa deploy). Bagian 5.6. |
| R3 | 💡 Sewa peralatan B2B umumnya kena **PPh 23 (2%)** yang dipotong pelanggan badan → pembayaran masuk lebih kecil dari tagihan. | Tabel `payments` punya `pph23_amount` + nomor bukti potong. Konfirmasi ke tim finance/pajak. |
| R4 | `customer_sites ... ON DELETE CASCADE` berbahaya karena site direferensikan oleh unit tersewa, SO, invoice. | Ganti `ON DELETE RESTRICT` + soft delete (`is_active`). |
| R5 | `rental_contracts.created_by` tanpa FK; belum ada tabel `users`. | Tabel `users`, `refresh_tokens` ditambahkan; semua `*_by` ber-FK ke `users`. |
| R6 | Kontrak belum punya **item & tarif sewa per unit/produk** → cron tagihan tidak tahu harga. | Tabel `rental_contract_items` (produk, qty kontrak, tarif bulanan, site). |
| R7 | Belum ada alur **pengiriman gagal**, **retur sewa (penarikan)** detail, **transfer antar gudang/rak**, **pemusnahan/retire unit**, **supplier** untuk LPB PURCHASE. | Subtype LPB diperluas (Bagian 5.2) + tabel `suppliers`. |
| R8 | Belum ada **penomoran dokumen** yang aman konkuren (LPB/SO/DO/INV). | Tabel `document_sequences` dengan `INSERT ... ON CONFLICT DO UPDATE ... RETURNING` (atomik, ikut rollback) (Bagian 6.1). |
| R9 | **Segregation of duties**: approver LPB tidak boleh sama dengan pembuat. | Validasi `approved_by != created_by` (kecuali ADMIN, opsional via setting). |
| R10 | Komponen scanner membuat `new AudioContext()` setiap beep → browser membatasi jumlah AudioContext, lama-lama beep gagal. | Pakai satu `AudioContext` bersama (singleton) dan `resume()` saat interaksi pertama. |
| R11 | Refresh token di `localStorage` (blueprint sudah mengakui). Jika FE & API beda domain, cookie HttpOnly jadi third-party cookie dan sering diblokir. | Proxy `/api/*` lewat **rewrite Vercel** (FE & API satu origin) → cookie `HttpOnly; Secure; SameSite=Lax` aman. Bagian 3.4. |
| R12 | Invariant stok SN: `stock_balances.qty_on_hand` (produk SN) harus = jumlah serial `AVAILABLE`+`RESERVED`+`MAINTENANCE`+`BROKEN` di rak itu. Belum ada pengaman. | Query rekonsiliasi + job harian + endpoint admin (Bagian 5.9). |
| R13 | Tile OpenStreetMap publik punya *usage policy* (wajib atribusi, tidak untuk trafik berat). | Wajib atribusi; jika user banyak, ganti tile provider (konfigurasi `VITE_MAP_TILE_URL`). |
| R14 | Error mapping domain → HTTP ditulis manual di tiap handler (switch). | Satu fungsi `httperr.Write(c, err)` / middleware terpusat (Bagian 7.4). |

---

## 3. Keputusan Arsitektur & Strategi Deployment

### 3.1 Keputusan Teknis (ADR ringkas)

| Topik | Keputusan | Alasan |
|---|---|---|
| Repo | **Monorepo**: `backend/`, `frontend/`, `deploy/`, `docs/` | Satu konteks untuk Claude Code, kontrak API & tipe mudah disinkronkan |
| HTTP | Gin | Sesuai blueprint, ekosistem middleware matang |
| DB driver | pgx/v5 + `pgxpool` | Sesuai blueprint, performa & dukungan tipe Postgres |
| Query | SQL tulis tangan di repository (sesuai blueprint). 💡 Opsional: **sqlc** untuk generate kode type-safe dari SQL | sqlc mengurangi bug scan kolom; tetap kompatibel dengan pola `GetExecutor(ctx)` |
| Migrasi | **golang-migrate** (`migrations/NNNNNN_name.up.sql/.down.sql`) | CLI + library, mudah dijalankan di CI dan Supabase |
| Uang | `NUMERIC(15,2)` di DB, **`shopspring/decimal`** di Go, string di JSON | Hindari float; JSON number besar bisa kehilangan presisi di JS |
| Waktu | `TIMESTAMPTZ` di DB (UTC), konversi **Asia/Jakarta** untuk periode tagihan & tampilan | Periode tagihan harus konsisten WIB |
| Auth | JWT sendiri (access 15 menit, refresh 7–14 hari, rotasi, disimpan hash di DB) | Tidak bergantung Supabase Auth → mudah pindah ke hosting sendiri |
| PDF | Gotenberg 8 (container) | Sesuai blueprint; render HTML/CSS via Chromium |
| Storage file | Interface `storage.Uploader` dengan implementasi **S3-compatible** (Supabase Storage untuk staging, MinIO/S3/lokal untuk production) | Satu kode, ganti lewat env |
| Job terjadwal | Fungsi usecase idempoten, dipanggil dari **(a)** `cmd/worker` (robfig/cron) di production, **(b)** endpoint `/internal/jobs/*` + Vercel Cron di staging | Vercel tidak bisa menjalankan proses worker permanen |
| Frontend | Vue 3 `<script setup lang="ts">`, Vite, Pinia, Vue Router, Tailwind v4, PrimeVue 4 (DataTable, Dialog, Calendar), vee-validate + zod, dayjs | Sesuai blueprint + validasi form yang rapi |
| Peta | Leaflet + OSM, `L.divIcon` | Sesuai blueprint, gratis tanpa API key |
| Kontrak API | `backend/api/openapi.yaml` sebagai sumber kebenaran → generate tipe TS dengan `openapi-typescript` | FE & BE tidak drift |

### 3.2 Tiga Environment

| Env | Frontend | Backend API | Database | PDF (Gotenberg) | Storage | Job terjadwal |
|---|---|---|---|---|---|---|
| **Local** | `vite dev` :5173 | `go run ./cmd/api` :8080 | Postgres di docker-compose | container lokal :3000 | MinIO atau folder lokal | `go run ./cmd/worker` |
| **Staging/Testing** | Vercel (project `erp-web`) | Vercel Go preset (project `erp-api`) **atau** container host (Render/Railway/Fly/Koyeb) | Supabase Postgres | Container terpisah (Render/Fly/Koyeb/VPS kecil) | Supabase Storage (S3 API) | Vercel Cron → `/internal/jobs/*` |
| **Production** | Static build dilayani Caddy/Nginx (atau tetap Vercel) | Container `api` di VPS | Postgres di VPS / managed | container `gotenberg` (jaringan internal) | MinIO/S3/disk | container `worker` |

### 3.3 Catatan Penting Deploy ke Vercel + Supabase

**Backend Go di Vercel**
- Vercel punya runtime Go (masih **Beta**) dengan Framework Preset yang mendukung `net/http`, `chi`, maupun `gin`. Preset mendeteksi `go.mod` di root project dan entrypoint `main.go`, `cmd/api/main.go`, atau `cmd/server/main.go`; server **wajib listen di env `PORT`**.
  → Atur **Root Directory** project Vercel ke `backend/` dan pastikan `cmd/api/main.go` membaca `PORT`.
- Gin di Vercel berjalan sebagai *function* — **jangan** bergantung pada state in-memory (cache lokal, goroutine background, rate limiter in-memory). Semua state di Postgres.
- **Gotenberg tidak bisa jalan di Vercel** → hosting sebagai container terpisah; set `GOTENBERG_URL`. Lindungi dengan basic auth / jaringan privat (Gotenberg 8 mendukung `--api-enable-basic-auth`).
- Batas ukuran body request function Vercel sekitar 4,5 MB → foto POD **wajib dikompres di browser** (blueprint sudah: < 300 KB). Untuk lampiran besar, pakai *signed upload URL* langsung ke storage.
- **Vercel Cron di paket Hobby hanya boleh minimal sekali per hari**, dan waktunya bisa meleset dalam rentang satu jam. Cukup untuk generate tagihan & alert stok harian. Endpoint cron diproteksi header `Authorization: Bearer $CRON_SECRET`. Jadwal cron Vercel memakai **UTC** → 01:00 WIB = `0 18 * * *`.
- Jika ingin perilaku identik dengan production (proses panjang, worker, Gotenberg satu jaringan), alternatif staging yang lebih mulus: deploy **container Docker yang sama** ke Render/Railway/Fly/Koyeb. Kode tidak berubah — hanya target deploy.

**Supabase Postgres**
- Gunakan **Shared pooler (Supavisor)**. Transaction mode di port **6543**, session mode di port **5432**; shared pooler bisa diakses lewat IPv4 di semua paket (direct connection `db.<ref>.supabase.co` umumnya IPv6).
- ⚠️ **Transaction mode tidak mendukung prepared statements.** pgx default memakai prepared statement cache → harus dimatikan:
  ```go
  cfg, _ := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
  if os.Getenv("DB_POOLER_MODE") == "transaction" {
      cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec // atau QueryExecModeSimpleProtocol
  }
  cfg.MaxConns = 5 // kecil di serverless; 20–30 di VPS
  ```
  Atau lewat DSN: `...:6543/postgres?default_query_exec_mode=exec`.
- `SELECT ... FOR UPDATE` di dalam transaksi **aman** di transaction mode (lock dilepas saat commit/rollback). Yang **tidak boleh**: advisory lock level session, `SET` level session, `LISTEN/NOTIFY` → desain ini tidak memakainya.
- **Migrasi** dijalankan lewat **session mode (5432)** atau direct URL (`MIGRATE_DATABASE_URL`), bukan 6543.
- Jangan pakai fitur Supabase-spesifik (RLS, Supabase Auth, Edge Functions) agar migrasi ke Postgres production tinggal `pg_dump`/`pg_restore`. Matikan akses Data API (PostgREST) untuk schema `public` atau aktifkan RLS deny-all, karena semua akses lewat backend Go.
- Supabase paket gratis bisa **di-pause** jika tidak aktif beberapa hari — wajar untuk testing, jangan dipakai sebagai production.

**Frontend Vue di Vercel**
- SPA history mode butuh rewrite ke `index.html`. Sekaligus **proxy API** agar satu origin:
  ```json
  // frontend/vercel.json
  {
    "rewrites": [
      { "source": "/api/:path*", "destination": "https://erp-api-staging.vercel.app/api/:path*" },
      { "source": "/((?!assets/).*)", "destination": "/index.html" }
    ]
  }
  ```
- HTTPS otomatis dari Vercel → **Geolocation & kamera di HP driver langsung bisa dites** (keduanya wajib HTTPS).

### 3.4 Strategi Token (satu origin)
- `access_token` → disimpan **di memori** (Pinia, tidak di localStorage), dikirim via header `Authorization: Bearer`.
- `refresh_token` → **cookie** `erp_rt`, `HttpOnly; Secure; SameSite=Lax; Path=/api/v1/auth`.
- Saat reload halaman: FE memanggil `POST /api/v1/auth/refresh` (cookie terkirim otomatis) → dapat access token baru + data user.
- Interceptor antrean refresh dari blueprint (Bagian 7.1 PDF) **tetap dipakai**, hanya sumber refresh token berubah ke cookie.
- Mode fallback (dev lintas origin): env `AUTH_REFRESH_MODE=body|cookie`.

### 3.5 Struktur Monorepo

```
rental-erp/
├── CLAUDE.md                          # aturan untuk Claude Code
├── docs/
│   ├── IMPLEMENTATION_PLAN.md         # dokumen ini
│   ├── blueprint.pdf                  # blueprint asli
│   └── adr/                           # catatan keputusan arsitektur tambahan
├── deploy/
│   ├── docker-compose.dev.yml         # postgres, gotenberg, minio, mailpit (opsional)
│   ├── docker-compose.prod.yml        # api, worker, gotenberg, postgres, caddy
│   ├── Caddyfile
│   └── backup/pg_backup.sh
├── backend/
│   ├── go.mod                         # module rental-inventory-api
│   ├── Makefile
│   ├── Dockerfile
│   ├── vercel.json                    # (jika API di Vercel) framework: go + crons
│   ├── .env.example
│   ├── api/openapi.yaml
│   ├── cmd/
│   │   ├── api/main.go
│   │   ├── worker/main.go
│   │   ├── migrate/main.go            # wrapper golang-migrate
│   │   └── seed/main.go               # data awal (role user, gudang, kategori, tax)
│   ├── config/config.go
│   ├── migrations/
│   ├── templates/                     # lpb_surat_jalan.html, invoice_rental.html, label_sn.html
│   ├── pkg/
│   │   ├── database/{postgres.go,tx_manager.go,executor.go}
│   │   ├── jwt/token.go
│   │   ├── password/bcrypt.go
│   │   ├── pdf/gotenberg.go
│   │   ├── qrcode/qrcode.go
│   │   ├── storage/{uploader.go,s3.go,local.go}
│   │   ├── response/response.go
│   │   ├── pagination/pagination.go
│   │   ├── money/money.go             # helper decimal & pembulatan rupiah
│   │   ├── clock/clock.go             # Now() bisa di-mock, zona Asia/Jakarta
│   │   └── docnum/docnum.go           # format nomor dokumen
│   └── internal/
│       ├── domain/                    # entity, konstanta status, sentinel error, interface repo
│       ├── repository/postgres/
│       ├── usecase/
│       ├── jobs/                      # billing_job.go, low_stock_job.go, reconcile_job.go
│       └── delivery/http/
│           ├── router.go
│           ├── middleware/{auth.go,rbac.go,logger.go,requestid.go,recover.go,idempotency.go,ratelimit.go}
│           ├── httperr/httperr.go     # mapping domain error → HTTP
│           ├── dto/
│           └── handler/
└── frontend/
    ├── package.json
    ├── vercel.json
    ├── .env.example
    ├── vite.config.ts
    └── src/
        ├── main.ts / App.vue
        ├── lib/{axios.ts,format.ts,audio.ts}
        ├── types/{auth.ts,api.generated.ts,...}
        ├── stores/{auth.ts,ui.ts}
        ├── router/index.ts
        ├── layouts/{AppLayout.vue,MobileLayout.vue,PublicLayout.vue}
        ├── composables/{useServerTable.ts,useDriverCapture.ts,useScanner.ts,usePermission.ts}
        ├── components/{SerialScannerPOS.vue,AssetTrackingMap.vue,DriverDeliveryAction.vue,StatusBadge.vue,MoneyText.vue,...}
        ├── services/                  # wrapper API per modul (lpbService.ts, ...)
        └── views/
            ├── auth/ dashboard/ master/ inventory/ lpb/ opname/
            ├── customers/ contracts/ sales/ billing/
            ├── driver/ tracking/ tickets/ reports/ admin/
            ├── public/LPBVerifyView.vue
            └── errors/
```

---

## 4. Desain Database — Konvensi

- **Single Source of Truth** (dari blueprint): `stock_balances` = saldo per produk per rak; `product_serials` = identitas fisik unit; `stock_ledgers` = buku besar append-only.
- Semua nominal uang `NUMERIC(15,2)`, tarif `NUMERIC(5,2)`, koordinat `DECIMAL(10,7)`.
- Semua timestamp `TIMESTAMPTZ` (disimpan UTC). Kolom `DATE` (periode tagihan, tanggal dokumen) dihitung di zona **Asia/Jakarta** oleh aplikasi.
- Tidak ada hard delete untuk data transaksi; master memakai `is_active` (soft delete).
- `stock_ledgers`, `product_serial_histories`, `audit_logs` dijaga trigger **append-only** (UPDATE/DELETE ditolak database).
- Status disimpan sebagai `VARCHAR + CHECK` (bukan enum Postgres) agar menambah status cukup `ALTER ... CHECK` tanpa `ALTER TYPE`.
- FK yang menunjuk tabel fase berikutnya ditambahkan lewat `ALTER TABLE` di migrasi fase tersebut (urutan migrasi tetap linear).
- **Lokasi unit SN dijaga CHECK** `chk_serial_location`: status di gudang (`AVAILABLE/RESERVED/MAINTENANCE/BROKEN`) wajib punya rak; `RENTED` wajib punya pelanggan.
- **Definisi "fisik di rak"** untuk produk SN: status `AVAILABLE`, `RESERVED`, `MAINTENANCE`, `BROKEN`. Ini yang dihitung `qty_on_hand`.
- **Rak non-sellable** (`racks.is_sellable = FALSE`, zona KARANTINA/WORKSHOP): stok fisik tetap tercatat, tapi tidak dihitung sebagai "available" untuk DO.
- 💡 **Asset tag internal** (`product_serials.asset_tag`, contoh `AST-2611-00012`): label barcode milik perusahaan yang ditempel di unit. SN pabrikan sering tidak seragam (ada yang tanpa barcode, ada yang dobel antar merek). Scanner menerima **asset tag atau SN**.

### 4.1 Daftar File Migrasi

| File | Fase | Isi |
|---|---|---|
| `000001_init_auth_master` | 1 | fungsi `set_updated_at` & `forbid_update_delete`, `warehouses`, `users`, `refresh_tokens`, `racks`, `categories`, `products`, `suppliers` |
| `000002_init_audit_settings` | 1 (T0.4) | `audit_logs` (+ trigger append-only), `login_attempts` (rate limit login, ditambah di T0.6), `app_settings`, `attachments`, `idempotency_keys`, `job_runs` |
| `000003_init_inventory_lpb` | 1 | `document_sequences`, `stock_balances`, `product_serials`, `lpb_documents`, `lpb_items`, `lpb_item_serials`, `stock_ledgers`, `product_serial_histories`, trigger append-only |
| `000004_init_stock_opname` | 1 | `stock_opnames`, `stock_opname_racks`, `stock_opname_items`, `stock_opname_serials` |
| `000005_init_customers_contracts` | 2 | `customers`, `customer_sites`, `rental_contracts`, `rental_contract_items`, `rental_placements` + FK ke serial & LPB |
| `000006_init_sales_delivery` | 2 | `sales_orders`, `sales_order_items`, `delivery_orders`, `delivery_order_items`, `delivery_order_reservations` |
| `000007_init_billing` | 2 | `tax_settings`, `invoices`, `invoice_items`, `payments` |
| `000008_init_tracking_tickets` | 3 | `shipment_tracking_logs`, `maintenance_tickets`, `ticket_spareparts`, `ticket_activities` |

> **Catatan penomoran (diputuskan saat T0.4):** golang-migrate hanya menjalankan versi yang lebih besar dari versi DB saat ini, dan `audit_logs` butuh fungsi `forbid_update_delete()`. Karena itu migrasi audit/settings (semula `000008`, dibuat di Fase 0) disisipkan sebagai `000002`, migrasi lain bergeser +1, dan fungsi `forbid_update_delete()` dipindah ke `000001`. Urutan file selalu linear tanpa celah. Blok SQL di Bagian 4.3 masih berurutan lama; **pakai nomor di tabel ini**. File `.sql` di `backend/migrations/` adalah sumber kebenaran.

> Semua migrasi di bawah **sudah diuji** di PostgreSQL 16 (42 tabel terbentuk) beserta uji constraint: stok negatif, reserved > on_hand, SN tanpa rak, type/subtype LPB salah, saldo ledger tidak konsisten, UPDATE/DELETE ledger, invoice ganda per periode, hapus pelanggan yang masih punya site, SO sewa tanpa kontrak → semuanya ditolak database.
>
> Setiap `.up.sql` wajib punya pasangan `.down.sql` (DROP dengan urutan terbalik; untuk `ALTER ... ADD CONSTRAINT` gunakan `DROP CONSTRAINT`).

### 4.2 ERD Ringkas (lengkap)

```
FASE 1
warehouses 1─< racks 1─< stock_balances >─1 products >─1 categories
     │            │                            │
     │            └──< product_serials >───────┘ ──< product_serial_histories
     │                       │
users ─< refresh_tokens      │
     │                       │
lpb_documents 1─< lpb_items 1─< lpb_item_serials >─ product_serials
     │   └─(APPROVED)──> stock_ledgers (append-only)
     └─< (pasangan transfer: counterpart_lpb_id)
stock_opnames 1─< stock_opname_racks / stock_opname_items / stock_opname_serials
     └─(FINALIZED)──> lpb_documents (OPNAME_ADJUSTMENT_IN / _OUT)
suppliers 1─< lpb_documents (PURCHASE), product_serials

FASE 2
customers 1─< customer_sites
     ├─< rental_contracts 1─< rental_contract_items >─ products
     │          ├─< rental_placements >─ product_serials   (periode unit terpasang → dasar tagihan)
     │          └─< invoices 1─< invoice_items ; invoices 1─< payments
     └─< sales_orders 1─< sales_order_items
                └─< delivery_orders 1─< delivery_order_items 1─< delivery_order_reservations
                          └── lpb_id ──> lpb_documents
tax_settings (tarif PPN berlaku per tanggal)

FASE 3
lpb_documents 1─< shipment_tracking_logs
maintenance_tickets >─ product_serials, customer_sites
     ├─< ticket_spareparts ──> lpb_documents (SPARE_PART_ISSUE)
     ├─< ticket_activities
     └── swap_so_id ──> sales_orders (UNIT_SWAP)

UMUM: audit_logs, app_settings, attachments, idempotency_keys, job_runs
```

### 4.3 Skema SQL Lengkap

#### `migrations/000001_init_auth_master.up.sql`

```sql
-- 000001_init_auth_master.up.sql
-- Fungsi umum, user & auth, master gudang/rak/kategori/produk/supplier

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Dipakai trigger append-only (audit_logs di 000002, stock_ledgers & product_serial_histories di 000003)
CREATE OR REPLACE FUNCTION forbid_update_delete() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Tabel % bersifat append-only: % tidak diizinkan', TG_TABLE_NAME, TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE warehouses (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(20) UNIQUE NOT NULL,          -- WH-SMG-01
    name        VARCHAR(100) NOT NULL,
    address     TEXT,
    phone       VARCHAR(30),
    latitude    DECIMAL(10, 7),
    longitude   DECIMAL(10, 7),
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
    id             BIGSERIAL PRIMARY KEY,
    name           VARCHAR(100) NOT NULL,
    email          VARCHAR(120) NOT NULL UNIQUE,
    phone          VARCHAR(30),
    password_hash  TEXT NOT NULL,
    role           VARCHAR(30) NOT NULL CHECK (role IN (
                       'ADMIN','MANAGEMENT','BUSINESS_ADMIN',
                       'STAFF_WAREHOUSE','HEAD_WAREHOUSE','DRIVER','TECHNICIAN')),
    warehouse_id   BIGINT REFERENCES warehouses(id),  -- scope gudang utk role warehouse
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    must_change_password BOOLEAN NOT NULL DEFAULT TRUE,
    last_login_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE refresh_tokens (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   CHAR(64) NOT NULL UNIQUE,            -- sha256 hex, token asli tidak disimpan
    family_id    UUID NOT NULL,                       -- deteksi reuse token curian
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    replaced_by  BIGINT REFERENCES refresh_tokens(id),
    user_agent   TEXT,
    ip_address   INET,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens(family_id);

CREATE TABLE racks (
    id            BIGSERIAL PRIMARY KEY,
    warehouse_id  BIGINT NOT NULL REFERENCES warehouses(id),
    code          VARCHAR(30) NOT NULL,               -- A-02-01 (Zona-Rak-Level)
    zone          VARCHAR(30),                        -- PRINTER, SPAREPART, KARANTINA, WORKSHOP
    is_sellable   BOOLEAN NOT NULL DEFAULT TRUE,      -- FALSE utk KARANTINA/WORKSHOP: tidak dihitung available
    description   TEXT,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    locked_by_opname_id BIGINT,                       -- FK ditambah di 000005
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (warehouse_id, code)
);

CREATE TABLE categories (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(100) NOT NULL UNIQUE,          -- Printer, PC, Monitor, Toner, Sparepart
    item_type  VARCHAR(20) NOT NULL CHECK (item_type IN ('ASSET', 'CONSUMABLE')),
    is_active  BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE products (
    id                 BIGSERIAL PRIMARY KEY,
    sku                VARCHAR(50) UNIQUE NOT NULL,   -- juga dipakai sbg barcode barang non-SN
    name               VARCHAR(150) NOT NULL,
    category_id        BIGINT NOT NULL REFERENCES categories(id),
    brand              VARCHAR(80),
    model              VARCHAR(80),
    unit               VARCHAR(20) NOT NULL DEFAULT 'unit',
    has_serial_number  BOOLEAN NOT NULL DEFAULT FALSE,
    min_stock_alert    INT NOT NULL DEFAULT 0 CHECK (min_stock_alert >= 0),
    default_monthly_rate NUMERIC(15, 2),              -- tarif sewa acuan (bisa dioverride di kontrak)
    default_sell_price NUMERIC(15, 2),                -- harga jual consumable
    description        TEXT,
    is_active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_products_category ON products(category_id);

CREATE TABLE suppliers (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(30) UNIQUE NOT NULL,
    name        VARCHAR(150) NOT NULL,
    pic_name    VARCHAR(100),
    phone       VARCHAR(30),
    email       VARCHAR(120),
    npwp        VARCHAR(30),
    address     TEXT,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_warehouses_updated BEFORE UPDATE ON warehouses FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_users_updated      BEFORE UPDATE ON users      FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_racks_updated      BEFORE UPDATE ON racks      FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_products_updated   BEFORE UPDATE ON products   FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_suppliers_updated  BEFORE UPDATE ON suppliers  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
```

#### `migrations/000003_init_inventory_lpb.up.sql`

```sql
-- 000003_init_inventory_lpb.up.sql
-- Stok hybrid, serial number, dokumen LPB, buku besar & riwayat SN, penomoran dokumen

CREATE TABLE document_sequences (
    doc_type     VARCHAR(20) NOT NULL,   -- LPBI, LPBO, SO, DO, INV, OPN, TKT, KTR, AST
    scope_key    VARCHAR(40) NOT NULL,   -- kode gudang atau 'GLOBAL'
    period       CHAR(6)     NOT NULL,   -- YYYYMM (WIB); '000000' utk sequence tanpa reset
    last_number  INT NOT NULL DEFAULT 0,
    PRIMARY KEY (doc_type, scope_key, period)
);

CREATE TABLE stock_balances (
    id            BIGSERIAL PRIMARY KEY,
    product_id    BIGINT NOT NULL REFERENCES products(id),
    rack_id       BIGINT NOT NULL REFERENCES racks(id),
    qty_on_hand   INT NOT NULL DEFAULT 0 CHECK (qty_on_hand >= 0),
    qty_reserved  INT NOT NULL DEFAULT 0 CHECK (qty_reserved >= 0 AND qty_reserved <= qty_on_hand),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, rack_id)
);
CREATE INDEX idx_stock_balances_rack ON stock_balances(rack_id);

CREATE TABLE product_serials (
    id                   BIGSERIAL PRIMARY KEY,
    product_id           BIGINT NOT NULL REFERENCES products(id),
    asset_tag            VARCHAR(30) NOT NULL UNIQUE,   -- label internal, mis. AST-2611-00012 (dicetak & discan)
    serial_number        VARCHAR(100) NOT NULL,         -- SN pabrikan
    status               VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE' CHECK (status IN (
                             'AVAILABLE','RESERVED','IN_TRANSIT','RENTED',
                             'MAINTENANCE','BROKEN','LOST','RETIRED')),
    condition            VARCHAR(20) NOT NULL DEFAULT 'GOOD'
                             CHECK (condition IN ('GOOD','MINOR_DEFECT','BROKEN')),
    current_warehouse_id BIGINT REFERENCES warehouses(id),
    current_rack_id      BIGINT REFERENCES racks(id),
    current_customer_id  BIGINT,     -- FK di 000005
    current_site_id      BIGINT,     -- FK di 000005
    current_contract_id  BIGINT,     -- FK di 000005
    reserved_for_do_id   BIGINT,     -- FK di 000006
    supplier_id          BIGINT REFERENCES suppliers(id),
    purchase_date        DATE,
    purchase_price       NUMERIC(15, 2),
    warranty_until       DATE,
    notes                TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, serial_number),
    -- Invariant lokasi: unit di gudang wajib punya rak; unit tersewa wajib punya pelanggan
    CONSTRAINT chk_serial_location CHECK (
        (status IN ('AVAILABLE','RESERVED','MAINTENANCE','BROKEN')
            AND current_rack_id IS NOT NULL AND current_customer_id IS NULL)
     OR (status = 'RENTED' AND current_customer_id IS NOT NULL AND current_rack_id IS NULL)
     OR (status IN ('IN_TRANSIT','LOST','RETIRED') AND current_rack_id IS NULL)
    )
);
CREATE INDEX idx_serials_status        ON product_serials(status);
CREATE INDEX idx_serials_rack          ON product_serials(current_rack_id) WHERE current_rack_id IS NOT NULL;
CREATE INDEX idx_serials_customer      ON product_serials(current_customer_id) WHERE current_customer_id IS NOT NULL;
CREATE INDEX idx_serials_sn            ON product_serials(serial_number);

CREATE TABLE lpb_documents (
    id                 BIGSERIAL PRIMARY KEY,
    lpb_number         VARCHAR(50) NOT NULL UNIQUE,
    type               VARCHAR(10) NOT NULL CHECK (type IN ('INBOUND','OUTBOUND')),
    sub_type           VARCHAR(30) NOT NULL CHECK (sub_type IN (
                           -- INBOUND
                           'PURCHASE','INITIAL_STOCK','RENTAL_RETURN','FAILED_DELIVERY_RETURN',
                           'TRANSFER_IN','OPNAME_ADJUSTMENT_IN',
                           -- OUTBOUND
                           'RENTAL_DELIVERY','CONSUMABLE_DELIVERY','SPARE_PART_ISSUE',
                           'TRANSFER_OUT','OPNAME_ADJUSTMENT_OUT','DISPOSAL')),
    status             VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN (
                           'DRAFT','PENDING_APPROVAL','APPROVED','REJECTED','CANCELLED')),
    warehouse_id       BIGINT NOT NULL REFERENCES warehouses(id),
    doc_date           DATE NOT NULL DEFAULT CURRENT_DATE,
    reference_type     VARCHAR(30) CHECK (reference_type IN (
                           'DELIVERY_ORDER','SALES_ORDER','STOCK_OPNAME','MAINTENANCE_TICKET',
                           'TRANSFER','PURCHASE_INVOICE','LPB')),
    reference_id       BIGINT,
    reference_number   VARCHAR(60),
    supplier_id        BIGINT REFERENCES suppliers(id),
    customer_id        BIGINT,       -- FK di 000005
    customer_site_id   BIGINT,       -- FK di 000005
    counterpart_lpb_id BIGINT REFERENCES lpb_documents(id),   -- pasangan TRANSFER_OUT/IN
    notes              TEXT,
    -- Pengiriman (OUTBOUND ke pelanggan / penjemputan unit retur)
    requires_delivery  BOOLEAN NOT NULL DEFAULT FALSE,
    driver_id          BIGINT REFERENCES users(id),
    vehicle_plate      VARCHAR(20),
    delivery_status    VARCHAR(20) CHECK (delivery_status IN (
                           'READY','PICKED_UP','IN_TRANSIT','ARRIVED','DELIVERED','FAILED','RETURNED')),
    delivered_at       TIMESTAMPTZ,
    receiver_name      VARCHAR(100),
    pod_photo_path     TEXT,
    -- Workflow approval
    created_by         BIGINT NOT NULL REFERENCES users(id),
    submitted_by       BIGINT REFERENCES users(id),
    submitted_at       TIMESTAMPTZ,
    approved_by        BIGINT REFERENCES users(id),
    approved_at        TIMESTAMPTZ,
    rejected_by        BIGINT REFERENCES users(id),
    rejected_at        TIMESTAMPTZ,
    rejection_reason   TEXT,
    verify_token       VARCHAR(64) UNIQUE,          -- random, dibuat saat APPROVED (untuk QR publik)
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_lpb_type_subtype CHECK (
        (type = 'INBOUND'  AND sub_type IN ('PURCHASE','INITIAL_STOCK','RENTAL_RETURN',
                                            'FAILED_DELIVERY_RETURN','TRANSFER_IN','OPNAME_ADJUSTMENT_IN'))
     OR (type = 'OUTBOUND' AND sub_type IN ('RENTAL_DELIVERY','CONSUMABLE_DELIVERY','SPARE_PART_ISSUE',
                                            'TRANSFER_OUT','OPNAME_ADJUSTMENT_OUT','DISPOSAL'))
    )
);
CREATE INDEX idx_lpb_status_wh   ON lpb_documents(status, warehouse_id);
CREATE INDEX idx_lpb_driver      ON lpb_documents(driver_id, delivery_status) WHERE driver_id IS NOT NULL;
CREATE INDEX idx_lpb_reference   ON lpb_documents(reference_type, reference_id);

CREATE TABLE lpb_items (
    id          BIGSERIAL PRIMARY KEY,
    lpb_id      BIGINT NOT NULL REFERENCES lpb_documents(id) ON DELETE CASCADE,
    product_id  BIGINT NOT NULL REFERENCES products(id),
    rack_id     BIGINT NOT NULL REFERENCES racks(id),   -- rak asal (OUT) / rak tujuan (IN)
    qty         INT NOT NULL CHECK (qty > 0),
    unit_cost   NUMERIC(15, 2),                          -- utk PURCHASE / INITIAL_STOCK
    do_item_id  BIGINT,                                  -- FK di 000006
    notes       TEXT,
    UNIQUE (lpb_id, product_id, rack_id)
);
CREATE INDEX idx_lpb_items_lpb ON lpb_items(lpb_id);

CREATE TABLE lpb_item_serials (
    id                 BIGSERIAL PRIMARY KEY,
    lpb_item_id        BIGINT NOT NULL REFERENCES lpb_items(id) ON DELETE CASCADE,
    product_serial_id  BIGINT REFERENCES product_serials(id),   -- NULL utk SN baru (PURCHASE) s/d APPROVED
    serial_number      VARCHAR(100) NOT NULL,
    condition_on_scan  VARCHAR(20) NOT NULL DEFAULT 'GOOD'
                           CHECK (condition_on_scan IN ('GOOD','MINOR_DEFECT','BROKEN')),
    scanned_by         BIGINT REFERENCES users(id),
    scanned_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (lpb_item_id, serial_number)
);

-- Buku besar mutasi stok: APPEND-ONLY (dijaga trigger di bawah)
CREATE TABLE stock_ledgers (
    id                 BIGSERIAL PRIMARY KEY,
    occurred_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    product_id         BIGINT NOT NULL REFERENCES products(id),
    warehouse_id       BIGINT NOT NULL REFERENCES warehouses(id),
    rack_id            BIGINT NOT NULL REFERENCES racks(id),
    direction          VARCHAR(3) NOT NULL CHECK (direction IN ('IN','OUT')),
    qty                INT NOT NULL CHECK (qty > 0),
    balance_before     INT NOT NULL,
    balance_after      INT NOT NULL,
    movement_type      VARCHAR(30) NOT NULL,                 -- = lpb_documents.sub_type
    lpb_id             BIGINT NOT NULL REFERENCES lpb_documents(id),
    lpb_item_id        BIGINT NOT NULL REFERENCES lpb_items(id),
    serial_ids         BIGINT[],                             -- id SN yang ikut bergerak (produk SN)
    unit_cost          NUMERIC(15, 2),
    created_by         BIGINT NOT NULL REFERENCES users(id),  -- = approver
    notes              TEXT,
    CONSTRAINT chk_ledger_balance CHECK (
        (direction = 'IN'  AND balance_after = balance_before + qty)
     OR (direction = 'OUT' AND balance_after = balance_before - qty)
    )
);
CREATE INDEX idx_ledgers_product_rack ON stock_ledgers(product_id, rack_id, occurred_at);
CREATE INDEX idx_ledgers_lpb          ON stock_ledgers(lpb_id);
CREATE INDEX idx_ledgers_serials      ON stock_ledgers USING GIN (serial_ids);

-- Riwayat perubahan status/lokasi setiap unit SN (termasuk event non-stok: RESERVED, RENTED, tiket)
CREATE TABLE product_serial_histories (
    id                 BIGSERIAL PRIMARY KEY,
    product_serial_id  BIGINT NOT NULL REFERENCES product_serials(id),
    from_status        VARCHAR(20),
    to_status          VARCHAR(20) NOT NULL,
    from_rack_id       BIGINT REFERENCES racks(id),
    to_rack_id         BIGINT REFERENCES racks(id),
    customer_id        BIGINT,
    event_type         VARCHAR(40) NOT NULL,   -- LPB_APPROVED, DO_RESERVED, DELIVERED, TICKET_OPENED, ...
    reference_type     VARCHAR(30),
    reference_id       BIGINT,
    user_id            BIGINT REFERENCES users(id),
    notes              TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_serial_histories_serial ON product_serial_histories(product_serial_id, created_at);

-- Fungsi forbid_update_delete() sudah didefinisikan di 000001.
CREATE TRIGGER trg_stock_ledgers_append_only
    BEFORE UPDATE OR DELETE ON stock_ledgers
    FOR EACH ROW EXECUTE FUNCTION forbid_update_delete();
CREATE TRIGGER trg_serial_histories_append_only
    BEFORE UPDATE OR DELETE ON product_serial_histories
    FOR EACH ROW EXECUTE FUNCTION forbid_update_delete();

CREATE TRIGGER trg_serials_updated BEFORE UPDATE ON product_serials FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_lpb_updated     BEFORE UPDATE ON lpb_documents   FOR EACH ROW EXECUTE FUNCTION set_updated_at();
```

#### `migrations/000004_init_stock_opname.up.sql`

```sql
-- 000004_init_stock_opname.up.sql

CREATE TABLE stock_opnames (
    id                     BIGSERIAL PRIMARY KEY,
    opname_number          VARCHAR(50) NOT NULL UNIQUE,
    warehouse_id           BIGINT NOT NULL REFERENCES warehouses(id),
    status                 VARCHAR(20) NOT NULL DEFAULT 'IN_PROGRESS' CHECK (status IN (
                               'IN_PROGRESS','SUBMITTED','FINALIZED','CANCELLED')),
    is_blind_count         BOOLEAN NOT NULL DEFAULT TRUE,   -- staf tidak melihat qty sistem saat hitung
    started_by             BIGINT NOT NULL REFERENCES users(id),
    started_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    submitted_by           BIGINT REFERENCES users(id),
    submitted_at           TIMESTAMPTZ,
    finalized_by           BIGINT REFERENCES users(id),
    finalized_at           TIMESTAMPTZ,
    adjustment_in_lpb_id   BIGINT REFERENCES lpb_documents(id),
    adjustment_out_lpb_id  BIGINT REFERENCES lpb_documents(id),
    notes                  TEXT
);

CREATE TABLE stock_opname_racks (
    opname_id  BIGINT NOT NULL REFERENCES stock_opnames(id) ON DELETE CASCADE,
    rack_id    BIGINT NOT NULL REFERENCES racks(id),
    PRIMARY KEY (opname_id, rack_id)
);

CREATE TABLE stock_opname_items (
    id            BIGSERIAL PRIMARY KEY,
    opname_id     BIGINT NOT NULL REFERENCES stock_opnames(id) ON DELETE CASCADE,
    product_id    BIGINT NOT NULL REFERENCES products(id),
    rack_id       BIGINT NOT NULL REFERENCES racks(id),
    system_qty    INT NOT NULL,          -- snapshot saat opname dimulai
    counted_qty   INT CHECK (counted_qty >= 0),
    diff_qty      INT GENERATED ALWAYS AS (counted_qty - system_qty) STORED,
    counted_by    BIGINT REFERENCES users(id),
    counted_at    TIMESTAMPTZ,
    notes         TEXT,
    UNIQUE (opname_id, product_id, rack_id)
);

CREATE TABLE stock_opname_serials (
    id                 BIGSERIAL PRIMARY KEY,
    opname_id          BIGINT NOT NULL REFERENCES stock_opnames(id) ON DELETE CASCADE,
    rack_id            BIGINT NOT NULL REFERENCES racks(id),
    product_serial_id  BIGINT REFERENCES product_serials(id),
    scanned_code       VARCHAR(100),
    result             VARCHAR(20) NOT NULL CHECK (result IN ('FOUND','MISSING','UNEXPECTED','WRONG_RACK')),
    scanned_by         BIGINT REFERENCES users(id),
    scanned_at         TIMESTAMPTZ,
    UNIQUE (opname_id, product_serial_id)
);

ALTER TABLE racks
    ADD CONSTRAINT fk_racks_locked_opname FOREIGN KEY (locked_by_opname_id) REFERENCES stock_opnames(id);
```

#### `migrations/000005_init_customers_contracts.up.sql`

```sql
-- 000005_init_customers_contracts.up.sql (Fase 2)

CREATE TABLE customers (
    id                   BIGSERIAL PRIMARY KEY,
    customer_code        VARCHAR(30) UNIQUE NOT NULL,
    company_name         VARCHAR(150) NOT NULL,
    pic_name             VARCHAR(100) NOT NULL,
    phone                VARCHAR(30) NOT NULL,
    email                VARCHAR(120),
    is_pkp               BOOLEAN NOT NULL DEFAULT TRUE,
    npwp                 VARCHAR(30),            -- NPWP 16 digit (format Coretax)
    tax_address          TEXT,                   -- alamat sesuai NPWP utk faktur
    billing_address      TEXT NOT NULL,
    billing_postal_code  VARCHAR(10) NOT NULL,
    billing_email        VARCHAR(120),           -- tujuan kirim invoice
    payment_term_days    INT NOT NULL DEFAULT 30 CHECK (payment_term_days >= 0),
    is_active            BOOLEAN NOT NULL DEFAULT TRUE,
    created_by           BIGINT REFERENCES users(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- KOREKSI: ON DELETE RESTRICT + soft delete (bukan CASCADE)
CREATE TABLE customer_sites (
    id            BIGSERIAL PRIMARY KEY,
    customer_id   BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    site_name     VARCHAR(100) NOT NULL,
    pic_name      VARCHAR(100) NOT NULL,
    pic_phone     VARCHAR(30) NOT NULL,
    address_line  TEXT NOT NULL,
    subdistrict   VARCHAR(80),
    district      VARCHAR(80),
    city          VARCHAR(80) NOT NULL,
    province      VARCHAR(80) NOT NULL,
    postal_code   VARCHAR(10) NOT NULL,
    latitude      DECIMAL(10, 7),
    longitude     DECIMAL(10, 7),
    is_primary    BOOLEAN NOT NULL DEFAULT FALSE,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_customer_sites_customer ON customer_sites(customer_id);
CREATE UNIQUE INDEX uq_customer_primary_site ON customer_sites(customer_id) WHERE is_primary AND is_active;

CREATE TABLE rental_contracts (
    id                    BIGSERIAL PRIMARY KEY,
    contract_number       VARCHAR(50) UNIQUE NOT NULL,
    customer_id           BIGINT NOT NULL REFERENCES customers(id),
    start_date            DATE NOT NULL,
    end_date              DATE NOT NULL,
    duration_months       INT NOT NULL CHECK (duration_months > 0),
    billing_day_of_month  SMALLINT NOT NULL DEFAULT 1 CHECK (billing_day_of_month BETWEEN 1 AND 28),
    billing_mode          VARCHAR(20) NOT NULL DEFAULT 'PRORATA'
                              CHECK (billing_mode IN ('PRORATA','FULL_MONTH')),
    billing_in_advance    BOOLEAN NOT NULL DEFAULT FALSE, -- MVP: tagih di belakang (arrears); di muka = backlog
    is_ppn                BOOLEAN NOT NULL DEFAULT TRUE,
    payment_term_days     INT NOT NULL DEFAULT 30,
    deposit_amount        NUMERIC(15, 2) NOT NULL DEFAULT 0,
    status                VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
                              CHECK (status IN ('DRAFT','ACTIVE','EXPIRED','TERMINATED')),
    terminated_at         DATE,
    termination_reason    TEXT,
    renewed_from_id       BIGINT REFERENCES rental_contracts(id),
    signed_document_path  TEXT,
    notes                 TEXT,
    created_by            BIGINT NOT NULL REFERENCES users(id),
    activated_by          BIGINT REFERENCES users(id),
    activated_at          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_contract_dates CHECK (end_date > start_date)
);
CREATE INDEX idx_contracts_customer ON rental_contracts(customer_id, status);

-- MASUKAN: tarif sewa per produk per kontrak (dibutuhkan cron tagihan)
CREATE TABLE rental_contract_items (
    id                BIGSERIAL PRIMARY KEY,
    contract_id       BIGINT NOT NULL REFERENCES rental_contracts(id) ON DELETE CASCADE,
    product_id        BIGINT NOT NULL REFERENCES products(id),
    customer_site_id  BIGINT REFERENCES customer_sites(id),
    qty               INT NOT NULL CHECK (qty > 0),           -- jumlah unit yang dikontrakkan
    monthly_rate      NUMERIC(15, 2) NOT NULL CHECK (monthly_rate >= 0),
    notes             TEXT,
    UNIQUE (contract_id, product_id, customer_site_id)
);

-- MASUKAN: periode penempatan setiap unit di pelanggan = dasar hitung tagihan prorata.
-- Interval [start_date, end_date) : hari start ditagih, hari end (unit keluar) tidak ditagih.
-- Swap di hari yang sama -> unit lama end_date = D, unit baru start_date = D -> hari D ditagih sekali.
CREATE TABLE rental_placements (
    id                 BIGSERIAL PRIMARY KEY,
    contract_id        BIGINT NOT NULL REFERENCES rental_contracts(id),
    contract_item_id   BIGINT REFERENCES rental_contract_items(id),
    product_serial_id  BIGINT NOT NULL REFERENCES product_serials(id),
    customer_site_id   BIGINT NOT NULL REFERENCES customer_sites(id),
    monthly_rate       NUMERIC(15, 2) NOT NULL,       -- snapshot tarif saat unit terpasang
    start_date         DATE NOT NULL,                 -- tanggal POD (DELIVERED)
    end_date           DATE,                          -- tanggal unit diambil driver (PICKED_UP) / swap
    start_lpb_id       BIGINT REFERENCES lpb_documents(id),
    end_lpb_id         BIGINT REFERENCES lpb_documents(id),
    end_reason         VARCHAR(20) CHECK (end_reason IN ('WITHDRAWAL','SWAP','CONTRACT_END','LOST')),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_placement_dates CHECK (end_date IS NULL OR end_date >= start_date)
);
CREATE UNIQUE INDEX uq_active_placement ON rental_placements(product_serial_id) WHERE end_date IS NULL;
CREATE INDEX idx_placements_contract ON rental_placements(contract_id, start_date, end_date);

ALTER TABLE product_serials
    ADD CONSTRAINT fk_serial_customer FOREIGN KEY (current_customer_id) REFERENCES customers(id),
    ADD CONSTRAINT fk_serial_site     FOREIGN KEY (current_site_id)     REFERENCES customer_sites(id),
    ADD CONSTRAINT fk_serial_contract FOREIGN KEY (current_contract_id) REFERENCES rental_contracts(id);

ALTER TABLE lpb_documents
    ADD CONSTRAINT fk_lpb_customer FOREIGN KEY (customer_id)      REFERENCES customers(id),
    ADD CONSTRAINT fk_lpb_site     FOREIGN KEY (customer_site_id) REFERENCES customer_sites(id);

CREATE TRIGGER trg_customers_updated BEFORE UPDATE ON customers        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_sites_updated     BEFORE UPDATE ON customer_sites   FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_contracts_updated BEFORE UPDATE ON rental_contracts FOR EACH ROW EXECUTE FUNCTION set_updated_at();
```

#### `migrations/000006_init_sales_delivery.up.sql`

```sql
-- 000006_init_sales_delivery.up.sql (Fase 2)

CREATE TABLE sales_orders (
    id                BIGSERIAL PRIMARY KEY,
    so_number         VARCHAR(50) NOT NULL UNIQUE,
    order_type        VARCHAR(20) NOT NULL CHECK (order_type IN (
                          'NEW_DELIVERY','WITHDRAWAL','UNIT_SWAP','CONSUMABLE')),
    customer_id       BIGINT NOT NULL REFERENCES customers(id),
    customer_site_id  BIGINT NOT NULL REFERENCES customer_sites(id),
    contract_id       BIGINT REFERENCES rental_contracts(id),  -- wajib utk NEW_DELIVERY/WITHDRAWAL/UNIT_SWAP
    status            VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN (
                          'DRAFT','CONFIRMED','IN_PROGRESS','COMPLETED','CANCELLED')),
    order_date        DATE NOT NULL DEFAULT CURRENT_DATE,
    requested_date    DATE,
    is_ppn            BOOLEAN NOT NULL DEFAULT TRUE,
    subtotal          NUMERIC(15, 2) NOT NULL DEFAULT 0,
    discount_amount   NUMERIC(15, 2) NOT NULL DEFAULT 0,
    dpp               NUMERIC(15, 2) NOT NULL DEFAULT 0,
    dpp_other_value   NUMERIC(15, 2) NOT NULL DEFAULT 0,
    tax_rate          NUMERIC(5, 2)  NOT NULL DEFAULT 0,
    tax_amount        NUMERIC(15, 2) NOT NULL DEFAULT 0,
    grand_total       NUMERIC(15, 2) NOT NULL DEFAULT 0,
    related_ticket_id BIGINT,                                  -- FK di 000008 (SO swap dari tiket)
    notes             TEXT,
    created_by        BIGINT NOT NULL REFERENCES users(id),
    confirmed_by      BIGINT REFERENCES users(id),
    confirmed_at      TIMESTAMPTZ,
    cancelled_by      BIGINT REFERENCES users(id),
    cancelled_at      TIMESTAMPTZ,
    cancel_reason     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_so_contract CHECK (order_type = 'CONSUMABLE' OR contract_id IS NOT NULL)
);
CREATE INDEX idx_so_customer ON sales_orders(customer_id, status);

CREATE TABLE sales_order_items (
    id                 BIGSERIAL PRIMARY KEY,
    so_id              BIGINT NOT NULL REFERENCES sales_orders(id) ON DELETE CASCADE,
    product_id         BIGINT NOT NULL REFERENCES products(id),
    direction          VARCHAR(3) NOT NULL DEFAULT 'OUT' CHECK (direction IN ('OUT','IN')),
                       -- OUT = dikirim ke pelanggan, IN = ditarik dari pelanggan (WITHDRAWAL / sisi lama SWAP)
    qty                INT NOT NULL CHECK (qty > 0),
    unit_price         NUMERIC(15, 2) NOT NULL DEFAULT 0,      -- tarif bulanan (sewa) / harga satuan (consumable)
    line_total         NUMERIC(15, 2) NOT NULL DEFAULT 0,
    product_serial_id  BIGINT REFERENCES product_serials(id),  -- unit spesifik yg ditarik (direction IN)
    notes              TEXT
);
CREATE INDEX idx_so_items_so ON sales_order_items(so_id);

CREATE TABLE delivery_orders (
    id              BIGSERIAL PRIMARY KEY,
    do_number       VARCHAR(50) NOT NULL UNIQUE,
    so_id           BIGINT NOT NULL REFERENCES sales_orders(id),
    warehouse_id    BIGINT NOT NULL REFERENCES warehouses(id),
    direction       VARCHAR(10) NOT NULL CHECK (direction IN ('OUTBOUND','INBOUND')),  -- kirim / tarik
    status          VARCHAR(20) NOT NULL DEFAULT 'OPEN' CHECK (status IN (
                        'OPEN','RESERVED','LPB_CREATED','COMPLETED','CANCELLED')),
    scheduled_date  DATE,
    lpb_id          BIGINT REFERENCES lpb_documents(id),
    notes           TEXT,
    created_by      BIGINT NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_do_so ON delivery_orders(so_id);
CREATE INDEX idx_do_wh_status ON delivery_orders(warehouse_id, status);

CREATE TABLE delivery_order_items (
    id           BIGSERIAL PRIMARY KEY,
    do_id        BIGINT NOT NULL REFERENCES delivery_orders(id) ON DELETE CASCADE,
    so_item_id   BIGINT NOT NULL REFERENCES sales_order_items(id),
    product_id   BIGINT NOT NULL REFERENCES products(id),
    qty          INT NOT NULL CHECK (qty > 0)
);

-- Hasil reservasi gudang: per rak (non-SN: qty) atau per unit (SN: product_serial_id, qty=1)
CREATE TABLE delivery_order_reservations (
    id                 BIGSERIAL PRIMARY KEY,
    do_item_id         BIGINT NOT NULL REFERENCES delivery_order_items(id) ON DELETE CASCADE,
    rack_id            BIGINT NOT NULL REFERENCES racks(id),
    product_serial_id  BIGINT REFERENCES product_serials(id),
    qty                INT NOT NULL CHECK (qty > 0),
    reserved_by        BIGINT NOT NULL REFERENCES users(id),
    reserved_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at        TIMESTAMPTZ,                -- diisi bila reservasi dibatalkan
    CONSTRAINT chk_reservation_serial_qty CHECK (product_serial_id IS NULL OR qty = 1)
);
CREATE UNIQUE INDEX uq_active_serial_reservation
    ON delivery_order_reservations(product_serial_id)
    WHERE product_serial_id IS NOT NULL AND released_at IS NULL;

ALTER TABLE lpb_items
    ADD CONSTRAINT fk_lpb_items_do_item FOREIGN KEY (do_item_id) REFERENCES delivery_order_items(id);
ALTER TABLE product_serials
    ADD CONSTRAINT fk_serial_reserved_do FOREIGN KEY (reserved_for_do_id) REFERENCES delivery_orders(id);

CREATE TRIGGER trg_so_updated BEFORE UPDATE ON sales_orders    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_do_updated BEFORE UPDATE ON delivery_orders FOR EACH ROW EXECUTE FUNCTION set_updated_at();
```

#### `migrations/000007_init_billing.up.sql`

```sql
-- 000007_init_billing.up.sql (Fase 2)

-- Tarif & skema PPN berlaku per tanggal (PMK 131/2024: tarif 12% x DPP Nilai Lain 11/12)
CREATE TABLE tax_settings (
    id               BIGSERIAL PRIMARY KEY,
    tax_code         VARCHAR(20) NOT NULL DEFAULT 'PPN',
    rate             NUMERIC(5, 2) NOT NULL,          -- 12.00
    dpp_numerator    INT NOT NULL DEFAULT 11,         -- DPP Nilai Lain = harga x 11/12
    dpp_denominator  INT NOT NULL DEFAULT 12,
    faktur_code      VARCHAR(5) NOT NULL DEFAULT '04',
    effective_from   DATE NOT NULL,
    effective_to     DATE,
    notes            TEXT,
    UNIQUE (tax_code, effective_from)
);

CREATE TABLE invoices (
    id                    BIGSERIAL PRIMARY KEY,
    invoice_number        VARCHAR(50) NOT NULL UNIQUE,
    invoice_type          VARCHAR(20) NOT NULL CHECK (invoice_type IN ('RENTAL','CONSUMABLE','SERVICE','OTHER')),
    customer_id           BIGINT NOT NULL REFERENCES customers(id),
    contract_id           BIGINT REFERENCES rental_contracts(id),
    so_id                 BIGINT REFERENCES sales_orders(id),
    billing_period_start  DATE,
    billing_period_end    DATE,
    invoice_date          DATE NOT NULL,
    due_date              DATE NOT NULL,
    status                VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN (
                              'DRAFT','ISSUED','PARTIALLY_PAID','PAID','VOID')),
    is_ppn                BOOLEAN NOT NULL,
    subtotal              NUMERIC(15, 2) NOT NULL DEFAULT 0,
    discount_amount       NUMERIC(15, 2) NOT NULL DEFAULT 0,
    dpp                   NUMERIC(15, 2) NOT NULL DEFAULT 0,   -- subtotal - diskon
    dpp_other_value       NUMERIC(15, 2) NOT NULL DEFAULT 0,   -- dpp x 11/12 (0 bila non-PPN)
    tax_rate              NUMERIC(5, 2)  NOT NULL DEFAULT 0,   -- snapshot 12.00 / 0
    tax_amount            NUMERIC(15, 2) NOT NULL DEFAULT 0,
    grand_total           NUMERIC(15, 2) NOT NULL DEFAULT 0,
    paid_amount           NUMERIC(15, 2) NOT NULL DEFAULT 0,   -- termasuk PPh 23 yang dipotong
    faktur_pajak_number   VARCHAR(40),
    pdf_path              TEXT,
    notes                 TEXT,
    generated_by_job      BOOLEAN NOT NULL DEFAULT FALSE,
    created_by            BIGINT REFERENCES users(id),
    issued_by             BIGINT REFERENCES users(id),
    issued_at             TIMESTAMPTZ,
    voided_by             BIGINT REFERENCES users(id),
    voided_at             TIMESTAMPTZ,
    void_reason           TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_invoice_period CHECK (
        invoice_type <> 'RENTAL'
        OR (contract_id IS NOT NULL AND billing_period_start IS NOT NULL
            AND billing_period_end >= billing_period_start))
);
-- Kunci anti tagihan ganda (blueprint): satu invoice RENTAL aktif per kontrak per periode
CREATE UNIQUE INDEX uq_invoice_rental_period
    ON invoices(contract_id, billing_period_start, billing_period_end)
    WHERE invoice_type = 'RENTAL' AND status <> 'VOID';
CREATE INDEX idx_invoices_customer ON invoices(customer_id, status);
CREATE INDEX idx_invoices_due      ON invoices(due_date) WHERE status IN ('ISSUED','PARTIALLY_PAID');

CREATE TABLE invoice_items (
    id                 BIGSERIAL PRIMARY KEY,
    invoice_id         BIGINT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    description        TEXT NOT NULL,
    product_id         BIGINT REFERENCES products(id),
    product_serial_id  BIGINT REFERENCES product_serials(id),
    customer_site_id   BIGINT REFERENCES customer_sites(id),
    rental_placement_id BIGINT REFERENCES rental_placements(id),
    period_start       DATE,
    period_end         DATE,
    days_billed        INT,
    days_in_period     INT,
    qty                NUMERIC(12, 4) NOT NULL DEFAULT 1,   -- faktor prorata utk sewa (mis. 0.5333)
    unit_price         NUMERIC(15, 2) NOT NULL,
    line_total         NUMERIC(15, 2) NOT NULL
);
CREATE INDEX idx_invoice_items_invoice ON invoice_items(invoice_id);

CREATE TABLE payments (
    id                 BIGSERIAL PRIMARY KEY,
    invoice_id         BIGINT NOT NULL REFERENCES invoices(id),
    payment_date       DATE NOT NULL,
    amount_received    NUMERIC(15, 2) NOT NULL CHECK (amount_received >= 0),  -- uang masuk
    pph23_amount       NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (pph23_amount >= 0),
    pph23_slip_number  VARCHAR(60),                                       -- no. bukti potong
    method             VARCHAR(20) NOT NULL CHECK (method IN ('TRANSFER','CASH','GIRO','OTHER')),
    reference_number   VARCHAR(80),
    proof_path         TEXT,
    notes              TEXT,
    recorded_by        BIGINT NOT NULL REFERENCES users(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_payment_positive CHECK (amount_received + pph23_amount > 0)
);
CREATE INDEX idx_payments_invoice ON payments(invoice_id);

CREATE TRIGGER trg_invoices_updated BEFORE UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION set_updated_at();
```

#### `migrations/000008_init_tracking_tickets.up.sql`

```sql
-- 000008_init_tracking_tickets.up.sql (Fase 3)

CREATE TABLE shipment_tracking_logs (
    id               BIGSERIAL PRIMARY KEY,
    lpb_id           BIGINT NOT NULL REFERENCES lpb_documents(id),
    driver_id        BIGINT NOT NULL REFERENCES users(id),
    status           VARCHAR(20) NOT NULL CHECK (status IN (
                         'PICKED_UP','IN_TRANSIT','ARRIVED','DELIVERED','FAILED')),
    latitude         DECIMAL(10, 7) NOT NULL,
    longitude        DECIMAL(10, 7) NOT NULL,
    accuracy_m       INT,
    recorded_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),   -- waktu server
    client_time      TIMESTAMPTZ,                          -- waktu HP (bisa offline)
    client_event_id  UUID UNIQUE,                          -- idempotensi retry dari HP
    receiver_name    VARCHAR(100),
    photo_path       TEXT,
    failure_reason   TEXT,
    notes            TEXT
);
CREATE INDEX idx_tracking_lpb ON shipment_tracking_logs(lpb_id, recorded_at);

CREATE TABLE maintenance_tickets (
    id                 BIGSERIAL PRIMARY KEY,
    ticket_number      VARCHAR(50) NOT NULL UNIQUE,
    customer_id        BIGINT NOT NULL REFERENCES customers(id),
    customer_site_id   BIGINT NOT NULL REFERENCES customer_sites(id),
    product_serial_id  BIGINT NOT NULL REFERENCES product_serials(id),
    reported_by_name   VARCHAR(100),
    reported_via       VARCHAR(20) NOT NULL DEFAULT 'PHONE' CHECK (reported_via IN (
                           'PHONE','WHATSAPP','EMAIL','PORTAL','INTERNAL')),
    category           VARCHAR(30) NOT NULL DEFAULT 'OTHER' CHECK (category IN (
                           'PAPER_JAM','PRINT_QUALITY','HARDWARE','SOFTWARE','NETWORK','PREVENTIVE','OTHER')),
    priority           VARCHAR(10) NOT NULL DEFAULT 'MEDIUM' CHECK (priority IN ('LOW','MEDIUM','HIGH','CRITICAL')),
    status             VARCHAR(20) NOT NULL DEFAULT 'OPEN' CHECK (status IN (
                           'OPEN','ASSIGNED','IN_PROGRESS','WAITING_PART','RESOLVED','CLOSED','CANCELLED')),
    title              VARCHAR(200) NOT NULL,
    description        TEXT,
    assigned_to        BIGINT REFERENCES users(id),
    sla_due_at         TIMESTAMPTZ,
    resolution_notes   TEXT,
    resolved_at        TIMESTAMPTZ,
    closed_at          TIMESTAMPTZ,
    swap_so_id         BIGINT REFERENCES sales_orders(id),     -- eskalasi UNIT_SWAP
    created_by         BIGINT NOT NULL REFERENCES users(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_tickets_status   ON maintenance_tickets(status, assigned_to);
CREATE INDEX idx_tickets_serial   ON maintenance_tickets(product_serial_id);

CREATE TABLE ticket_spareparts (
    id                 BIGSERIAL PRIMARY KEY,
    ticket_id          BIGINT NOT NULL REFERENCES maintenance_tickets(id) ON DELETE CASCADE,
    product_id         BIGINT NOT NULL REFERENCES products(id),
    qty                INT NOT NULL CHECK (qty > 0),
    status             VARCHAR(20) NOT NULL DEFAULT 'REQUESTED' CHECK (status IN (
                           'REQUESTED','LPB_CREATED','ISSUED','REJECTED','CANCELLED')),
    lpb_id             BIGINT REFERENCES lpb_documents(id),    -- LPB OUTBOUND SPARE_PART_ISSUE
    is_billable        BOOLEAN NOT NULL DEFAULT FALSE,         -- ditagihkan ke pelanggan?
    requested_by       BIGINT NOT NULL REFERENCES users(id),
    requested_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes              TEXT
);

CREATE TABLE ticket_activities (
    id             BIGSERIAL PRIMARY KEY,
    ticket_id      BIGINT NOT NULL REFERENCES maintenance_tickets(id) ON DELETE CASCADE,
    user_id        BIGINT NOT NULL REFERENCES users(id),
    activity_type  VARCHAR(20) NOT NULL CHECK (activity_type IN (
                       'COMMENT','STATUS_CHANGE','ASSIGN','VISIT_CHECKIN','VISIT_CHECKOUT','PHOTO')),
    message        TEXT,
    from_status    VARCHAR(20),
    to_status      VARCHAR(20),
    latitude       DECIMAL(10, 7),
    longitude      DECIMAL(10, 7),
    photo_path     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_ticket_activities_ticket ON ticket_activities(ticket_id, created_at);

ALTER TABLE sales_orders
    ADD CONSTRAINT fk_so_ticket FOREIGN KEY (related_ticket_id) REFERENCES maintenance_tickets(id);

CREATE TRIGGER trg_tickets_updated BEFORE UPDATE ON maintenance_tickets FOR EACH ROW EXECUTE FUNCTION set_updated_at();
```

#### `migrations/000002_init_audit_settings.up.sql`

```sql
-- 000002_init_audit_settings.up.sql

CREATE TABLE audit_logs (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT REFERENCES users(id),
    action       VARCHAR(60) NOT NULL,      -- LPB_APPROVE, USER_CREATE, INVOICE_VOID, LOGIN_FAILED, ...
    entity_type  VARCHAR(40),
    entity_id    BIGINT,
    before_data  JSONB,
    after_data   JSONB,
    ip_address   INET,
    user_agent   TEXT,
    request_id   VARCHAR(64),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX idx_audit_user   ON audit_logs(user_id, created_at);
CREATE TRIGGER trg_audit_append_only
    BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION forbid_update_delete();

CREATE TABLE app_settings (
    key         VARCHAR(80) PRIMARY KEY,   -- company.profile, lpb.allow_self_approve, billing.default_mode, ...
    value       JSONB NOT NULL,
    updated_by  BIGINT REFERENCES users(id),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE attachments (
    id           BIGSERIAL PRIMARY KEY,
    entity_type  VARCHAR(40) NOT NULL,
    entity_id    BIGINT NOT NULL,
    file_path    TEXT NOT NULL,
    file_name    VARCHAR(200),
    mime_type    VARCHAR(100) NOT NULL,
    size_bytes   BIGINT NOT NULL,
    uploaded_by  BIGINT REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_attachments_entity ON attachments(entity_type, entity_id);

-- Idempotency untuk endpoint kritis (approve LPB, deliver POD, record payment)
CREATE TABLE idempotency_keys (
    idem_key       VARCHAR(100) NOT NULL,
    user_id        BIGINT NOT NULL REFERENCES users(id),
    endpoint       VARCHAR(150) NOT NULL,
    status_code    INT,
    response_body  JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (idem_key, user_id)
);

CREATE TABLE job_runs (
    id           BIGSERIAL PRIMARY KEY,
    job_name     VARCHAR(60) NOT NULL,      -- BILLING_MONTHLY, LOW_STOCK_ALERT, STOCK_RECONCILE, CONTRACT_EXPIRY
    run_key      VARCHAR(60) NOT NULL,      -- mis. 2026-11-21
    status       VARCHAR(20) NOT NULL CHECK (status IN ('RUNNING','SUCCESS','FAILED')),
    summary      JSONB,
    error        TEXT,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at  TIMESTAMPTZ,
    UNIQUE (job_name, run_key)
);
```
#### Seed wajib (`cmd/seed`)

```sql
-- Tarif PPN berlaku sejak 1 Jan 2025 (PMK 131/2024): 12% x DPP Nilai Lain 11/12
INSERT INTO tax_settings (tax_code, rate, dpp_numerator, dpp_denominator, faktur_code, effective_from, notes)
VALUES ('PPN', 12.00, 11, 12, '04', '2025-01-01', 'PMK 131/2024 - BKP/JKP non-mewah');

INSERT INTO app_settings (key, value) VALUES
('company.profile', '{"name":"PT Contoh Rental","address":"Semarang","phone":"024-xxx","email":"admin@contoh.id","npwp":""}'),
('lpb.allow_self_approve', 'false'),
('billing.run_day_offset', '0'),
('billing.rounding', '"HALF_UP"'),
('pdf.verify_base_url', '"https://erp.contoh.id/verify/lpb"');
-- + 1 user ADMIN awal (password dari env SEED_ADMIN_PASSWORD, must_change_password = TRUE)
-- + kategori standar: Printer, PC, Monitor (ASSET); Toner, Tinta, Kertas, Sparepart (CONSUMABLE)
```

---

## 5. Aturan Bisnis & State Machine

### 5.1 Matriks Hak Akses (RBAC) per Aksi

Legenda: **C** buat/ubah · **R** lihat · **S** submit · **A** approve/finalisasi · **X** aksi khusus · `-` tidak ada akses.
`ADMIN` punya semua akses. Role warehouse dibatasi ke `users.warehouse_id` miliknya (*warehouse scoping*), kecuali `warehouse_id` NULL.

| Modul / Aksi | MGMT | BIZ_ADMIN | STAFF_WH | HEAD_WH | DRIVER | TECH |
|---|---|---|---|---|---|---|
| Master gudang, rak, kategori, produk, supplier | R | R | C (rak) / R | C | - | R produk |
| Saldo stok, cari SN (scan) | R | R | R | R | - | R |
| Buku besar / kartu stok | R | - | R | R | - | - |
| LPB draft, scan SN, submit | R | - | C,S | C,S | - | - |
| LPB approve / reject | R | - | - | **A** | - | - |
| Cetak PDF Surat Jalan | R | R | R | R | R (tugasnya) | - |
| Stock opname: mulai & hitung | R | - | C | C | - | - |
| Stock opname: finalisasi | R | - | - | **A** | - | - |
| Pelanggan & site | R | C | - | - | - | R |
| Kontrak sewa (buat/aktifkan/terminasi) | R | C,A | - | - | - | - |
| SO (buat/konfirmasi/batal) | R | C,A | - | - | - | - |
| DO terbitkan | R | C | R | R | - | - |
| DO reserve stok → draft LPB | R | R | X | X | - | - |
| Assign driver & kendaraan | R | C | C | C | - | - |
| Tugas antar, checkpoint GPS, POD | R | R | - | R | X (miliknya) | - |
| Invoice generate/issue/void, catat bayar | R | C,A | - | - | - | - |
| Tiket maintenance buat | R | C | - | - | - | C |
| Tiket kerjakan, minta sparepart, rekomendasi swap | R | R | - | - | - | X |
| Peta sebaran aset | R | R | - | R | - | R |
| Dashboard | R | R (komersial) | R (gudang) | R (gudang) | - | - |
| User & role, app settings | - | - | - | - | - | - |

> Implementasi: tabel ini dikodekan sebagai map `permission → []role` di `internal/domain/permission.go` dan dipakai oleh middleware `RequirePermission("lpb.approve")`. Lebih fleksibel daripada `RequireRoles(...)` tersebar di router, dan FE memakai daftar permission yang sama dari `GET /auth/me`.

### 5.2 Matriks Tipe LPB → Efek saat APPROVED

| Sub-type | Arah | Sumber dokumen | Efek `stock_balances` | Efek `product_serials` | Pengiriman |
|---|---|---|---|---|---|
| `PURCHASE` | IN | Manual staff (+ supplier) | `+qty` rak tujuan (upsert) | SN **baru** dibuat; status `AVAILABLE` (kondisi BROKEN → `BROKEN`, rak wajib non-sellable); error jika SN sudah ada | - |
| `INITIAL_STOCK` | IN | Import data awal (migrasi dari pencatatan manual) | `+qty` | SN baru `AVAILABLE` | - |
| `RENTAL_RETURN` | IN | DO INBOUND (penarikan / sisi lama swap) | `+qty` rak tujuan (disarankan KARANTINA) | `IN_TRANSIT` → `MAINTENANCE` (perlu QC) atau `AVAILABLE` bila kondisi GOOD & rak sellable; kosongkan customer/site/contract | Penjemputan oleh driver |
| `FAILED_DELIVERY_RETURN` | IN | Otomatis dari LPB OUT yang delivery `FAILED` | `+qty` ke rak asal | `IN_TRANSIT` → `AVAILABLE` | - |
| `TRANSFER_IN` | IN | Pasangan otomatis `TRANSFER_OUT` | `+qty` rak tujuan | `IN_TRANSIT` → `AVAILABLE` di rak tujuan | - |
| `OPNAME_ADJUSTMENT_IN` | IN | Finalisasi opname (selisih +) | `+selisih` | SN `UNEXPECTED` didaftarkan / `LOST` → `AVAILABLE` | - |
| `RENTAL_DELIVERY` | OUT | DO OUTBOUND (`NEW_DELIVERY`, sisi baru `UNIT_SWAP`) | `−qty` dan `−qty_reserved` rak asal | `RESERVED` → `IN_TRANSIT` (customer tujuan diisi); → `RENTED` saat POD | Ya (driver) |
| `CONSUMABLE_DELIVERY` | OUT | DO OUTBOUND (`CONSUMABLE`) | `−qty`, `−qty_reserved` | - (non-SN) | Ya |
| `SPARE_PART_ISSUE` | OUT | `ticket_spareparts` | `−qty` | Bila sparepart ber-SN: `AVAILABLE` → `RETIRED` (terpasang di unit) | Dibawa teknisi |
| `TRANSFER_OUT` | OUT | Manual (pindah rak / antar gudang) | `−qty` rak asal | `AVAILABLE` → `IN_TRANSIT` | Opsional |
| `OPNAME_ADJUSTMENT_OUT` | OUT | Finalisasi opname (selisih −) | `−selisih` | SN `MISSING` → `LOST` | - |
| `DISPOSAL` | OUT | Manual (unit rusak total / dijual scrap) | `−qty` | `BROKEN` → `RETIRED` | - |

**Aturan transfer:**
- *Pindah rak dalam gudang yang sama*: approve `TRANSFER_OUT` **otomatis meng-approve** pasangan `TRANSFER_IN` dalam transaksi yang sama (barang tidak benar-benar "di jalan").
- *Antar gudang*: `TRANSFER_IN` di-approve Kepala Warehouse gudang tujuan setelah barang tiba dan discan ulang.

### 5.3 State Machine

**LPB (`lpb_documents.status`)**
```
DRAFT ──submit──> PENDING_APPROVAL ──approve──> APPROVED   (mutasi stok + ledger + verify_token)
  ▲  │                    │
  │  └──cancel──> CANCELLED│──reject(alasan)──> REJECTED ──revisi──> DRAFT
  └──────────────────────────────────────────────────────────────────┘
```
- Edit item/scan hanya saat `DRAFT`. APPROVED bersifat final: koreksi dilakukan dengan LPB baru (bukan edit).
- Reject wajib alasan. Revisi = status kembali `DRAFT` (dokumen sama, histori di `audit_logs`).
- Cancel LPB yang berasal dari DO → reservasi DO **tidak** dilepas otomatis (staf bisa buat ulang LPB); lepas reservasi lewat aksi DO "release".

**Delivery (`lpb_documents.delivery_status`, hanya LPB OUT `requires_delivery`)**
```
(APPROVED) READY ─> PICKED_UP ─> IN_TRANSIT ─> ARRIVED ─> DELIVERED   (SN → RENTED, buat rental_placements)
                         └──────────────┴───────────┴────> FAILED ─> RETURNED (LPB FAILED_DELIVERY_RETURN approved)
```

**Product Serial (`product_serials.status`)**
```
            PURCHASE/INITIAL
                  │
                  ▼
 ┌──────────> AVAILABLE ──DO reserve──> RESERVED ──LPB OUT approved──> IN_TRANSIT ──POD──> RENTED
 │                ▲  ▲                     │ release                        │ FAILED           │
 │                │  └─────────────────────┘                               ▼                  │ pickup
 │                │                                   (FAILED_DELIVERY_RETURN) AVAILABLE     ▼
 │                └──── QC OK ──── MAINTENANCE <──── RENTAL_RETURN approved ──────── IN_TRANSIT
 │                                     │
 │                                     └── rusak berat ──> BROKEN ──DISPOSAL──> RETIRED
 └── opname ditemukan lagi ── LOST <── opname MISSING
```
Setiap perubahan status **wajib** menulis 1 baris `product_serial_histories`.

**Sales Order:** `DRAFT → CONFIRMED → IN_PROGRESS (DO pertama dibuat) → COMPLETED (semua DO COMPLETED) | CANCELLED` (cancel hanya jika belum ada LPB APPROVED).
**Delivery Order:** `OPEN → RESERVED → LPB_CREATED → COMPLETED (LPB APPROVED + DELIVERED / approved retur) | CANCELLED (reservasi dilepas)`.
**Kontrak:** `DRAFT → ACTIVE (punya ≥1 item, tanggal valid) → EXPIRED (job harian, end_date lewat) | TERMINATED (manual + alasan)`. Unit masih terpasang saat kontrak berakhir → muncul di alert dashboard.
**Invoice:** `DRAFT → ISSUED → PARTIALLY_PAID → PAID`, `DRAFT/ISSUED → VOID` (alasan wajib; invoice yang sudah ada pembayaran tidak bisa VOID). `OVERDUE` = kondisi turunan (`due_date < today` & belum PAID), tidak disimpan.
**Tiket:** `OPEN → ASSIGNED → IN_PROGRESS ⇄ WAITING_PART → RESOLVED → CLOSED`, `* → CANCELLED`. Eskalasi swap membuat SO `UNIT_SWAP` dan mengisi `swap_so_id`.
**Stock Opname:** `IN_PROGRESS (rak dikunci) → SUBMITTED → FINALIZED (LPB adjustment dibuat & di-approve, rak dibuka) | CANCELLED (rak dibuka)`.

### 5.4 Validasi LPB

**Saat simpan draft / scan SN**
- Produk SN: setiap scan → `GET /inventory/serials/scan/:code` (asset tag atau SN).
  - OUTBOUND: SN harus ada, `current_rack_id` = rak item, status `AVAILABLE` (atau `RESERVED` untuk DO yang sama).
  - INBOUND PURCHASE: SN **tidak boleh** sudah ada di `product_serials` (per produk).
  - INBOUND RENTAL_RETURN: SN harus status `RENTED`/`IN_TRANSIT` milik customer dokumen.
- SN yang sama tidak boleh ada di LPB lain yang masih `DRAFT/PENDING_APPROVAL` (cek di usecase → error `ErrSerialInOtherLPB`).
- Rak yang sedang opname (`locked_by_opname_id` NOT NULL) tidak boleh dipakai.

**Saat submit**: minimal 1 item; produk SN: `jumlah scan = qty`; non-SN OUTBOUND: `qty ≤ available` (peringatan dini; validasi final tetap saat approve).

**Saat approve** (semua dalam 1 transaksi, lihat Bagian 6.2): status `PENDING_APPROVAL`; approver punya role HEAD_WAREHOUSE di gudang yang sama; `approver ≠ creator` kecuali `lpb.allow_self_approve = true`; validasi ulang semua aturan di atas dengan row lock.

### 5.5 Aturan Reservasi DO
- Reservasi hanya dari rak `is_sellable = TRUE` dan tidak terkunci opname.
- Produk SN: staf **scan unit** yang akan dikirim → SN `AVAILABLE` → `RESERVED`, `reserved_for_do_id` diisi, `qty_reserved += 1` di raknya.
- Non-SN: sistem menyarankan rak (urut `qty_available` terbesar), staf boleh ubah. `qty_reserved += qty`.
- Reservasi selesai → status DO `RESERVED` → tombol "Buat Draft LPB" (item & SN terisi otomatis dari reservasi).
- **Release** reservasi (DO dibatalkan): kembalikan `qty_reserved`, SN → `AVAILABLE`, isi `released_at`.
- 💡 Reservasi kedaluwarsa: job harian memberi peringatan DO `RESERVED` > N hari tanpa LPB (tidak otomatis dilepas).

### 5.6 Perhitungan Harga, PPN & PPh 23

**Rumus (dengan `tax_settings` aktif: rate 12, numerator 11, denominator 12):**
```
subtotal        = Σ line_total
dpp             = subtotal − discount_amount
dpp_other_value = round(dpp × 11 / 12)           ← DPP Nilai Lain (0 bila non-PPN)
tax_amount      = round(dpp_other_value × 12%)   ← PPN (0 bila non-PPN)
grand_total     = dpp + tax_amount
```
**Contoh:** 10 printer × Rp500.000 = subtotal Rp5.000.000 → DPP Nilai Lain Rp4.583.333 → PPN 12% = Rp550.000 (sama dengan 11% × Rp5.000.000) → grand total Rp5.550.000.

- Pembulatan: `HALF_UP` ke rupiah penuh untuk `dpp_other_value` dan `tax_amount` (konfigurasi `billing.rounding`). Semua di `pkg/money` memakai `shopspring/decimal`, **dilarang float64**.
- Tarif disnapshot ke invoice (`tax_rate`), jadi perubahan aturan pajak berikutnya cukup menambah baris `tax_settings` baru dengan `effective_from`.
- `is_ppn` diambil dari kontrak (sewa) atau SO (consumable). Tampilkan DPP, DPP Nilai Lain, tarif, dan PPN di PDF invoice.
- **PPh 23**: pelanggan badan biasanya memotong PPh 23 atas sewa peralatan. Saat catat pembayaran: `amount_received` (uang masuk) + `pph23_amount` (dipotong) = pengurang tagihan. `paid_amount += amount_received + pph23_amount`. Simpan nomor bukti potong. Tarif & perlakuan pastikan ke tim finance/konsultan pajak.
- Faktur pajak Coretax: Fase 2 cukup simpan `faktur_pajak_number` manual. 💡 Backlog: export XML/Excel format impor Coretax.

### 5.7 Aturan Tagihan Bulanan (MVP: tagih di belakang / arrears)
- **Periode** ditentukan `billing_day_of_month = D`: periode = tanggal D bulan lalu s/d D−1 bulan ini (D=1 → tanggal 1 s/d akhir bulan lalu). Dihitung dalam WIB.
- Job `BILLING_MONTHLY` berjalan harian; untuk setiap kontrak `ACTIVE`/`EXPIRED`/`TERMINATED` yang periodenya **baru berakhir** dan belum punya invoice periode itu → buat invoice `DRAFT` (atau langsung `ISSUED` via setting).
- **Baris invoice** = setiap `rental_placements` yang beririsan dengan periode. Hari tagih = irisan interval `[start_date, end_date)` dengan `[period_start, period_end + 1)`.
  - `PRORATA`: `line_total = round(monthly_rate × hari_tagih / hari_dalam_periode, 2)`.
  - `FULL_MONTH`: `line_total = monthly_rate` bila hari_tagih > 0.
  - Contoh periode 1–30 Nov (30 hari, Rp500.000/bulan): terpasang penuh → Rp500.000; dikirim 16 Nov → 15 hari → Rp250.000; ditarik 10 Nov → 9 hari → Rp150.000. (Sudah diuji dengan query SQL di Bagian 6.5.)
- Idempotensi dijamin `uq_invoice_rental_period` + `job_runs (job_name, run_key)`.
- Invoice consumable dibuat saat SO `CONSUMABLE` selesai dikirim (DELIVERED).
- Sparepart tiket `is_billable = TRUE` → masuk invoice `SERVICE`.

### 5.8 Aturan Pengiriman & POD
- Driver hanya melihat LPB dengan `driver_id = dirinya`, status APPROVED, delivery belum selesai.
- Checkpoint wajib GPS; `client_event_id` (UUID dari HP) mencegah duplikasi saat retry sinyal buruk.
- `DELIVERED` wajib: foto POD, nama penerima, GPS. 💡 Peringatan (bukan blokir) bila jarak GPS ke koordinat site > 500 m.
- Saat `DELIVERED` (1 transaksi): log tracking, `delivery_status`, `delivered_at`, SN `IN_TRANSIT → RENTED` (+ customer/site/contract), insert `rental_placements(start_date = tanggal POD WIB)`, update DO → `COMPLETED`, SO → `COMPLETED` bila semua DO selesai.
- Penjemputan (penarikan/swap): saat driver `PICKED_UP` di lokasi pelanggan → SN `RENTED → IN_TRANSIT`, `rental_placements.end_date = tanggal pickup` (tagihan berhenti). Saat tiba di gudang → LPB `RENTAL_RETURN` di-approve → stok masuk.
- `FAILED` wajib alasan → sistem membuat draft LPB `FAILED_DELIVERY_RETURN` untuk di-approve saat barang kembali.

### 5.9 Invariant & Rekonsiliasi (job harian `STOCK_RECONCILE` + endpoint admin)

```sql
-- I1: saldo rak produk SN = jumlah unit fisik di rak itu
SELECT sb.product_id, sb.rack_id, sb.qty_on_hand, COUNT(ps.id) AS serial_count
FROM stock_balances sb
JOIN products p ON p.id = sb.product_id AND p.has_serial_number
LEFT JOIN product_serials ps ON ps.product_id = sb.product_id AND ps.current_rack_id = sb.rack_id
     AND ps.status IN ('AVAILABLE','RESERVED','MAINTENANCE','BROKEN')
GROUP BY sb.product_id, sb.rack_id, sb.qty_on_hand
HAVING sb.qty_on_hand <> COUNT(ps.id);

-- I2: qty_reserved SN = jumlah unit RESERVED di rak itu
-- I3: saldo rak = Σ ledger IN − Σ ledger OUT (per produk, rak)
SELECT sb.product_id, sb.rack_id, sb.qty_on_hand,
       COALESCE(SUM(CASE WHEN l.direction='IN' THEN l.qty ELSE -l.qty END),0) AS ledger_qty
FROM stock_balances sb
LEFT JOIN stock_ledgers l ON l.product_id = sb.product_id AND l.rack_id = sb.rack_id
GROUP BY sb.product_id, sb.rack_id, sb.qty_on_hand
HAVING sb.qty_on_hand <> COALESCE(SUM(CASE WHEN l.direction='IN' THEN l.qty ELSE -l.qty END),0);

-- I4: setiap SN RENTED punya tepat 1 rental_placements aktif (end_date NULL)
-- I5: invoice.paid_amount = Σ payments(amount_received + pph23_amount)
```
Hasil ≠ 0 baris → catat di `job_runs.summary` + tampil di dashboard Admin. **Jangan auto-fix**; koreksi lewat LPB adjustment.

---

## 6. Algoritma Transaksi Inti

> Prinsip umum semua operasi yang mengubah stok/status:
> 1. Satu usecase = satu `txManager.WithinTransaction(...)`.
> 2. **Urutan lock tetap** untuk mencegah deadlock: dokumen header (`FOR UPDATE`) → `stock_balances` urut `(product_id, rack_id)` → `product_serials` urut `id`.
> 3. Validasi bisnis dilakukan **setelah** lock (data yang dicek = data terkunci).
> 4. Error domain = sentinel error di `internal/domain/errors.go`; transaksi otomatis rollback.
> 5. Tulis `audit_logs` di dalam transaksi yang sama.

### 6.1 Pelengkap Kode Blueprint yang Terpotong

**`pkg/database/executor.go`** (dipakai repository; blueprint memanggil `database.GetExecutor` tapi definisinya terpotong)
```go
package database

import (
    "context"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

// GetExecutor mengembalikan tx aktif dari context bila ada, selain itu pool.
func GetExecutor(ctx context.Context, pool *pgxpool.Pool) DBTX {
    if tx, ok := ctx.Value(txCtxKey{}).(pgx.Tx); ok {
        return tx
    }
    return pool
}

// MustInTx dipakai repository yang WAJIB dipanggil di dalam transaksi (method Lock*).
func MustInTx(ctx context.Context) pgx.Tx {
    tx, ok := ctx.Value(txCtxKey{}).(pgx.Tx)
    if !ok {
        panic("database: method ini wajib dipanggil di dalam WithinTransaction")
    }
    return tx
}
```
Bagian akhir `WithinTransaction` di blueprint terpotong — penutupnya cukup `return nil` setelah commit sukses.

**`pkg/docnum` — nomor dokumen** (atomik, tanpa celah karena ikut rollback bila transaksi gagal)
```go
// Format: {PREFIX}/{SCOPE}/{YYYY}/{MM}/{NNNN}  contoh: LPBO/WH-SMG-01/2026/11/0007
func (r *SequenceRepo) Next(ctx context.Context, docType, scope string, t time.Time) (string, error) {
    db := database.GetExecutor(ctx, r.pool)
    period := t.In(clock.WIB).Format("200601")
    var n int
    err := db.QueryRow(ctx, `
        INSERT INTO document_sequences (doc_type, scope_key, period, last_number)
        VALUES ($1, $2, $3, 1)
        ON CONFLICT (doc_type, scope_key, period)
        DO UPDATE SET last_number = document_sequences.last_number + 1
        RETURNING last_number`, docType, scope, period).Scan(&n)
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("%s/%s/%s/%s/%04d", docType, scope, period[:4], period[4:], n), nil
}
```
Prefix: `LPBI` (LPB masuk), `LPBO` (LPB keluar), `SO`, `DO`, `INV`, `OPN`, `TKT`, `KTR` (kontrak), `AST` (asset tag, scope `GLOBAL`, period `000000`).

### 6.2 `ApproveLPB(lpbID, approverID)` — versi lengkap

```
WithinTransaction:
  1. lpb := LockLPBForUpdate(lpbID)                       -- SELECT ... FOR UPDATE
     if not found            → ErrLPBNotFound
     if status != PENDING    → ErrLPBNotPending
     if approver == creator && !setting(allow_self_approve) → ErrSelfApproveForbidden
     if approver.warehouse_id != nil && != lpb.warehouse_id → ErrForbiddenWarehouse
  2. items := GetItemsWithSerials(lpb.ID); if empty → ErrLPBEmptyItems
     sort items by (ProductID, RackID)
  3. cek rak tidak terkunci opname (kecuali sub_type OPNAME_ADJUSTMENT_*) → ErrRackLocked
  4. for each item:
       if product.has_sn && len(serials) != qty → ErrSerialCountMismatch
       switch lpb.Type:
       case INBOUND:
         before, after := UpsertAddStockBalance(product, rack, qty)   -- INSERT .. ON CONFLICT DO UPDATE .. RETURNING
         if has_sn:
            serials sort by serial_number
            PURCHASE / INITIAL_STOCK:
               for sn: if ExistsSerial(product, sn) → ErrSerialAlreadyExists
                       assetTag := docnum.Next("AST", "GLOBAL", ...)
                       id := InsertSerial(status = AVAILABLE|BROKEN by condition, rack, warehouse)
                       UpdateLPBItemSerial(set product_serial_id = id)
            RENTAL_RETURN / FAILED_DELIVERY_RETURN / TRANSFER_IN / OPNAME_ADJUSTMENT_IN:
               rows := LockSerialsForUpdate(ids sorted)                -- FOR UPDATE
               validasi status asal sesuai Bagian 5.2 → ErrSerialNotValid
               status baru = AVAILABLE | MAINTENANCE (RENTAL_RETURN: kondisi != GOOD atau rak non-sellable)
               UpdateSerialLocation(ids, status, rack, warehouse, customer=NULL, contract=NULL, reserved_for_do=NULL)
       case OUTBOUND:
         bal := LockStockBalance(product, rack)                        -- FOR UPDATE; tidak ada → ErrInsufficientStock
         fromDO := lpb.ReferenceType == DELIVERY_ORDER
         if fromDO:   need reserved ≥ qty (reservasi milik DO ini)       → ErrReservationMismatch
                      newReserved = bal.reserved − qty
         else:        need (bal.on_hand − bal.reserved) ≥ qty          → ErrInsufficientStock
                      newReserved = bal.reserved
         newOnHand = bal.on_hand − qty
         UpdateStockBalance(bal.ID, newOnHand, newReserved)            -- satu UPDATE, aman terhadap CHECK
         if has_sn:
            rows := LockSerialsForUpdate(ids sorted)
            validasi: rack = item.rack, status = (fromDO ? RESERVED & reserved_for_do_id = DO : AVAILABLE)
            status baru: RENTAL_DELIVERY → IN_TRANSIT (customer/site tujuan diisi)
                         TRANSFER_OUT     → IN_TRANSIT
                         SPARE_PART_ISSUE → RETIRED
                         DISPOSAL         → RETIRED (asal harus BROKEN)
                         OPNAME_ADJ_OUT   → LOST
            UpdateSerialLocation(... rack = NULL ...)
       InsertLedger(direction, qty, before, after, movement_type = sub_type, serial_ids, created_by = approver)
       InsertSerialHistories(event = "LPB_APPROVED", ref = LPB)
  5. if fromDO: reservasi DO dianggap terpakai (qty_reserved sudah dikurangi di langkah 4);
               status DO tetap LPB_CREATED sampai DELIVERED (Bagian 6.4)
  6. if sub_type == TRANSFER_OUT && counterpart same warehouse: approve counterpart TRANSFER_IN (rekursif, tx sama)
  7. lpb.status = APPROVED, approved_by/at, verify_token = random 32 byte base64url
     if requires_delivery: delivery_status = READY
  8. InsertAuditLog("LPB_APPROVE", before, after)
```
**Pemetaan HTTP** (di `httperr`): NotFound→404; NotPending/InsufficientStock/SerialAlreadyExists/ReservationMismatch/RackLocked→409; SerialCountMismatch/SerialNotValid/EmptyItems→422; SelfApproveForbidden/ForbiddenWarehouse→403.

### 6.3 `ReserveDO(doID, reservations[])` dan `CreateLPBFromDO(doID)`
```
ReserveDO (STAFF/HEAD WH), tx:
  do := LockDO(doID); status harus OPEN atau RESERVED (tambah reservasi)
  group input per (product, rack) sort
  for each (product, rack, qty, serialIDs?):
     rack harus sellable & tidak terkunci
     bal := LockStockBalance; available = on_hand − reserved ≥ qty  else ErrInsufficientStock
     UpdateStockBalance(on_hand, reserved + qty)
     if SN: LockSerials; status AVAILABLE & rack cocok → RESERVED, reserved_for_do_id = do
     InsertReservation(...); InsertSerialHistories("DO_RESERVED")
  total reservasi per do_item harus ≤ qty do_item; bila semua item terpenuhi → DO.status = RESERVED

CreateLPBFromDO, tx:
  do RESERVED → buat LPB DRAFT (OUTBOUND RENTAL_DELIVERY / CONSUMABLE_DELIVERY,
     reference = DELIVERY_ORDER, customer/site dari SO, requires_delivery = TRUE)
  item & lpb_item_serials diisi dari reservasi aktif; DO.status = LPB_CREATED, DO.lpb_id = lpb
```

### 6.4 `DeliverLPB` (POD) & `PickupForReturn`
```
DeliverLPB(lpbID, driverID, lat, lng, receiver, photo, clientEventID), tx:
  idempotensi: bila clientEventID sudah ada di shipment_tracking_logs → kembalikan hasil lama
  lpb := LockLPB; driver_id == driverID; status APPROVED; delivery_status ∈ {PICKED_UP, IN_TRANSIT, ARRIVED}
  photoPath := storage.Upload(...)     ← lakukan SEBELUM tx dimulai agar tx singkat; hapus file bila tx gagal
  InsertTrackingLog(DELIVERED, ...)
  lpb.delivery_status = DELIVERED, delivered_at, receiver_name, pod_photo_path
  for SN pada LPB (lock urut id): IN_TRANSIT → RENTED, customer/site/contract dari SO
      InsertRentalPlacement(contract, contract_item (cocokkan product+site), serial, site,
                            monthly_rate = contract_item.monthly_rate, start_date = today WIB, start_lpb = lpb)
      InsertSerialHistory("DELIVERED")
  DO → COMPLETED; SO → COMPLETED bila semua DO COMPLETED
  (consumable) → buat invoice CONSUMABLE DRAFT dari SO

PickupForReturn(lpbID [RENTAL_RETURN draft/pending], driverID, gps), tx:
  untuk setiap SN: RENTED → IN_TRANSIT; placement aktif: end_date = today WIB, end_reason = WITHDRAWAL|SWAP, end_lpb = lpb
  InsertTrackingLog(PICKED_UP)
```

### 6.5 `GenerateMonthlyInvoices(runDate)` — job `BILLING_MONTHLY`
```
run_key = runDate (WIB). Insert job_runs(RUNNING) — bila konflik & status SUCCESS → skip (sudah jalan).
for each contract (status ACTIVE/EXPIRED/TERMINATED) :
   period := periodEndingBefore(runDate, contract.billing_day_of_month)
   if runDate != period.end + 1 day → continue           (hanya kontrak yg periodenya baru berakhir)
   tx per kontrak (gagal satu kontrak tidak menggagalkan semua):
      if exists invoice RENTAL non-VOID utk periode → continue
      placements := query irisan (lihat SQL di bawah)
      if empty → continue
      hitung baris (PRORATA/FULL_MONTH), subtotal, pajak via tax_settings yang berlaku di period.end
      invoice_number := docnum.Next("INV", "GLOBAL", runDate)
      InsertInvoice(DRAFT, invoice_date = runDate, due_date = runDate + payment_term_days, generated_by_job = TRUE)
      InsertInvoiceItems
summary {contracts_checked, invoices_created, errors[]} → job_runs SUCCESS/FAILED
```
Query irisan periode (sudah diuji di PostgreSQL 16):
```sql
WITH p AS (SELECT $2::date AS ps, $3::date AS pe)
SELECT rp.id, rp.product_serial_id, rp.customer_site_id, rp.monthly_rate,
       GREATEST(rp.start_date, p.ps)                          AS bill_from,
       LEAST(COALESCE(rp.end_date, p.pe + 1), p.pe + 1)       AS bill_to_excl,
       (LEAST(COALESCE(rp.end_date, p.pe + 1), p.pe + 1) - GREATEST(rp.start_date, p.ps)) AS days_billed,
       (p.pe - p.ps + 1)                                      AS days_in_period
FROM rental_placements rp, p
WHERE rp.contract_id = $1
  AND rp.start_date <= p.pe
  AND (rp.end_date IS NULL OR rp.end_date > p.ps)
ORDER BY rp.customer_site_id, rp.product_serial_id;
```

### 6.6 `FinalizeOpname(opnameID, headID)`
```
tx: opname FOR UPDATE, status SUBMITTED
  hitung diff per (product, rack): non-SN dari stock_opname_items.diff_qty;
  SN: MISSING → keluar, UNEXPECTED → masuk, WRONG_RACK → keluar dari rak sistem + masuk ke rak ditemukan
  bila ada selisih +  → buat LPB OPNAME_ADJUSTMENT_IN (PENDING)
  bila ada selisih −  → buat LPB OPNAME_ADJUSTMENT_OUT (PENDING)
  approve keduanya langsung via ApproveLPB (approver = head, dalam tx yang sama)
  opname FINALIZED; racks.locked_by_opname_id = NULL
```
Catatan: saat opname `IN_PROGRESS`, `system_qty` disnapshot; transaksi LPB pada rak terkunci ditolak sehingga snapshot tetap valid.

### 6.7 Tiket → Sparepart & Swap
```
RequestSparepart(ticket, product, qty) → ticket_spareparts REQUESTED → notifikasi gudang
Gudang: CreateLPBFromSparepart → LPB OUTBOUND SPARE_PART_ISSUE (reference = MAINTENANCE_TICKET)
   → approve (Bagian 6.2) → ticket_spareparts ISSUED, tiket WAITING_PART → IN_PROGRESS

EscalateToSwap(ticket) [TECH rekomendasi → BIZ_ADMIN konfirmasi]:
   buat SO UNIT_SWAP: item OUT (produk pengganti, qty 1) + item IN (product_serial_id unit rusak)
   ticket.swap_so_id = SO; SO.related_ticket_id = ticket
   SO dikonfirmasi → 2 DO: OUTBOUND (unit baru) & INBOUND (unit lama), idealnya 1 driver 1 perjalanan
   Hari swap: pickup unit lama (placement lama end_date = D) & POD unit baru (placement baru start_date = D)
```

---

## 7. Arsitektur Backend Go

### 7.1 Aturan Layer (wajib dipatuhi)

```
handler (Gin)  →  usecase  →  repository (pgx)  →  PostgreSQL
   │ bind+validate DTO        │ aturan bisnis, TxManager      │ SQL saja, baca tx dari ctx
   │ panggil usecase          │ tidak tahu Gin/HTTP           │ tidak ada aturan bisnis
   │ map error → HTTP         │ tidak import pgx              │ kembalikan entity domain
```
- `internal/domain` tidak meng-import package lain di `internal/` maupun library DB/HTTP.
- Usecase menerima interface repository (didefinisikan di `domain`) → mudah di-mock untuk unit test.
- Handler **tipis**: parse param → DTO → usecase → `response.OK(c, data)` atau `httperr.Write(c, err)`.
- Semua method repository menerima `ctx` dan memakai `database.GetExecutor(ctx, r.pool)`.
- Method `Lock*` wajib dipanggil di dalam transaksi (`database.MustInTx`).

### 7.2 Library

| Kebutuhan | Library |
|---|---|
| HTTP | `github.com/gin-gonic/gin`, `github.com/gin-contrib/cors` |
| DB | `github.com/jackc/pgx/v5` (+ `pgxpool`), `github.com/golang-migrate/migrate/v4` |
| Uang | `github.com/shopspring/decimal` |
| JWT | `github.com/golang-jwt/jwt/v5` |
| Password | `golang.org/x/crypto/bcrypt` (cost 12) |
| Validasi | `github.com/go-playground/validator/v10` (bawaan binding Gin) + pesan error Bahasa Indonesia |
| QR | `github.com/skip2/go-qrcode` |
| Storage S3 | `github.com/aws-sdk-go-v2/service/s3` (Supabase Storage / MinIO / S3) |
| Cron | `github.com/robfig/cron/v3` (hanya `cmd/worker`) |
| Excel | `github.com/xuri/excelize/v2` |
| Config | `github.com/caarlos0/env/v11` + `github.com/joho/godotenv` (dev) |
| Log | `log/slog` (JSON di staging/production) |
| Test | `testing`, `github.com/stretchr/testify`, `github.com/testcontainers/testcontainers-go/modules/postgres` |
| Docs | `api/openapi.yaml` (ditulis manual, divalidasi di CI) |

### 7.3 Konfigurasi (`backend/.env.example`)

```dotenv
APP_ENV=local                       # local | staging | production
PORT=8080
APP_BASE_URL=http://localhost:5173  # dipakai untuk link verifikasi QR
CORS_ALLOWED_ORIGINS=http://localhost:5173   # tidak boleh '*' (cookie refresh memakai credentials)
TRUSTED_PROXIES=                    # IP/CIDR proxy tepercaya utk X-Forwarded-For; kosong = tidak ada (ditambah di T0.5)
TZ_APP=Asia/Jakarta

DATABASE_URL=postgres://postgres:postgres@localhost:5432/rental_erp?sslmode=disable
MIGRATE_DATABASE_URL=               # Supabase: session pooler 5432 / direct; kosong = pakai DATABASE_URL
DB_POOLER_MODE=session              # session | transaction (Supabase 6543 → matikan prepared statement)
DB_MAX_CONNS=10

JWT_ACCESS_SECRET=ganti-dengan-random-64-char
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=336h
AUTH_REFRESH_MODE=cookie            # cookie | body
COOKIE_SECURE=false

GOTENBERG_URL=http://localhost:3000
GOTENBERG_USERNAME=
GOTENBERG_PASSWORD=

STORAGE_DRIVER=local                # local | s3
STORAGE_LOCAL_DIR=./storage
S3_ENDPOINT=                        # Supabase: https://<ref>.supabase.co/storage/v1/s3
S3_REGION=
S3_BUCKET=erp-files
S3_ACCESS_KEY=
S3_SECRET_KEY=
S3_FORCE_PATH_STYLE=true

CRON_SECRET=ganti-random            # header Authorization utk /internal/jobs/*
SEED_ADMIN_EMAIL=admin@contoh.id
SEED_ADMIN_PASSWORD=
```

### 7.4 Error, Response & Pagination

```go
// internal/delivery/http/httperr/httperr.go
var mapping = []struct{ err error; status int; code string }{
    {domain.ErrNotFound, 404, "NOT_FOUND"},
    {domain.ErrLPBNotFound, 404, "LPB_NOT_FOUND"},
    {domain.ErrLPBNotPending, 409, "LPB_NOT_PENDING"},
    {domain.ErrInsufficientStock, 409, "INSUFFICIENT_STOCK"},
    {domain.ErrSerialAlreadyExists, 409, "SERIAL_EXISTS"},
    {domain.ErrRackLocked, 409, "RACK_LOCKED_BY_OPNAME"},
    {domain.ErrReservationMismatch, 409, "RESERVATION_MISMATCH"},
    {domain.ErrSerialInOtherLPB, 409, "SERIAL_IN_OTHER_LPB"},
    {domain.ErrSerialCountMismatch, 422, "SERIAL_COUNT_MISMATCH"},
    {domain.ErrSerialNotValid, 422, "SERIAL_NOT_VALID"},
    {domain.ErrLPBEmptyItems, 422, "LPB_EMPTY"},
    {domain.ErrSelfApproveForbidden, 403, "SELF_APPROVE_FORBIDDEN"},
    {domain.ErrForbiddenWarehouse, 403, "FORBIDDEN_WAREHOUSE"},
    {domain.ErrInvalidStateTransition, 409, "INVALID_STATE"},
}
func Write(c *gin.Context, err error) { /* errors.Is loop → JSON; default 500 + log dengan request_id */ }
```
Format respons (sesuai blueprint 3.3) + kode error mesin + detail validasi:
```json
{ "success": false, "message": "Stok di rak tidak mencukupi", "code": "INSUFFICIENT_STOCK",
  "errors": [{ "field": "items[0].qty", "message": "melebihi stok tersedia (10)" }],
  "request_id": "01JD..." }
```
- Pagination: `?page=1&limit=20` (maks 100), sort `?sort=-created_at,lpb_number`, filter `?status=PENDING_APPROVAL&warehouse_id=1&q=epson&date_from=2026-11-01&date_to=2026-11-30`.
- Whitelist kolom sort per endpoint (cegah SQL injection lewat ORDER BY).
- Nominal uang di JSON dikirim sebagai **string** (`"550000.00"`), tanggal `YYYY-MM-DD`, timestamp RFC3339.

### 7.5 Middleware (urutan)
`RequestID → Recover → Logger(slog) → CORS → RateLimit(login: per IP+email, disimpan di DB/tabel atau di proxy) → Auth(JWT) → RequirePermission → Idempotency (endpoint tertentu)`.

`Idempotency`: header `Idempotency-Key` wajib untuk `POST /lpb/:id/approve`, `POST /tracking/lpb/:id/deliver`, `POST /invoices/:id/payments`. Respons pertama disimpan di `idempotency_keys`; request ulang dengan key sama → respons yang sama (mencegah double klik / retry jaringan).

### 7.6 PDF (Gotenberg)
- Template di `backend/templates/*.html`, di-embed dengan `//go:embed` agar ikut ke binary (penting untuk Vercel/container).
- QR berisi URL `{APP_BASE_URL}/verify/lpb/{verify_token}` → **token acak**, bukan ID (tidak bisa ditebak).
- Endpoint Gotenberg: `POST /forms/chromium/convert/html` (multipart `index.html`), timeout 30 detik.
- Template tambahan: `invoice_rental.html`, `label_asset.html` (label stiker asset tag / kode rak, Code128/QR, ukuran kertas label).
- Halaman verifikasi publik hanya menampilkan: nomor LPB, tanggal, gudang, nama pelanggan, jumlah item, approver & waktu approve. **Tanpa** harga/SN lengkap.

---

## 8. Spesifikasi REST API (prefix `/api/v1`)

Kolom "Perm" = kunci permission di matriks 5.1. Semua list endpoint mendukung pagination/filter Bagian 7.4.

### 8.1 Auth & Admin
| Method | Endpoint | Perm | Keterangan |
|---|---|---|---|
| POST | `/auth/login` | publik | rate-limited; set cookie refresh; return access token + user + permissions |
| POST | `/auth/refresh` | cookie | rotasi refresh token; reuse token lama → revoke seluruh family |
| POST | `/auth/logout` | cookie refresh | revoke seluruh family refresh token sesi ini; berbasis refresh token (bukan access token) agar tetap bisa logout saat access token kedaluwarsa; idempoten |
| GET | `/auth/me` | login | user + permissions + default home |
| POST | `/auth/change-password` | login | wajib bila `must_change_password` |
| GET/POST | `/users` | user.manage | daftar (filter `q`, `role`, `is_active`, `warehouse_id`; sort `name,email,role,created_at`) / buat user. Respons buat memuat `temporary_password` (acak, **tampil sekali**, `must_change_password=true`) |
| GET/PATCH | `/users/:id` | user.manage | detail / ubah `name, phone, role, warehouse_id, is_active` (parsial; `null` mengosongkan phone & warehouse_id). `is_active=false` mencabut semua refresh token. Email tidak dapat diubah. Tidak ada hard delete |
| POST | `/users/:id/reset-password` | user.manage | password sementara baru (tampil sekali), wajib ganti saat login, semua sesi dicabut. Tidak untuk akun sendiri (pakai `/auth/change-password`) |
| GET/PUT | `/settings/:key` | admin | `app_settings` |
| GET | `/audit-logs` | admin, mgmt | filter entity/user/tanggal |

### 8.2 Master & Inventory (Fase 1)
| Method | Endpoint | Perm | Keterangan |
|---|---|---|---|
| GET/POST/PATCH | `/warehouses`, `/racks`, `/categories`, `/products`, `/suppliers` | master.* | CRUD + `is_active` |
| POST | `/racks/labels/pdf` | master.read | cetak label barcode kode rak |
| GET | `/inventory/balances` | inventory.read | `qty_on_hand, qty_reserved, qty_available, is_low_stock`; filter product/rack/warehouse/low_stock |
| GET | `/inventory/balances/summary` | inventory.read | total per produk (semua rak) |
| GET | `/inventory/serials` | inventory.read | list SN, filter status/produk/customer |
| GET | `/inventory/serials/scan/:code` | inventory.read | lookup asset tag **atau** SN: produk, rak, status, customer, 5 histori terakhir |
| GET | `/inventory/serials/:id/history` | inventory.read | timeline `product_serial_histories` + tiket |
| POST | `/inventory/serials/labels/pdf` | inventory.read | cetak label asset tag (batch) |
| GET | `/inventory/ledgers` | ledger.read | buku besar; filter produk/rak/tanggal/movement_type |
| GET | `/inventory/stock-card` | ledger.read | kartu stok produk per rak (saldo awal, mutasi, saldo akhir) |
| GET | `/inventory/reconcile` | admin | hasil invariant Bagian 5.9 |

### 8.3 LPB & Opname (Fase 1)
| Method | Endpoint | Perm | Keterangan |
|---|---|---|---|
| GET | `/lpb` | lpb.read | filter type/sub_type/status/warehouse/tanggal |
| POST | `/lpb` | lpb.create | buat draft (header + items) |
| GET | `/lpb/:id` | lpb.read | detail + items + serials + timeline |
| PATCH | `/lpb/:id` | lpb.create | ubah header/items (DRAFT saja) |
| POST | `/lpb/:id/items/:itemId/serials` | lpb.create | tambah SN hasil scan (validasi real-time) |
| DELETE | `/lpb/:id/items/:itemId/serials/:serialId` | lpb.create | hapus SN salah scan |
| POST | `/lpb/:id/submit` | lpb.submit | DRAFT → PENDING_APPROVAL |
| POST | `/lpb/:id/cancel` | lpb.create | DRAFT → CANCELLED |
| GET | `/lpb/approvals` | lpb.approve | antrean PENDING untuk gudang approver |
| POST | `/lpb/:id/approve` | lpb.approve | **ACID** (Bagian 6.2), wajib `Idempotency-Key` |
| POST | `/lpb/:id/reject` | lpb.approve | body `{reason}` |
| POST | `/lpb/:id/revise` | lpb.create | REJECTED → DRAFT |
| PATCH | `/lpb/:id/delivery-assignment` | delivery.assign | set driver & plat nomor |
| GET | `/lpb/:id/pdf` | lpb.print | hanya APPROVED; `?download=1` |
| GET | `/public/lpb/verify/:token` | publik | data ringkas verifikasi QR (rate-limited) |
| POST | `/inventory/opnames` | opname.create | mulai opname (pilih rak → dikunci, snapshot qty) |
| GET | `/inventory/opnames/:id` | opname.read | detail + progres |
| PUT | `/inventory/opnames/:id/counts` | opname.create | input hitungan non-SN |
| POST | `/inventory/opnames/:id/scans` | opname.create | scan SN per rak |
| POST | `/inventory/opnames/:id/submit` | opname.create | |
| POST | `/inventory/opnames/:id/finalize` | opname.finalize | buat & approve LPB adjustment, buka kunci rak |
| POST | `/inventory/opnames/:id/cancel` | opname.finalize | buka kunci rak |

### 8.4 Komersial (Fase 2)
| Method | Endpoint | Perm | Keterangan |
|---|---|---|---|
| GET/POST/PATCH | `/customers`, `/customers/:id` | customer.* | |
| GET/POST/PATCH | `/customers/:id/sites`, `/customer-sites/:id` | customer.* | koordinat dipilih via peta (klik pin) |
| GET | `/customers/:id/assets` | customer.read | unit terpasang per site |
| GET/POST/PATCH | `/contracts`, `/contracts/:id` | contract.* | header + items |
| POST | `/contracts/:id/activate` \| `/terminate` \| `/renew` | contract.approve | |
| GET/POST/PATCH | `/sales-orders`, `/sales-orders/:id` | so.* | kalkulasi DPP/PPN server-side |
| POST | `/sales-orders/:id/confirm` \| `/cancel` | so.approve | |
| POST | `/sales-orders/:id/delivery-orders` | do.create | terbitkan DO ke gudang |
| GET | `/delivery-orders` | do.read | antrean gudang |
| POST | `/delivery-orders/:id/reserve` | do.reserve | Bagian 6.3 |
| POST | `/delivery-orders/:id/release` | do.reserve | lepas reservasi |
| POST | `/delivery-orders/:id/lpb` | do.reserve | buat draft LPB dari reservasi |
| GET | `/invoices` | invoice.read | filter status/customer/periode/overdue |
| POST | `/invoices/generate` | invoice.create | jalankan billing manual utk tanggal tertentu (dry-run `?preview=1`) |
| POST | `/invoices/:id/issue` \| `/void` | invoice.approve | |
| POST | `/invoices/:id/payments` | invoice.pay | catat pembayaran + PPh 23 (Idempotency-Key) |
| GET | `/invoices/:id/pdf` | invoice.read | |
| GET | `/invoices/export` | invoice.read | Excel daftar tagihan / rekap PPN |

### 8.5 Lapangan (Fase 3)
| Method | Endpoint | Perm | Keterangan |
|---|---|---|---|
| GET | `/tracking/my-deliveries` | driver | antrean tugas aktif (kirim & jemput) |
| POST | `/tracking/lpb/:id/checkpoint` | driver | `{status, latitude, longitude, accuracy, client_event_id, notes}` |
| POST | `/tracking/lpb/:id/pickup` | driver | jemput unit retur (Bagian 6.4) |
| POST | `/tracking/lpb/:id/deliver` | driver | multipart: `pod_photo`, `receiver_name`, `latitude`, `longitude`, `client_event_id` |
| POST | `/tracking/lpb/:id/fail` | driver | `{reason, lat, lng}` |
| GET | `/tracking/lpb/:id/route` | tracking.read | polyline checkpoint + foto POD |
| GET | `/tracking/map/assets` | map.read | pin per site: jumlah & daftar unit RENTED |
| GET/POST/PATCH | `/tickets`, `/tickets/:id` | ticket.* | |
| POST | `/tickets/:id/assign` \| `/status` \| `/activities` | ticket.* | |
| POST | `/tickets/:id/spareparts` | ticket.work | minta sparepart |
| POST | `/ticket-spareparts/:id/lpb` | lpb.create | gudang membuat LPB SPARE_PART_ISSUE |
| POST | `/tickets/:id/escalate-swap` | ticket.work | buat SO UNIT_SWAP draft |
| GET | `/dashboard/summary` | dashboard.read | metrik Bagian 9.5 |
| GET | `/reports/*` | report.read | utilisasi aset, omzet per pelanggan, aging piutang, SLA tiket |

### 8.6 Internal Jobs
| Method | Endpoint | Auth | Keterangan |
|---|---|---|---|
| GET | `/internal/jobs/billing` | `Bearer CRON_SECRET` | `BILLING_MONTHLY` untuk hari ini (WIB) |
| GET | `/internal/jobs/daily` | `Bearer CRON_SECRET` | low-stock alert, contract expiry, reconcile, DO reservasi menggantung |
| GET | `/healthz`, `/readyz` | publik | liveness & cek DB |

Contoh `backend/vercel.json` (bila API di Vercel; Hobby = maksimal sekali sehari, jadwal UTC):
```json
{
  "framework": "go",
  "crons": [
    { "path": "/api/v1/internal/jobs/billing", "schedule": "0 18 * * *" },
    { "path": "/api/v1/internal/jobs/daily",   "schedule": "30 18 * * *" }
  ]
}
```
(18:00 UTC = 01:00 WIB.) Pastikan format `vercel.json` dicek ulang ke dokumentasi Vercel saat setup karena runtime Go masih Beta.

---

## 9. Frontend Vue 3

### 9.1 Setup
```bash
npm create vue@latest frontend   # TypeScript, Router, Pinia, ESLint, Prettier, Vitest
cd frontend
npm i axios primevue @primeuix/themes primeicons leaflet dayjs vee-validate zod @vee-validate/zod
npm i -D tailwindcss @tailwindcss/vite @types/leaflet openapi-typescript vite-plugin-pwa
```
`.env.example`:
```dotenv
VITE_API_BASE_URL=/api/v1
VITE_MAP_TILE_URL=https://tile.openstreetmap.org/{z}/{x}/{y}.png
VITE_MAP_ATTRIBUTION=&copy; OpenStreetMap contributors
VITE_DEFAULT_CENTER=-6.9825,110.4091
```
Dev proxy di `vite.config.ts`: `server.proxy['/api'] = 'http://localhost:8080'` → FE & API satu origin juga saat lokal (cookie refresh jalan sama seperti staging).

### 9.2 Konvensi
- Selalu `<script setup lang="ts">`, TypeScript `strict`.
- Akses API hanya lewat `src/services/*.ts` (bukan axios langsung di view). Tipe dari `src/types/api.generated.ts` (`npx openapi-typescript ../backend/api/openapi.yaml -o src/types/api.generated.ts`).
- Tabel server-side via composable `useServerTable(fetchFn)` → state `page, limit, sort, filters, rows, total, loading`, filter tersinkron ke query string URL (bisa di-bookmark/dibagikan).
- Format: `formatRupiah()` (`Intl.NumberFormat('id-ID', {style:'currency', currency:'IDR'})`), tanggal `dayjs` locale `id`, zona `Asia/Jakarta`.
- Uang dari API berupa string → tampilkan saja; perhitungan DPP/PPN **hanya di backend** (FE boleh pratinjau, angka final dari respons server).
- Semua teks UI Bahasa Indonesia; label tombol diawali kata kerja ("Setujui LPB", "Cetak Surat Jalan").
- `StatusBadge.vue` satu sumber warna/label untuk semua status (LPB, SN, SO, DO, invoice, tiket).
- Tombol aksi disembunyikan bila tidak punya permission (`usePermission().can('lpb.approve')`) — **otorisasi sebenarnya tetap di backend**.

### 9.3 Perubahan dari Kode Blueprint
| Komponen | Perubahan |
|---|---|
| `lib/axios.ts` | Refresh token dari cookie (`withCredentials: true` untuk `/auth/refresh`); access token di memori Pinia. Antrean refresh tetap seperti blueprint. Tambah header `Idempotency-Key` (`crypto.randomUUID()`) untuk aksi kritis. |
| `stores/auth.ts` | Hapus penyimpanan token di `localStorage`; `bootstrap()` memanggil `/auth/refresh` saat app start; simpan `permissions[]`. |
| `router/index.ts` | `meta.permission` (bukan `meta.roles` saja); guard menunggu `auth.bootstrap()` sekali. |
| `SerialScannerPOS.vue` | `AudioContext` singleton di `lib/audio.ts` + `resume()` saat klik pertama; terima asset tag atau SN; deteksi scan dobel lokal sebelum hit API; mode "batch" (antrean scan tetap jalan walau validasi sebelumnya belum selesai, maks 3 paralel). |
| `AssetTrackingMap.vue` | URL tile & atribusi dari env; marker clustering (`leaflet.markercluster`) bila pin > 200. |
| `DriverDeliveryAction.vue` | Kirim `client_event_id`; simpan aksi gagal ke antrean offline (IndexedDB) dan kirim ulang saat online (Fase 3 lanjutan). |

### 9.4 Daftar Halaman per Fase
| Fase | Route | View | Role utama |
|---|---|---|---|
| 0 | `/login`, `/403`, `/akun/password` | Login, Forbidden, Ganti password | semua |
| 1 | `/master/gudang`, `/master/rak`, `/master/kategori`, `/master/produk`, `/master/supplier` | CRUD master | HEAD_WH, ADMIN |
| 1 | `/stok` | Saldo stok per produk/rak + filter stok menipis | WH, MGMT |
| 1 | `/stok/serial`, `/stok/serial/:id` | Daftar & detail unit (timeline histori) | WH, TECH |
| 1 | `/stok/buku-besar`, `/stok/kartu-stok` | Ledger & kartu stok | WH, MGMT |
| 1 | `/lpb`, `/lpb/baru`, `/lpb/:id` | Daftar, form + scanner POS, detail | STAFF_WH |
| 1 | `/lpb/approvals` | Antrean approval (diff stok sebelum/sesudah) | HEAD_WH |
| 1 | `/opname`, `/opname/:id` | Opname per rak (mode scan & hitung) | WH |
| 1 | `/verify/lpb/:token` | Verifikasi QR publik | publik |
| 1 | `/dashboard` | Versi gudang | WH, MGMT |
| 2 | `/pelanggan`, `/pelanggan/:id` | Pelanggan + site (picker peta) + aset terpasang | BIZ_ADMIN |
| 2 | `/kontrak`, `/kontrak/:id` | Kontrak + item tarif | BIZ_ADMIN |
| 2 | `/sales-orders`, `/sales-orders/:id` | SO 4 tipe + DO | BIZ_ADMIN |
| 2 | `/delivery-orders` | Antrean DO gudang → reservasi (scan) → buat LPB | STAFF_WH |
| 2 | `/invoices`, `/invoices/:id` | Tagihan, pratinjau generate, pembayaran | BIZ_ADMIN |
| 3 | `/driver/deliveries`, `/driver/deliveries/:id` | Mobile layout: tugas, navigasi, checkpoint, POD | DRIVER |
| 3 | `/tracking/map` | Peta sebaran aset + rute driver | MGMT, BIZ_ADMIN |
| 3 | `/tickets`, `/tickets/:id` | Papan kanban tiket + detail | TECH, BIZ_ADMIN |
| 3 | `/laporan/*` | Laporan | MGMT |
| 4 | `/admin/users`, `/admin/settings`, `/admin/audit`, `/admin/rekonsiliasi`, `/admin/jobs` | Administrasi | ADMIN |

### 9.5 Metrik Dashboard
Gudang: stok aman vs menipis (`< min_stock_alert`), antrean LPB menunggu approval, DO menunggu reservasi, opname berjalan.
Komersial: unit RENTED aktif, utilisasi aset (`RENTED / total unit non-RETIRED`), invoice jatuh tempo & overdue, omzet bulan berjalan, kontrak berakhir ≤ 30 hari.
Lapangan: pengiriman IN_TRANSIT, gagal kirim hari ini, tiket terbuka per prioritas & yang melewati SLA.
Admin: hasil rekonsiliasi terakhir, status job terakhir.

### 9.6 Driver Mobile Web (PWA)
- Layout khusus mobile (tombol besar, satu kolom), `vite-plugin-pwa` agar bisa "Add to Home Screen".
- Kamera belakang: `<input type="file" accept="image/*" capture="environment">` + kompresi canvas (blueprint) — target < 300 KB.
- Wajib HTTPS (Vercel/production sudah HTTPS; lokal pakai Cloudflare Tunnel/ngrok untuk tes di HP).
- Tombol navigasi ke Google Maps (link tanpa API key, sesuai blueprint).

---

## 10. Keamanan

| Area | Implementasi |
|---|---|
| Password | bcrypt cost 12, minimal 8 karakter, `must_change_password` untuk akun baru/reset |
| Login | rate limit per IP + email (mis. 5 gagal / 15 menit), audit `LOGIN_FAILED` |
| Token | Access JWT 15 menit (claims: `sub`, `role`, `wid`, `exp`), refresh opaque random 32 byte disimpan **hash SHA-256**, rotasi setiap refresh, reuse detection per `family_id` |
| Otorisasi | `RequirePermission` di setiap route + cek kepemilikan data di usecase (driver hanya LPB miliknya, warehouse scoping) |
| Input | Validasi DTO, whitelist sort/filter, query parameterized (tidak ada string concat SQL) |
| Upload | Maks 5 MB, cek MIME dari isi file (`http.DetectContentType`), hanya `image/jpeg|png|webp|application/pdf`, nama file di-generate server (UUID), simpan di bucket **privat**; akses via signed URL berumur pendek |
| Data API Supabase | Nonaktifkan exposure schema `public` di Data API / RLS deny-all; backend memakai koneksi Postgres langsung |
| Secrets | Hanya di env Vercel/host, tidak di repo; `.env` di `.gitignore` |
| Header | `X-Content-Type-Options`, `X-Frame-Options: DENY`, `Referrer-Policy`, CSP dasar di FE |
| Gotenberg | Tidak terekspos publik (jaringan internal) atau basic auth |
| Audit | Semua aksi approve/reject/void/payment/user management tercatat di `audit_logs` |
| Backup (prod) | `pg_dump` harian terenkripsi + retensi 14 hari + uji restore bulanan; backup bucket file |

---

## 11. Strategi Testing

| Level | Cakupan wajib | Tools |
|---|---|---|
| Unit (usecase) | aturan transisi status, validasi LPB, perhitungan PPN & prorata (tabel kasus), periode tagihan per `billing_day_of_month` | `testing` + mock repo |
| Unit (pkg) | `money` pembulatan, `docnum`, JWT, kompresi param | `testing` |
| Integrasi (repo + tx) | ApproveLPB semua sub-type, reserve DO, deliver, billing job idempoten, opname finalize | testcontainers Postgres 16 + migrasi asli |
| **Konkurensi** | 2 approve LPB OUT paralel pada stok yang sama → tepat 1 sukses, saldo tidak negatif; 2 reservasi SN yang sama → 1 gagal; approve LPB dobel (idempotency) | goroutine + `sync.WaitGroup` di test integrasi |
| Invariant | Setelah setiap test integrasi jalankan query Bagian 5.9 → harus 0 baris | helper `assertInvariants(t, db)` |
| API | handler + middleware RBAC (role salah → 403) | `httptest` |
| Frontend | composable (`useServerTable`, `useDriverCapture`), komponen scanner | Vitest + Vue Test Utils |
| E2E (opsional Fase 4) | login → LPB → approve → cetak PDF; SO → DO → reserve → POD → invoice | Playwright |

**Data uji kasus tagihan** (wajib ada di test): penuh 1 periode; kirim tengah periode; tarik tengah periode; swap di hari yang sama (hari tidak ganda); periode Februari (28/29 hari); `billing_day_of_month = 15`; kontrak non-PPN; invoice VOID lalu generate ulang.

CI (GitHub Actions): `go vet`, `golangci-lint`, `go test ./... -race` (dengan service Postgres), `npm run lint`, `npm run type-check`, `npm run test`, `npm run build`, validasi `openapi.yaml`.

---

## 12. Runbook Environment & Deploy

### 12.1 Local (docker-compose.dev.yml)
```yaml
services:
  postgres:
    image: postgres:16
    environment: { POSTGRES_PASSWORD: postgres, POSTGRES_DB: rental_erp, TZ: Asia/Jakarta }
    ports: ["5432:5432"]
    volumes: [pgdata:/var/lib/postgresql/data]
  gotenberg:
    image: gotenberg/gotenberg:8
    ports: ["3000:3000"]
  minio:            # opsional, bila ingin menguji S3 lokal
    image: minio/minio
    command: server /data --console-address ":9001"
    ports: ["9000:9000", "9001:9001"]
volumes: { pgdata: {} }
```
`Makefile` backend: `make dev` (air/hot reload), `make migrate-up`, `make migrate-down`, `make migrate-new name=...`, `make seed`, `make test`, `make test-integration`, `make lint`.

### 12.2 Staging (Vercel + Supabase) — langkah
1. **Supabase**: buat project region Singapura (`ap-southeast-1`) agar latensi ke Indonesia rendah. Simpan 2 connection string: *Transaction pooler* (6543) untuk runtime, *Session pooler* (5432) untuk migrasi.
2. Jalankan migrasi dari laptop/CI: `MIGRATE_DATABASE_URL=<session pooler> make migrate-up && make seed`.
3. **Supabase Storage**: buat bucket privat `erp-files`, buat S3 access key (menu Storage → S3), isi env `S3_*`.
4. **Gotenberg**: deploy image `gotenberg/gotenberg:8` di container host (Render/Fly/Koyeb/VPS kecil) dengan basic auth → isi `GOTENBERG_URL`.
5. **API**: project Vercel baru, Root Directory `backend`, framework Go; env: `DATABASE_URL` (6543) + `DB_POOLER_MODE=transaction` + `DB_MAX_CONNS=5`, JWT, S3, Gotenberg, `CRON_SECRET`, `APP_ENV=staging`, `COOKIE_SECURE=true`.
   *Alternatif*: deploy `backend/Dockerfile` ke Render/Fly/Koyeb (gunakan `DB_POOLER_MODE=session` atau transaction) — lebih mirip production.
6. **Frontend**: project Vercel, Root Directory `frontend`, build `npm run build`, output `dist`; `vercel.json` rewrites `/api/*` → URL API staging (Bagian 3.3).
7. Uji: login → buat LPB → approve → cetak PDF → buka QR dari HP → driver POD dari HP (HTTPS).

### 12.3 Production (VPS + domain)
- `docker-compose.prod.yml`: `caddy` (HTTPS otomatis Let's Encrypt, serve `frontend/dist` + reverse proxy `/api` → `api:8080`), `api`, `worker`, `gotenberg` (tanpa port publik), `postgres:16` (atau managed DB), `minio` (opsional).
- Domain: `erp.domainanda.co.id` (satu origin untuk FE + API).
- **Migrasi data dari Supabase** (jika data staging ingin dibawa): `pg_dump --no-owner --no-acl -Fc` dari Supabase (session pooler) → `pg_restore` ke Postgres production; salin isi bucket storage.
- Ganti `DB_POOLER_MODE=session`, `DB_MAX_CONNS=20`, `STORAGE_DRIVER=s3|local`, worker cron aktif (`cmd/worker`), matikan endpoint `/internal/jobs/*` atau tetap dengan `CRON_SECRET`.
- Monitoring: uptime check `/healthz`, log JSON (docker logs → Loki/Grafana atau cukup `journald` di awal), 💡 Sentry untuk error FE & BE.
- Backup harian ke storage eksternal (Bagian 10).

### 12.4 Checklist Go-Live
- [ ] Data master final (gudang, rak + label tercetak, produk, pelanggan, site + koordinat, kontrak + tarif)
- [ ] Stock opname awal fisik → import `INITIAL_STOCK` (CSV/Excel) → approve Kepala Warehouse
- [ ] Unit yang **sudah** di pelanggan diimport sebagai SN `RENTED` + `rental_placements` (start_date = tanggal kontrak/terpasang) lewat skrip migrasi khusus (bukan LPB biasa) + catatan audit
- [ ] Semua user dibuat, password awal dibagikan, wajib ganti password
- [ ] Uji tagihan bulan pertama dengan `POST /invoices/generate?preview=1` dibandingkan dengan tagihan manual
- [ ] Backup & restore teruji, HTTPS aktif, secrets production berbeda dari staging
- [ ] Pelatihan per role (staff gudang: scanner & LPB; kepala gudang: approval & opname; admin: SO/DO/invoice; driver: HP)

---

## 13. Roadmap & Task Breakdown

> Estimasi asumsi **1 developer full-stack** dibantu Claude Code; 1 sprint ≈ 1–2 minggu. Sesuaikan dengan kapasitas.
> Format task: `T<fase>.<no>` · centang `[x]` bila DoD terpenuhi. Setiap task backend **wajib** disertai test (unit/integrasi) yang relevan.

### Fase 0 — Fondasi (Sprint 0)

- [x] **T0.1 Inisialisasi monorepo** — struktur folder Bagian 3.5, `.gitignore`, `README.md`, `CLAUDE.md`, `docs/`, `.editorconfig`.
  DoD: repo ter-push, README berisi cara menjalankan lokal.
- [x] **T0.2 Skeleton backend** — `go.mod` (module `rental-inventory-api`), `config` (env), slog, Gin router, `/healthz` `/readyz`, graceful shutdown, baca `PORT`, `Dockerfile` multi-stage, `Makefile`.
  DoD: `make dev` jalan; `docker build` sukses; `/healthz` 200.
- [x] **T0.3 Lapisan database** — `pkg/database` (pgxpool + `DB_POOLER_MODE`, TxManager, `GetExecutor`, `MustInTx`), `cmd/migrate`, `deploy/docker-compose.dev.yml`.
  DoD: test integrasi TxManager (commit, rollback saat error, rollback saat panic, nested pakai tx yang sama).
- [x] **T0.4 Migrasi awal + seed** — `000001`, `000002` (+ `.down.sql`), `cmd/seed` (admin, app_settings, kategori).
  DoD: `make migrate-up && make migrate-down && make migrate-up` bersih.
- [x] **T0.5 Fondasi HTTP** — `pkg/response`, `pkg/pagination`, `httperr`, pesan validasi Bahasa Indonesia, middleware RequestID/Recover/Logger/CORS/security headers.
  DoD: test handler contoh untuk 200/400/404/500 sesuai format Bagian 7.4.
- [x] **T0.6 Auth** — login, refresh (cookie, rotasi, reuse detection), logout, me, change-password, `domain/permission.go` (matriks 5.1), `RequirePermission`, rate limit login, audit login.
  DoD: test reuse refresh token me-revoke family; role tanpa permission → 403.
- [x] **T0.7 Manajemen user** — CRUD user, reset password, nonaktifkan (refresh token dicabut).
  Aturan pengaman (diputuskan saat T0.7): admin tidak dapat menonaktifkan / mengubah role / mereset password akunnya sendiri; ADMIN aktif terakhir tidak dapat dinonaktifkan atau diturunkan (dijaga dengan lock berurutan); `warehouse_id` hanya untuk role `STAFF_WAREHOUSE`/`HEAD_WAREHOUSE` (NULL = semua gudang).
- [x] **T0.8 Skeleton frontend** — Vite + TS + Tailwind + PrimeVue, `AppLayout`/`MobileLayout`/`PublicLayout`, axios + antrean refresh, store auth (`bootstrap()`), router guard permission, halaman Login, 403, Ganti Password, sidebar dinamis per permission.
  DoD: login → redirect ke default home per role; refresh halaman tetap login (via cookie).
  Keputusan saat T0.8: **PrimeVue dipin ke 4.x** (npm `latest` sudah 5.x) dengan `@primeuix/themes` 2.x; **zod 3.x** karena `@vee-validate/zod` belum mendukung zod 4; default home yang halamannya belum ada jatuh ke `/beranda`; menu sidebar tampil hanya bila permission dimiliki **dan** rutenya sudah terdaftar (halaman fase berikutnya muncul otomatis); tipe API sementara ditulis tangan di `src/types` (ganti dengan hasil generate setelah `openapi.yaml` ada); laci menu memakai `<dialog>` modal tanpa gestur geser.
- [x] **T0.9 CI** — GitHub Actions (Bagian 11).
  Keputusan saat T0.9: workflow `.github/workflows/ci.yml` dengan 3 job (backend, frontend, openapi). CI frontend memakai skrip baru `check:lint` / `check:format` karena `npm run lint` memakai `--fix` (CI harus gagal, bukan memperbaiki diam-diam). Test integrasi dijalankan terhadap service Postgres 16 dan **gagal bila ada test yang di-skip**. `golangci-lint` v2 memakai `backend/.golangci.yml` (ST1005 dimatikan karena sentinel error sengaja berupa kalimat Indonesia berhuruf kapital). Job `openapi` hanya melakukan lint bila `backend/api/openapi.yaml` sudah ada (belum ada sampai fase berikutnya).
- [ ] **T0.10 Deploy staging pertama** — Supabase + Vercel (FE & API) + Gotenberg container (Bagian 12.2).
  DoD: login berhasil di URL staging HTTPS.
  **Ditunda** atas permintaan user: dikerjakan setelah menu/fitur utama lengkap; Fase 1 berjalan lebih dulu di lokal.

### Fase 1 — Inti Gudang (Sprint 1–4)

**Sprint 1 — Master data**
- [x] **T1.1 Migrasi `000003`, `000004`** (+ down) dan `pkg/docnum`.
  DoD: test `docnum` paralel 50 goroutine → nomor unik berurutan tanpa celah.
  Catatan T1.1: `docnum.Generator.Next(ctx, docType, scope, t)` wajib dipanggil di dalam transaksi (`database.MustInTx` panik bila tidak) dan periodenya dihitung WIB; `NextNumber(ctx, docType, scope, period)` mengembalikan counter mentah (dengan `docnum.PeriodNone` = `000000` untuk urutan tanpa reset). Antarmuka `domain.DocNumberGenerator` dipakai usecase. FK `racks.locked_by_opname_id` ditambahkan di `000004` (komentar di `000001` yang menyebut `000005` sudah dikoreksi). `lpb_documents.doc_date` memakai `DEFAULT CURRENT_DATE` mengikuti zona DB, jadi usecase **wajib** mengisinya eksplisit dari `pkg/clock`. **Format asset tag (diputuskan, menyatukan Bagian 4 dan 6.1):** `AST-{YYMM}-{NNNNN}`, mis. `AST-2611-00012`. Counter `NNNNN` bersifat global dan **tidak pernah di-reset** (`AST`, scope `GLOBAL`, periode `000000`, sesuai 6.1); `YYMM` (WIB) hanya penanda bulan unit didaftarkan (sesuai contoh Bagian 4). Keunikan dijamin counter, bukan bulan. Dipakai lewat `docnum.Generator.NextAssetTag(ctx, t)` di dalam transaksi yang membuat `product_serials` (T1.6).
- [x] **T1.2 API master** — warehouses, racks (zone, `is_sellable`), categories, products (`has_serial_number`, `min_stock_alert`), suppliers.
  Keputusan T1.2 (plan tidak merinci; diambil praktik terbaik yang konsisten dengan Bagian 4–5):
  - Endpoint: `GET/POST /{warehouses,racks,categories,products,suppliers}` dan `GET/PATCH /…/:id` (tanpa hard delete; `is_active` lewat PATCH). Baca: `master.read` (produk: `product.read` agar TECHNICIAN ikut); tulis: `master.write` (HEAD_WAREHOUSE) — **rak: `rack.write`** (STAFF_WAREHOUSE ikut). Daftar mendukung `page/limit/sort/q/is_active` + filter khusus (rak: `warehouse_id/zone/is_sellable`; produk: `category_id/item_type/has_serial_number`; kategori: `item_type`).
  - **Warehouse scoping**: akun yang dibatasi ke satu gudang hanya melihat/mengubah gudangnya dan rak di dalamnya (403 `FORBIDDEN_WAREHOUSE` untuk gudang lain) dan **tidak boleh membuat gudang baru**. Kategori, produk, supplier bersifat global.
  - **Kode tidak dapat diubah** setelah dibuat: kode gudang (dipakai sebagai scope nomor dokumen, huruf besar/angka/tanda hubung, maks 20), kode rak (tercetak di label), kode supplier. SKU & `has_serial_number` produk dikunci (409 `PRODUCT_IN_USE`) setelah produk punya stok/SN/LPB/opname; `item_type` kategori dikunci (409 `CATEGORY_IN_USE`) setelah dipakai produk.
  - **Nonaktifkan** gudang/rak yang masih menyimpan stok atau unit SN ditolak (409 `LOCATION_HAS_STOCK`); rak yang dikunci opname tidak dapat diubah (409 `RACK_LOCKED_BY_OPNAME`); rak hanya bisa dibuat/diaktifkan kembali di gudang aktif. Kategori/produk/supplier boleh dinonaktifkan kapan saja (histori tetap utuh). **Untuk T1.5–T1.6:** LPB/approval wajib memastikan gudang, rak, produk aktif di dalam transaksi.
  - Rak: `is_sellable` bawaan `false` untuk zona `KARANTINA`/`WORKSHOP`, selain itu `true`; nilai eksplisit menang. Zona disimpan huruf besar.
  - Produk: barang `CONSUMABLE` tidak boleh `has_serial_number` (dikelola per qty); tarif/harga berupa **string desimal** (`"550000.00"`, maks 2 desimal, tidak negatif; angka JSON ditolak), satuan bawaan `unit`.
  - Nilai unik bentrok → 409 `DUPLICATE_VALUE` dengan `errors[].field` (kode gudang/rak, nama kategori, SKU, kode supplier); SKU/kode dinormalisasi huruf besar.
  - Setiap create/update menulis `audit_logs` (`<ENTITY>_CREATE|UPDATE|ACTIVATE|DEACTIVATE`; update hanya mencatat field yang berubah; PATCH tanpa perubahan tidak menulis apa pun).
  - Belum ada: `POST /racks/labels/pdf` (T1.11), `openapi.yaml`.
- [x] **T1.3 UI master** — CRUD + pencarian + toggle aktif.
  Keputusan T1.3 (praktik terbaik UX proyek; plan tidak merinci):
  - Lima daftar di `/master/{produk,kategori,gudang,rak,supplier}` (pola **List / Index**): pencarian berlabel (`/` memfokuskan), satu kontrol **Filter** (panel popover) + chip filter aktif, tabel dalam bingkai dengan header lengket dan urutan lewat tombol kolom (`aria-sort` hanya pada kolom aktif), paginasi selalu tampil, status kosong **di dalam bingkai** (pertama kali: judul + nilai + tombol buat; hasil filter kosong: tombol *Hapus semua filter*; galat memuat: *Coba lagi*). Status daftar (q, filter, urutan, halaman, limit) tersimpan di **query string URL** lewat `useServerTable` (bisa di-bookmark; reload mempertahankan).
  - **Buat/ubah**: ≥ 3 field = halaman sendiri (`/master/X/baru`, `/master/X/:id`, pola **Form**, blur-first, Simpan tidak pernah dinonaktifkan, galat server 409/422 dipetakan ke field-nya); kategori (2 field) memakai **panel samping** (`<dialog>` modal). Klik kode/SKU/nama membuka form ubah; kode (gudang/rak/supplier) tampil readonly saat ubah.
  - **Nonaktifkan** memakai dialog konfirmasi yang **menyebut entitas** ("Nonaktifkan gudang 'WH-A'?"), fokus awal di *Batal*, Escape membatalkan; **Aktifkan** langsung. Penolakan server (mis. rak masih berisi stok) tampil sebagai toast apa adanya.
  - Hak akses di UI mengikuti backend: tombol/tautan tulis disembunyikan tanpa `master.write` (rak: `rack.write`); rute form diblok guard (403). Teknisi (hanya `product.read`) melihat daftar Produk saja. Menu sidebar **Data Master** memakai `product.read` (superset dari `master.read`) dan aktif di semua halaman `/master/*`.
  - Komponen bersama baru: `ListPage`, `DataTable`, `TablePagination`, `FilterPanel`, `FormPage`, `SlideOver`, `ConfirmDialog`, `StatusBadge`, `SelectField`/`TextAreaField`/`CheckboxField`; composable `useServerTable`, `useEntityForm`, `useActiveToggle`, `useOptions`; `formatRupiah` (uang tetap string dari API).
  - Batasan yang diketahui: pilihan gudang/kategori pada select & filter memuat maksimal 100 data; nama kategori unik di database bersifat **peka huruf besar/kecil** ("Printer" dan "printer" dapat berdampingan) — pertimbangkan indeks unik `lower(name)` pada migrasi berikutnya.

**Sprint 2 — LPB masuk & scanner**
- [x] **T1.4 API baca inventory** — balances (+ `qty_available`, `is_low_stock`), summary, serials, `scan/:code` (asset tag / SN).
  Keputusan T1.4 (plan tidak merinci; praktik terbaik yang konsisten dengan Bagian 4–5):
  - Endpoint (semua `inventory.read`: MGMT, BIZ_ADMIN, STAFF, HEAD, TECH; DRIVER tidak): `GET /inventory/balances`, `/balances/summary`, `/serials`, `/serials/scan/:code`. Hanya baca — tidak mengubah stok, ledger, maupun audit. `/serials/:id/history` (timeline + tiket) ditunda ke **T1.13** sesuai plan; `scan` sudah memuat 5 histori terakhir.
  - **`qty_available`** = `qty_on_hand − qty_reserved` pada rak `is_sellable`, selain itu **0** (stok karantina/workshop tetap tercatat tetapi tidak tersedia).
  - **`is_low_stock`** dinilai di **tingkat produk** (`min_stock_alert` melekat pada produk): total `qty_available` produk di cakupan gudang pemanggil/filter `warehouse_id` `< min_stock_alert`, dan `min_stock_alert > 0`. Filter rak/pencarian tidak mengubah penilaian. Akun satu gudang menilai di gudangnya saja.
  - `balances`: baris saldo per (produk, rak); rak yang saldonya 0 disembunyikan kecuali `include_zero=true`. `summary`: satu baris per **produk** (semua produk ikut, termasuk yang stoknya 0 → dapat ditandai menipis; produk nonaktif hanya bila masih menyimpan stok atau `is_active=false` diminta), `rack_count` = jumlah rak berstok; filter `low_stock`, `has_stock`, `category_id`, `item_type`, `warehouse_id`, `q`.
  - **Warehouse scoping**: saldo dibatasi ke gudang akun (`warehouse_id` gudang lain → 403 `FORBIDDEN_WAREHOUSE`). Daftar unit SN: unit di gudang akun **plus** unit yang tidak berada di gudang mana pun (RENTED/IN_TRANSIT/LOST/RETIRED) tetap terlihat.
  - **Scan**: urutan pencarian asset tag (tanpa peduli huruf) → SN persis → SN tanpa peduli huruf; spasi tepi dipangkas; tidak ketemu → 404 (pesan menyebut kodenya); SN yang sama pada >1 produk → **409 `SERIAL_AMBIGUOUS`** (minta scan asset tag, tidak menebak); unit di gudang lain dari akun yang dibatasi → 403 tanpa membocorkan detail unit.
  - `purchase_price`/data biaya unit **tidak diekspos** di API baca ini (visibilitas biaya belum diatur di matriks 5.1; putuskan saat dibutuhkan).
- [x] **T1.5 API LPB draft** — create/update/detail/list, tambah-hapus SN scan (validasi Bagian 5.4), submit, cancel, revise.
  Keputusan T1.5 (plan tidak merinci; praktik terbaik yang konsisten dengan Bagian 5.2–5.4):
  - Endpoint: `GET/POST /lpb`, `GET/PATCH /lpb/:id`, `POST /lpb/:id/items/:itemId/serials`, `DELETE /lpb/:id/items/:itemId/serials/:serialId` (id baris scan), `POST /lpb/:id/{submit,cancel,revise}`. Baca `lpb.read` (MGMT/STAFF/HEAD), tulis `lpb.create`, ajukan `lpb.submit`. `approve/reject` + antrean `/lpb/approvals` = **T1.6**. **Tahap draft tidak pernah mengubah stok, ledger, maupun status unit** (invarian #1) — diuji.
  - **Jenis yang dapat dibuat manual**: `PURCHASE`, `INITIAL_STOCK`, `DISPOSAL`. Jenis lain lahir dari proses lain (DO, tiket, opname, pasangan transfer) → ditolak 422 dengan penjelasan; `TRANSFER_OUT` menunggu T1.8 (butuh rancangan rak/gudang tujuan karena skema belum menyimpannya).
  - **Penomoran**: `LPBI` (masuk) / `LPBO` (keluar) + kode gudang, lewat `docnum` di transaksi pembuatan; dokumen dibatalkan tetap memegang nomornya, pembuatan gagal tidak menghabiskan nomor. `doc_date` bawaan hari ini (WIB), tidak boleh di masa depan.
  - **Aturan header**: `PURCHASE` wajib `supplier_id` (aktif), jenis lain tidak boleh; `reference_number` pembelian → `reference_type = PURCHASE_INVOICE`; biaya satuan (`unit_cost`, string desimal) hanya untuk penerimaan pembelian/stok awal. Jenis dan gudang **tidak dapat diubah** setelah dibuat (batalkan lalu buat baru). Draft boleh tanpa item; minimal 1 item wajib saat submit. Maks 200 item.
  - **Item**: produk & rak aktif, rak milik gudang LPB, tidak dikunci opname (409 `RACK_LOCKED_BY_OPNAME`), (produk, rak) unik. `PATCH` dengan `items` **menyelaraskan** daftar: item dengan (produk, rak) sama dipertahankan beserta scan-nya; qty tidak boleh di bawah jumlah scan; item yang tidak dikirim dihapus beserta scan-nya.
  - **Scan SN**: SN dinormalisasi (pangkas + huruf besar; dibandingkan tanpa peduli huruf); tidak melebihi qty; produk non-SN ditolak. `PURCHASE/INITIAL_STOCK`: SN tidak boleh sudah terdaftar untuk produk itu (409 `SERIAL_EXISTS`); kondisi `BROKEN` hanya di rak non-sellable. `DISPOSAL`: scan **asset tag atau SN** unit terdaftar, harus `BROKEN` dan berada di rak item (422 `SERIAL_NOT_VALID`), SN dan asset tag diambil dari unit. SN yang sudah ada di LPB `DRAFT/PENDING_APPROVAL` lain → 409 `SERIAL_IN_OTHER_LPB` (menyebut nomor LPB-nya). Aturan ini **bukan constraint DB**, jadi scan diserialkan per (produk, SN) dengan `pg_advisory_xact_lock` (tingkat *transaksi*, aman untuk transaction pooler Supabase; terbukti oleh tes 6 scan paralel → tepat 1 berhasil).
  - **Kepemilikan**: draft milik pembuatnya; HEAD_WAREHOUSE dan ADMIN boleh membantu (403 `FORBIDDEN` untuk staf lain); gudang lain → 403 `FORBIDDEN_WAREHOUSE`. Baca dibatasi ke gudang akun.
  - **Submit** (`DRAFT → PENDING_APPROVAL`): minimal 1 item (422 `LPB_EMPTY`); produk SN: jumlah scan = qty (422 `SERIAL_COUNT_MISMATCH`, pesan memuat "ter-scan X dari Y"); non-SN keluar: peringatan dini stok tersedia (409 `INSUFFICIENT_STOCK`); seluruh aturan scan diperiksa **ulang** (SN baru terdaftar, rak baru dikunci, produk dinonaktifkan, unit disposal sudah bukan BROKEN). Header di-lock `FOR UPDATE` → klik ganda paralel: tepat 1 berhasil. `cancel`: hanya `DRAFT`; `revise`: hanya `REJECTED → DRAFT` (alasan penolakan tetap terlihat sampai diajukan ulang, lalu dibersihkan).
  - **Detail** memuat header, item (+ serial dan `scanned_count`), dan **timeline** dari `audit_logs` (`LPB_CREATE/UPDATE/SUBMIT/CANCEL/REVISE` beserta pelaku). Audit memakai pelaku dari context pengguna terautentikasi.
  - Perbaikan yang ditemukan sepanjang T1.5: (1) audit manajemen user/data master sebelumnya **tidak mencatat pelaku** — sekarang otomatis dari `reqctx.UserIDFrom`; (2) CI integrasi gagal pada batas bawaan `go test -timeout 10m` karena tes login bcrypt berulang — tes kini menerbitkan JWT langsung (paket HTTP: >3 menit → ±60 detik) dan CI memakai `-timeout 30m`.
- [x] **T1.6 ApproveLPB INBOUND** — `PURCHASE`, `INITIAL_STOCK` (+ pembuatan asset tag), reject. Ledger + histori SN + audit.
  Keputusan T1.6 (mengikuti Bagian 6.2; hal yang tidak dirinci diputuskan dengan praktik terbaik):
  - Endpoint: `GET /lpb/approvals` (antrean PENDING di gudang approver, tertua dulu; filter `warehouse_id`, `sub_type`), `POST /lpb/:id/approve` (**wajib `Idempotency-Key`**), `POST /lpb/:id/reject` `{reason}`. Permission `lpb.approve` (HEAD_WAREHOUSE + ADMIN). Jenis yang efek stoknya belum ada (mis. `DISPOSAL`, T1.8) ditolak 422 `sub_type` tanpa mengubah apa pun.
  - **Approve = satu transaksi atomik**: lock header `FOR UPDATE` → gudang & rak `FOR SHARE` (rak tidak dapat dikunci opname/dinonaktifkan selama approve) → saldo per (produk, rak) **urut** → unit baru. Validasi dilakukan setelah lock: status `PENDING_APPROVAL` (409 `LPB_NOT_PENDING`), akses gudang approver (403), **approver ≠ pembuat** kecuali `app_settings.lpb.allow_self_approve = true` (403 `SELF_APPROVE_FORBIDDEN`, bawaan false bila kunci tidak ada), minimal 1 item, rak tidak dikunci opname (409), produk/rak/gudang aktif, jumlah SN = qty.
  - **Efek**: saldo di-upsert (`qty_on_hand += qty`); produk SN → unit baru (`AST-{YYMM}-{NNNNN}` lewat `docnum.NextAssetTag`, diberikan **urut serial number**), status `AVAILABLE` (kondisi `BROKEN` → `BROKEN` dan rak wajib non-sellable), `supplier_id`/`purchase_date`/`purchase_price` dari dokumen, baris scan dikaitkan ke unit, **satu riwayat unit per unit** (`LPB_APPROVED`, pelaku = approver); **satu baris ledger per item** (`IN`, saldo sebelum/sesudah, `serial_ids`, biaya satuan, `created_by` = approver); dokumen → `APPROVED` + `verify_token` acak 32 byte (base64url) untuk QR; audit `LPB_APPROVE` dengan ringkasan saldo sebelum→sesudah per item.
  - **Gagal di mana pun = tidak ada perubahan apa pun** (saldo, unit, ledger, riwayat, nomor asset tag — nomor tidak habis). Diuji dengan kegagalan di tengah proses (SN baru didaftarkan pihak lain setelah submit) dan dengan `pgtest.AssertInvariants` (I1–I6 Bagian 5.9, kini helper baku untuk setiap test yang mengubah stok).
  - **Lock order terbukti**: item diproses urut (produk, rak); tes dua dokumen dengan urutan item berlawanan diproses paralel → semua berhasil; dengan pengurutan dimatikan terjadi `deadlock detected` (diverifikasi lewat mutation check).
  - **Idempotency** (`middleware.Idempotency`, tabel `idempotency_keys`, TTL 24 jam, per (kunci, pengguna)): kunci 8–100 karakter `[A-Za-z0-9._:-]`; permintaan pertama diproses dan respons final (2xx/4xx) disimpan; ulang → respons yang **sama** tanpa menjalankan ulang + header `Idempotency-Replayed: true`; ulang saat yang pertama masih berjalan → 409 `IDEMPOTENCY_IN_PROGRESS`; kunci yang sama untuk permintaan lain → 422 `IDEMPOTENCY_KEY_REUSED`; respons 5xx/panik tidak disimpan (kunci dibebaskan agar dapat diulang). Disimpan di database (aman untuk serverless). Dipasang setelah `RequirePermission`. Catatan: body disimpan sebagai `jsonb` sehingga urutan kunci JSON pada respons ulang dapat berbeda (isi sama). **Belum ada job pembersih** `idempotency_keys` kedaluwarsa (kunci yang dipakai lagi dibuang otomatis).
  - **Reject**: alasan 3–500 karakter; `PENDING_APPROVAL → REJECTED` (+`rejected_by/at/reason`), tidak mengubah stok; pembuat merevisi (`REJECTED → DRAFT`) lalu mengajukan ulang. Audit `LPB_REJECT` memuat alasan.
  - **Detail LPB** kini memuat per item `current_qty` (saldo rak saat ini, untuk tampilan "sebelum → sesudah" di halaman approval T1.7) dan, setelah APPROVED, `balance_before`/`balance_after` dari ledger.
  - `advisory lock` tingkat transaksi dipakai pada scan SN (T1.5); invarian #9 hanya melarang advisory lock *session*.
  DoD: test integrasi: approve → saldo, SN, ledger, histori sesuai; SN dobel → 409 & rollback total.
- [x] **T1.7 UI LPB** — daftar (filter status), form + `SerialScannerPOS` (perbaikan Bagian 9.3), detail + timeline, halaman Approval (tampilkan saldo sebelum → sesudah per item).
  Keputusan T1.7 (plan hanya menyebut rute dan komponen; praktik terbaik UX proyek yang konsisten dengan Bagian 9):
  - Rute: `/lpb` (**List**), `/lpb/baru` dan `/lpb/:id/ubah` (**Form**, hanya draft; LPB non-draft → pesan jelas), `/lpb/:id` (**Detail**), `/lpb/approvals` (**List**, antrean tertua dulu). `/lpb/:id/ubah` ditambahkan di luar tabel 9.4 karena draft harus dapat diubah. Menu sidebar: hanya satu item aktif (awalan terpanjang) — `/lpb/approvals` tidak lagi menyalakan "LPB".
  - **Alur**: form membuat draft (header + item) → halaman rincian untuk **scan SN** (id item baru ada setelah draft tersimpan) → Ajukan. Form: jenis (hanya `PURCHASE`, `INITIAL_STOCK`; `DISPOSAL` baru ditawarkan di T1.8 karena belum dapat disetujui), gudang (akun satu gudang hanya melihat gudangnya), supplier + referensi hanya untuk pembelian, baris item (produk, rak milik gudang terpilih, jumlah, biaya satuan) sebagai kartu per baris, pasangan (produk, rak) kembar ditolak di klien, tanggal tidak boleh di masa depan (WIB). Mengganti gudang mengosongkan pilihan rak.
  - **`SerialScannerPOS`** (+ `lib/audio.ts`, `composables/useScanQueue.ts`): `AudioContext` tunggal yang di-`resume()` pada gestur pertama dan boleh dimatikan (suara hanya pelengkap — hasil selalu juga berupa ikon + teks di `role="log"`); kolom isian selalu terfokus (alat scan = papan ketik + Enter), kode dinormalisasi (pangkas + huruf besar, sama dengan backend); **dobel dikenali lokal** (sudah tercatat atau sedang diantre) tanpa memanggil API; kapasitas item dijaga lokal; **mode batch**: scan berikutnya tidak menunggu, maksimal **3 permintaan paralel**; item penuh → pindah otomatis ke item berikutnya; kondisi unit dibawa per scan. Scan hanya untuk draft milik pengguna (atau Kepala Gudang/ADMIN, sama dengan backend).
  - **Approval**: keputusan dibuat di halaman rincian (bukan di antrean) agar dampak stok terlihat: tabel **saldo rak sekarang → setelah disetujui** (prakiraan, dari `current_qty`) dan setelah APPROVED angka final dari ledger (`balance_before → balance_after`). Layar sempit memakai **kartu per item** (kolom saldo di tabel tersembunyi di luar layar — ditemukan lewat uji browser 375px). **Setujui** = dialog konfirmasi yang menyebut dokumen (nada primer, fokus awal di Batal) + `Idempotency-Key` dibuat **sekali per dialog** dan dipakai ulang bila mengulang setelah galat jaringan (tes: kunci sama; dialog baru → kunci baru). **Tolak** = panel samping satu isian (alasan 3–500 karakter, blur-first; galat tidak terhapus oleh fokus terprogram).
  - Riwayat dokumen dari timeline audit; daftar memakai kolom "Dibuat" (tanggal + pembuat) sebagai kolom terurut bawaan (terbaru dulu) agar urutan terlihat lewat `aria-sort`. Filter: status, jenis, "dokumen saya".
  - Perbaikan komponen bersama: `useBlurField` menerima nama field reaktif (baris item dinamis), `TextField` mendukung `type="date"`, `ConfirmDialog` punya `tone="primary"`, `FormPage` punya `wide`, `useEntityForm` punya `afterSave`, `listAll` (maks 5×100 data) untuk pilihan produk/supplier/rak.
  - Batasan yang diketahui: pilihan produk/supplier/rak maksimal 500 data per daftar (kolom pencarian pilihan belum ada); filter gudang pada daftar LPB belum ada; `DISPOSAL`/`TRANSFER_OUT` di UI menunggu T1.8; toast tampil di kanan atas dan dapat menutupi tombol aksi selama beberapa detik.
  - Pengujian: 270 tes Vitest (unit logika scan/audio/saldo, view daftar/form/rincian/approval, service); uji browser end-to-end terhadap backend dan database sungguhan di **1280px dan 375px** — alur penuh staf (buat → scan batch → ajukan) → Kepala Gudang (tolak) → staf (revisi, ubah, ajukan ulang) → Kepala Gudang (setujui), stok awal dengan saldo sebelumnya (6 → 10), manajemen read-only — tanpa overflow horizontal dan semua target sentuh ≥ 44px. Saldo, ledger, unit, dan audit di database cocok dengan layar. Mutation check: kunci idempotensi per klik membuat tes gagal.

**Sprint 3 — LPB keluar & PDF**
- [x] **T1.8 ApproveLPB OUTBOUND manual** — `TRANSFER_OUT/IN` (pasangan, auto-approve satu gudang), `DISPOSAL`; lock order Bagian 6.
  Keputusan T1.8 (skema belum menyimpan tujuan transfer; dirancang dengan praktik terbaik yang konsisten dengan Bagian 5.2 dan 6.2):
  - **Migrasi 000005** (baru, bukan edit migrasi lama): `lpb_items.dest_rack_id` (rak tujuan, `CHECK ≠ rack_id`) dan indeks `counterpart_lpb_id`. Nomor 000005/000006 yang disebut sebagai "FK di 000005" pada skema Bagian 8 kini bergeser ke migrasi berikutnya.
  - **Dibuat manual**: `TRANSFER_OUT` (tiap item wajib `dest_rack_id`; boleh di gudang lain; **satu dokumen = satu gudang tujuan**; rak tujuan ≠ rak asal; rak tujuan aktif, di gudang aktif, tidak dikunci opname) dan `DISPOSAL`. Produk **berserial hanya boleh dipindah ke rak yang dapat dijual** (unit tiba berstatus AVAILABLE; AVAILABLE di rak non-jual merusak hitungan stok tersedia). Pemeriksaan sama diulang saat submit dan saat approve (setelah lock).
  - **Scan** (`TRANSFER_OUT`/`DISPOSAL`): asset tag atau SN unit **terdaftar**, harus berada di rak item dengan status `AVAILABLE` (transfer) / `BROKEN` (penghapusan) → selain itu 422 `SERIAL_NOT_VALID`. Stok bebas non-SN diperiksa saat submit memakai `FreeQty` (on hand − reserved, **tanpa** syarat rak dapat dijual — barang di rak karantina tetap boleh dipindah/dihapus).
  - **Approve `DISPOSAL`**: saldo rak asal berkurang; unit `BROKEN → RETIRED` (tanpa rak/gudang); ledger `OUT` + riwayat unit (`LPB_APPROVED`).
  - **Approve `TRANSFER_OUT`**: saldo rak asal berkurang, unit `AVAILABLE → IN_TRANSIT`, dan **LPB pasangan `TRANSFER_IN` dibuat otomatis** (nomor `LPBI/{gudang tujuan}`, dibuat atas nama pembuat OUT, diajukan oleh penyetuju, saling menaut lewat `counterpart_lpb_id`; item dari beberapa rak asal ke rak tujuan yang sama digabung satu baris). **Satu gudang** (pindah rak): pasangan langsung disetujui dalam transaksi yang sama — unit `IN_TRANSIT → AVAILABLE` di rak tujuan, ledger `IN`, dua riwayat per unit. **Antar gudang**: pasangan berstatus `PENDING_APPROVAL` di gudang tujuan (muncul di antrean Kepala Gudang tujuan; gudang lain 403 `FORBIDDEN_WAREHOUSE`); saat disetujui saldo/unit pindah ke rak tujuan.
  - **`TRANSFER_IN` tidak dapat ditolak** (422): barang sudah keluar dari gudang asal. Barang yang tidak tiba dikoreksi lewat selisih stock opname (T1.12). Re-scan fisik saat tiba (disebut Bagian 5.2) **belum** ada — pasangan membawa unit yang sama dari sisi keluar dan disetujui sebagai konfirmasi kedatangan; dicatat sebagai batasan.
  - **Lock order** (dokumen → gudang/rak `FOR SHARE` urut id → saldo → unit): perubahan saldo dilakukan **berurutan (produk, rak; keluar sebelum masuk)** lalu semua unit dikunci sekaligus urut id (dua tahap, bukan per item). Catatan jujur: persetujuan transfer satu gudang juga diserialkan oleh baris urutan nomor dokumen (`docnum`) yang dikunci lebih dulu, sehingga deadlock lewat HTTP tidak dapat dipancing; pengurutan saldo diuji langsung di tingkat unit (`TestApplyMoves_TouchesBalancesInProductRackOrder`) dan terbukti lewat mutation check (urutan dimatikan → tes gagal).
  - **Atomik**: gagal di mana pun (mis. stok tujuan/asal habis oleh dokumen lain, unit sudah berubah) = tidak ada perubahan: saldo, unit, ledger, **dan dokumen pasangan** kembali seperti semula (diuji).
  - API: item menerima `dest_rack_id`; respons item memuat `dest_rack_id/code`, `dest_warehouse_id/code`; respons LPB memuat `counterpart_id/number`. Audit approve memuat ringkasan saldo dan nomor pasangan; pasangan mendapat audit `LPB_CREATE`/`LPB_SUBMIT`/`LPB_APPROVE` sendiri.
  - **UI**: form menawarkan Transfer keluar (rak asal + rak tujuan lintas gudang) dan Penghapusan; scanner memakai mode unit terdaftar (asset tag atau SN, tanpa kondisi); rincian menampilkan tujuan, tautan pasangan (dan kembali), dialog setujui yang menjelaskan efeknya per jenis, tanpa tombol Tolak untuk transfer masuk.
  - Temuan sepanjang T1.8 (diperbaiki): (1) membuka LPB pasangan lewat tautan tidak memuat ulang data (komponen rincian dipakai ulang) — kini memantau `id` rute; (2) galat validasi 422 dari server tampil sebagai "Data LPB tidak valid" — kini rincian per field yang ditampilkan (scanner dan toast aksi); (3) field biaya yang tidak dirender dibuang vee-validate sehingga validasi gagal diam-diam untuk transfer/penghapusan — dibuat opsional.
  - Pengujian: tes integrasi baru (penghapusan SN dan non-SN dari karantina, stok habis di tengah → rollback, scan hanya unit BROKEN di rak item, transfer satu gudang + pasangan otomatis + ledger + riwayat, penggabungan item, antar gudang + antrean + 403 + tolak ditolak + persetujuan tujuan, validasi, rollback total termasuk dokumen pasangan, transfer berlawanan serentak) + tes unit urutan saldo; 286 tes Vitest; uji browser 1280px dan 375px terhadap backend dan database nyata (transfer satu gudang, antar gudang oleh Kepala Gudang B, penghapusan) tanpa overflow dan semua target sentuh ≥ 44px.
  DoD (terpenuhi): transfer satu gudang menghasilkan dua dokumen APPROVED dengan ledger di kedua sisi; DISPOSAL menghapus unit BROKEN; kegagalan = rollback total.
- [x] **T1.9 Test konkurensi & invariant** — helper `assertInvariants`, skenario paralel Bagian 11.
  Catatan T1.9:
  - `internal/testutil/pgtest/invariants.go`: menambahkan `CheckInvariants` (diekspos agar dapat diuji langsung), memperluas invarian dengan `I1b` (unit fisik SN di rak tanpa baris saldo) dan `I5b` (rantai ledger per `(product_id, rack_id)` berkesinambungan tanpa celah `LAG(balance_after) = balance_before`). Dilengkapi `invariants_integration_test.go` untuk memastikan pelanggaran `I1`, `I1b`, `I2`, `I3`, `I5b` benar-benar terdeteksi.
  - `internal/delivery/http/lpb_concurrency_integration_test.go`: 6 skenario balapan paralel dengan barrier (`close(start)`): approve paralel banyak LPB OUT pada stok non-SN sama (tepat 1 menang, saldo tidak negatif), balapan `TRANSFER_OUT` vs `DISPOSAL` pada stok sama (tanpa pasangan `TRANSFER_IN` yatim), campuran `PURCHASE` + `DISPOSAL` paralel pada saldo sama (rantai ledger utuh), balapan `approve` vs `reject` pada LPB sama, approve paralel `TRANSFER_IN` antar gudang, dan approve paralel dua `PURCHASE` dengan SN sama (`SERIAL_EXISTS` + rollback penuh). Skenario reservasi SN DO paralel diuji di Fase 2 (`T2.6`).
  - Verifikasi: `go vet -tags=integration ./...` bersih; 12 tes konkurensi lulus **`-count=20`** (206s) + seluruh suite integrasi backend lulus terhadap PostgreSQL 16 lokal. Catatan: `-race` butuh CGO/`gcc` yang belum terpasang di Windows lokal; CI GitHub Actions (Linux) menjalankan `go test -race`.
  DoD: `go test -race` lulus 20x berulang.
- [x] **T1.10 PDF Surat Jalan** — Gotenberg client, template embed, QR token, `GET /lpb/:id/pdf`, `GET /public/lpb/verify/:token`, halaman `/verify/lpb/:token`.
  Catatan T1.10:
  - Backend: `pkg/qrcode` (`DataURI` PNG base64 via `skip2/go-qrcode`), `pkg/pdf` (`GotenbergClient` POST `/forms/chromium/convert/html` timeout 30s + basic auth + `A4Options`, interface `Renderer`), `templates/lpb_surat_jalan.html` + `embed.go` (`//go:embed`, auto-escape `html/template`).
  - Endpoint: `GET /api/v1/lpb/:id/pdf` (permission `lpb.print`; non-APPROVED → **422 `LPB_NOT_APPROVED`**; DRIVER hanya LPB yang ditugaskan kepadanya; `?download=1` → `attachment`; Gotenberg down → **503 `PDF_SERVICE_UNAVAILABLE`**) dan `GET /api/v1/public/lpb/verify/:token` (publik tanpa login; hanya ringkasan nomor, jenis, tanggal, gudang, supplier, jumlah item/qty, approver & waktu approve — tanpa harga/SN/ID internal).
  - Frontend: tombol **"Cetak Surat Jalan"** pada `LpbDetailView.vue` (tampil saat `APPROVED` & punya `lpb.print`, ambil blob via Bearer token di memori) dan halaman publik `LPBVerifyView.vue` di `/verify/lpb/:token`.
  - Verifikasi: unit test `pkg/qrcode`, `pkg/pdf`, `templates`; integrasi `TestLPB_PDF_And_PublicVerify` (fake Gotenberg + RBAC + 422 non-APPROVED + 503 + verifikasi publik); 288 tes Vitest, `check:lint`, `type-check`, `vite build` lulus.
  DoD: PDF A4 rapi, QR dari HP membuka halaman verifikasi; LPB non-APPROVED → 422.
- [x] **T1.11 Label** — PDF label asset tag (batch) & label kode rak.
  Catatan T1.11:
  - Template `templates/label_asset.html` (`//go:embed`, lembar A4 grid 3 kolom dengan QR per stiker, garis potong, dan badge `NON-SELLABLE` untuk rak karantina/workshop).
  - Endpoint: `POST /api/v1/racks/labels/pdf` (`master.read`, body `{rack_ids}`, maks 200) dan `POST /api/v1/inventory/serials/labels/pdf` (`inventory.read`, body `{serial_ids}`, maks 200); keduanya menegakkan warehouse scoping, mendukung `?download=1`, dan mengembalikan `503 PDF_SERVICE_UNAVAILABLE` bila Gotenberg tidak tersedia.
  - Frontend: tombol **"Cetak Label"** per baris di `RackListView.vue` dan tombol **"Cetak Label Aset"** batch di `LpbDetailView.vue` untuk LPB `APPROVED` yang memiliki unit SN terdaftar (helper bersama `src/lib/pdf.ts`).
  - Verifikasi: `TestRenderLabels`, `TestLabels_RackAndSerialBatchPDF` (RBAC, scoping, validasi, 200 PDF, 503), `check:lint`, 288 Vitest, `type-check`, dan `vite build` lulus.

**Sprint 4 — Opname, laporan stok, dashboard gudang**
- [x] **T1.12 Stock opname** — mulai (kunci rak + snapshot), hitung non-SN, scan SN (FOUND/MISSING/UNEXPECTED/WRONG_RACK), submit, finalize (LPB adjustment auto-approve), cancel; UI mode HP/tablet.
  Catatan T1.12:
  - Backend (`domain/opname.go`, `postgres/opname_repo.go`, `usecase/opname.go`, `handler/opname.go`): `GET/POST /inventory/opnames`, `GET /inventory/opnames/:id`, `PUT /inventory/opnames/:id/counts`, `POST /inventory/opnames/:id/scans`, `POST /inventory/opnames/:id/{submit,finalize,cancel}`.
  - Mulai opname mengunci rak (`racks.locked_by_opname_id`, LPB/opname lain pada rak terkunci → **409 `RACK_LOCKED_BY_OPNAME`**), men-snapshot saldo non-SN (>0) dan unit SN fisik (awal `MISSING`). Mode `is_blind_count=true` menyembunyikan `system_qty`/`diff_qty` dari staf gudang selama `IN_PROGRESS`.
  - Finalize (`SUBMITTED → FINALIZED`) berjalan atomik dalam 1 transaksi: membuat & auto-approve `OPNAME_ADJUSTMENT_IN` (surplus non-SN + unit `LOST` yang ditemukan kembali → `AVAILABLE`) dan/atau `OPNAME_ADJUSTMENT_OUT` (kekurangan non-SN + unit `MISSING` → `LOST`), menulis `stock_ledgers` + `product_serial_histories` + `audit_logs`, lalu membuka kunci rak. Cancel membuka kunci rak tanpa mengubah stok.
  - Frontend: `OpnameListView.vue` (daftar + slide-over mulai opname pilih rak & blind count) dan `OpnameDetailView.vue` (responsif HP/tablet: tab Hitung Non-SN + tab Scan SN dengan umpan balik audio `beep()`, tombol Ajukan/Finalisasi/Batalkan, tautan LPB penyesuaian). Menu sidebar "Stock Opname" muncul otomatis.
  - Verifikasi: `TestOpname_FullLifecycle_LockAdjustInvariantsAndRediscoverLost` & `TestOpname_Cancel_UnlocksRacksWithoutChangingStock` lulus dengan `pgtest.AssertInvariants` = 0 pelanggaran; `check:lint` (0 error), **289/289 Vitest**, `type-check`, dan `vite build` lulus.
  DoD: LPB lain ke rak terkunci → 409; setelah finalize invariant = 0 baris.
- [x] **T1.13 Ledger, kartu stok, riwayat SN** — API + UI + export Excel.
  Catatan T1.13:
  - Backend (`usecase/inventory_ledger.go`, `postgres/inventory_repo.go`, `handler/inventory.go`, `excelize/v2`):
    - `GET /api/v1/inventory/serials/:id/history` (`inventory.read`) — detail unit + timeline `product_serial_histories` lengkap (dengan warehouse scoping).
    - `GET /api/v1/inventory/ledgers` (`ledger.read`) — daftar buku besar mutasi stok terpaginasi (filter produk/rak/gudang/arah/jenis/rentang tanggal WIB/cari) + `GET /api/v1/inventory/ledgers/export` (`.xlsx`).
    - `GET /api/v1/inventory/stock-card` (`ledger.read`) — kartu stok per produk/rak/periode (saldo awal, total masuk, total keluar, saldo akhir, daftar mutasi kronologis) + `?format=xlsx` untuk export Excel.
  - Frontend: 5 halaman gudang lengkap — `/stok` (`StockBalanceListView`), `/stok/serial` (`SerialListView`), `/stok/serial/:id` (`SerialDetailView` + timeline + cetak label), `/stok/buku-besar` (`StockLedgerView` + export Excel), dan `/stok/kartu-stok` (`StockCardView` + export Excel). Menu sidebar **Stok**, **Serial Number**, dan **Buku Besar** otomatis aktif.
  - Verifikasi: `TestLedger_StockCard_SerialHistory_And_ExcelExport` lulus (termasuk parsing ulang file `.xlsx` hasil export); `go mod tidy` bersih; `check:lint` (0 error), **289/289 Vitest**, `type-check`, dan `vite build` lulus.
- [x] **T1.14 Dashboard gudang** — metrik Bagian 9.5 (gudang).
  Catatan T1.14:
  - Backend (`domain/dashboard.go`, `postgres/dashboard_repo.go`, `usecase/dashboard.go`, `handler/dashboard.go`): `GET /api/v1/dashboard/summary` (permission `dashboard.read`, mendukung `?warehouse_id=` + warehouse scoping). Mengembalikan jumlah produk aktif/aman/menipis, total qty tersedia, unit SN tersedia/aktif, antrean LPB `PENDING_APPROVAL`, stock opname berjalan, `pending_do_reservations` (0 di Fase 1), top 5 produk menipis, 5 antrean LPB tertua, dan 5 mutasi ledger terbaru.
  - Frontend: `DashboardView.vue` di `/dashboard` (menu **Dashboard** di sidebar aktif otomatis) dengan 4 kartu KPI, filter gudang, daftar peringatan stok menipis, antrean LPB, dan mutasi terbaru. `homeFor` di `guards.ts` diperkuat agar memeriksa permission rute `default_home` sebelum redirect (fallback aman ke `/beranda`).
  - Verifikasi: `TestDashboard_WarehouseSummary`, `check:lint` (0 error), **289/289 Vitest**, `type-check`, dan `vite build` lulus.
- [x] **T1.15 Import stok awal** — upload Excel/CSV (template disediakan) → draft LPB `INITIAL_STOCK` → approve.
  Catatan T1.15:
  - Backend (`usecase/lpb_import.go`, `handler/lpb.go`, `router.go`): `GET /api/v1/lpb/import-initial-stock/template` (`.xlsx` berisi sheet `Stok Awal` + contoh + sheet `Petunjuk`) dan `POST /api/v1/lpb/import-initial-stock` (`lpb.create`, multipart `.xlsx`/`.csv` maks 5 MB, dukung pemisah `,` dan `;`). Memvalidasi semua baris (SKU aktif, rak milik gudang & tidak dikunci opname, SN unik & belum terdaftar/diklaim LPB lain, `BROKEN` hanya di rak non-sellable, desimal biaya) — bila ada 1 baris salah, tolak 422 dengan rincian `row[N].<kolom>` tanpa menulis sebagian. Bila valid, buat 1 `DRAFT` LPB `INITIAL_STOCK` + seluruh item & SN scan dalam 1 transaksi (tetap wajib diajukan & disetujui Kepala Gudang).
  - Frontend: tombol **"Import Stok Awal (Excel/CSV)"** + slide-over di `LpbListView.vue` (unduh template, pilih gudang/tanggal/file, tampilkan galat per baris bila 422, sukses langsung buka rincian draft LPB).
  - Verifikasi: `TestLPB_ImportInitialStock_TemplateCSVXLSXAndValidation` (download template, import CSV non-SN + SN → submit → approve → `pgtest.AssertInvariants` = 0, validasi 422), `check:lint` (0 error), **289/289 Vitest**, `type-check`, dan `vite build` lulus.
  **🎯 Milestone M1 (TERCAPAI):** seluruh Inti Gudang (Fase 1: `T1.1`–`T1.15`) selesai!

### Fase 2 — Komersial (Sprint 5–7)

**Sprint 5 — Pelanggan & kontrak**
- [x] **T2.1 Migrasi `000005`–`000007`** (+ down) + seed `tax_settings`.
  Catatan T2.1:
  - Penomoran file migrasi bergeser +1 karena `000005_lpb_transfer_destination` sudah dipakai di T1.8 → `000006_init_customers_contracts` (`customers`, `customer_sites`, `rental_contracts`, `rental_contract_items`, `rental_placements` + FK ke `product_serials`/`lpb_documents`), `000007_init_sales_delivery` (`sales_orders`, `sales_order_items`, `delivery_orders`, `delivery_order_items`, `delivery_order_reservations` + FK ke `lpb_items`/`product_serials`), dan `000008_init_billing` (`tax_settings`, `invoices`, `invoice_items`, `payments`). Total 39 tabel; `up → down → up` bersih tanpa residu.
  - `internal/seed/seed.go` menambahkan seed idempoten `tax_settings` PMK 131/2024 (`PPN`, `rate=12.00`, `dpp_numerator=11`, `dpp_denominator=12`, `faktur_code='04'`, `effective_from='2025-01-01'`).
  - Verifikasi: `TestMigrations_UpDownUp_LeavesNoResidue` (39 tabel), `TestMigrations_Phase2CommercialConstraints` (`ON DELETE RESTRICT`, site utama unik, `chk_so_contract`, `uq_invoice_rental_period`, `chk_payment_positive`), `internal/seed`, dan `TestInventory_` lulus; migrasi & seed lokal selesai.
- [x] **T2.2 Pelanggan & site** — CRUD, picker koordinat (klik peta Leaflet / input manual), site utama unik.
  Catatan T2.2:
  - Backend (`domain/customer.go`, `postgres/customer_repo.go`, `usecase/customer.go`, `dto/customer.go`, `handler/customer.go`, `router.go`, `app.go`): `GET/POST /api/v1/customers`, `GET/PATCH /api/v1/customers/:id`, `GET/POST /api/v1/customers/:id/sites`, `PATCH /api/v1/customer-sites/:id`, dan `GET /api/v1/customers/:id/assets` (daftar unit `RENTED` terpasang per pelanggan/site). Otorisasi `customer.read` (`MANAGEMENT`, `BUSINESS_ADMIN`, `TECHNICIAN`, `ADMIN`) & `customer.write` (`BUSINESS_ADMIN`, `ADMIN`).
  - Aturan bisnis: `customer_code` unik & immutable, validasi pasangan koordinat `latitude`/`longitude`, pergantian `is_primary=true` dikunci per baris pelanggan (`FindByIDForUpdate`) lalu menurunkan site utama lama secara atomik dalam 1 transaksi (menjaga `uq_customer_primary_site`), menonaktifkan site utama otomatis membersihkan `is_primary`, dan pelanggan/site yang masih memiliki unit `RENTED` tidak dapat dinonaktifkan (422). Semua perubahan tercatat di `audit_logs`.
  - Frontend: `types/customer.ts`, `services/customerService.ts`, `components/CoordinatePickerMap.vue` (Leaflet + OpenStreetMap click-to-pin + sinkronisasi input manual lat/lng), `views/customers/CustomerListView.vue` (`/pelanggan`), `CustomerFormView.vue` (`/pelanggan/baru`, `/pelanggan/:id/ubah`), dan `CustomerDetailView.vue` (`/pelanggan/:id` dengan tab Cabang/Site, Aset Terpasang, dan Profil & Perpajakan). Menu sidebar **Pelanggan** aktif otomatis.
  - Verifikasi: `go vet -tags=integration ./...` bersih, 4 tes integrasi (`TestCustomer_RBAC`, `TestCustomer_CRUDAndValidation`, `TestCustomer_SitesPrimaryAndCoordinates`, `TestCustomer_InstalledAssetsAndDeactivateGuard`) lulus; frontend `check:lint` (0 error), **291/291 Vitest** (24 file), `vue-tsc --build`, dan `vite build` lulus.
- [x] **T2.3 Kontrak** — header + item tarif, activate/terminate/renew, validasi tanggal & durasi.
  Catatan T2.3:
  - Backend (`domain/contract.go`, `postgres/contract_repo.go`, `usecase/contract.go`, `dto/contract.go`, `handler/contract.go`, `router.go`, `app.go`): `GET/POST /api/v1/contracts`, `GET/PATCH /api/v1/contracts/:id`, `POST /api/v1/contracts/:id/{activate,terminate,renew}`. Nomor kontrak `KTR/GLOBAL/{YYYY}/{MM}/{NNNN}` via `docnum`. Otorisasi `contract.read`, `contract.write`, `contract.approve`.
  - Aturan bisnis: hanya produk bertipe `ASSET` aktif; site opsional harus milik pelanggan & aktif; kombinasi `(product_id, customer_site_id)` unik dalam kontrak; `end_date > start_date` (auto-hitung `start_date + duration_months - 1 hari` bila kosong); hanya `DRAFT` dapat diubah/diaktifkan (non-DRAFT → `409`); `terminate` hanya untuk `ACTIVE` dengan alasan wajib (5–500 karakter) & `terminated_at >= start_date`; `renew` (`ACTIVE`/`EXPIRED`) membuat `DRAFT` baru (`renewed_from_id`) menyalin header & seluruh item tarif mulai `end_date + 1 hari`. Semua perubahan tercatat di `audit_logs`.
  - Frontend: `types/contract.ts`, `services/contractService.ts`, `views/contracts/ContractListView.vue` (`/kontrak`, filter status & akan berakhir ≤30/60 hari), `ContractFormView.vue` (`/kontrak/baru`, `/kontrak/:id/ubah`, auto-hitung `end_date` & tarif bawaan produk), `ContractDetailView.vue` (`/kontrak/:id`, aksi Aktifkan, Terminasi, Perpanjang). Menu sidebar **Kontrak** aktif otomatis.
  - Verifikasi: `go vet -tags=integration ./...` bersih, `TestContract_RBACAndValidation` & `TestContract_Lifecycle_DraftUpdateActivateRenewTerminate` lulus; frontend `check:lint` (0 error), **293/293 Vitest** (25 file), `vue-tsc --build`, dan `vite build` lulus.

**Sprint 6 — SO, DO, reservasi**
- [ ] **T2.4 `pkg/money` + kalkulator pajak** — rumus Bagian 5.6 dengan `tax_settings` per tanggal.
  DoD: test tabel kasus (PPN, non-PPN, diskon, pembulatan).
- [ ] **T2.5 Sales Order** — 4 tipe (`NEW_DELIVERY`, `WITHDRAWAL`, `UNIT_SWAP`, `CONSUMABLE`), confirm/cancel, terbitkan DO (OUTBOUND/INBOUND).
- [ ] **T2.6 DO & reservasi** — reserve (scan SN / pilih rak), release, buat draft LPB dari DO; jalur ApproveLPB dari DO (pakai `qty_reserved`).
  DoD: test 2 reservasi paralel SN sama → satu gagal; cancel DO mengembalikan reserved.
- [ ] **T2.7 POD manual (sementara sebelum app driver)** — Admin/Kepala WH menandai DELIVERED + upload foto surat jalan bertanda tangan → SN RENTED + `rental_placements`. (Endpoint sama dengan driver, permission berbeda.) Penarikan: tandai PICKED_UP manual.

**Sprint 7 — Tagihan**
- [ ] **T2.8 Billing job** — `GenerateMonthlyInvoices` (Bagian 6.5) + `POST /invoices/generate?preview=1`.
  DoD: semua kasus uji tagihan Bagian 11 lulus; jalan 2x → tidak ada invoice ganda.
- [ ] **T2.9 Invoice** — list/detail, issue, void, pembayaran + PPh 23, status otomatis, PDF invoice, export Excel.
- [ ] **T2.10 Penjadwalan** — `/internal/jobs/*` + Vercel Cron (staging) + `cmd/worker` (production), `job_runs`, alert kontrak berakhir.
- [ ] **T2.11 Dashboard komersial.**
  **🎯 Milestone M2:** siklus SO → DO → LPB → terkirim → invoice berjalan di staging.

### Fase 3 — Lapangan (Sprint 8–10)

**Sprint 8 — Driver**
- [ ] **T3.1 Migrasi `000008`** (+ down).
- [ ] **T3.2 Storage** — interface + S3 (Supabase Storage/MinIO) + local; signed URL.
- [ ] **T3.3 API driver** — my-deliveries, checkpoint (idempoten `client_event_id`), deliver (multipart), fail, pickup.
- [ ] **T3.4 UI driver mobile + PWA** — daftar tugas, detail, navigasi, checkpoint GPS, kamera + kompresi, POD.
  DoD: diuji di HP Android & iPhone via URL staging HTTPS.

**Sprint 9 — Retur, swap, peta**
- [ ] **T3.5 Alur penarikan & swap end-to-end** — DO INBOUND, pickup, LPB `RENTAL_RETURN`, QC (MAINTENANCE → AVAILABLE).
- [ ] **T3.6 Gagal kirim** — FAILED → draft `FAILED_DELIVERY_RETURN` → approve.
- [ ] **T3.7 Peta** — `AssetTrackingMap` (pin per site, cluster), rute checkpoint + foto POD.

**Sprint 10 — Tiket maintenance**
- [ ] **T3.8 Tiket** — CRUD, assign, status, aktivitas (komentar, foto, check-in GPS), papan kanban.
- [ ] **T3.9 Sparepart** — permintaan → LPB `SPARE_PART_ISSUE` → ISSUED; flag `is_billable` → invoice SERVICE.
- [ ] **T3.10 Eskalasi swap** — tiket → SO `UNIT_SWAP`.
- [ ] **T3.11 SLA & dashboard lapangan.**
  **🎯 Milestone M3:** semua fitur blueprint selesai di staging.

### Fase 4 — Hardening & Production (Sprint 11–12)

- [ ] **T4.1 Security review** — checklist Bagian 10, uji upload berbahaya, uji IDOR (driver A buka LPB driver B → 403).
- [ ] **T4.2 Rekonsiliasi** — job harian + halaman admin.
- [ ] **T4.3 Audit log viewer & halaman jobs.**
- [ ] **T4.4 Laporan** — utilisasi aset, omzet per pelanggan/bulan, aging piutang, SLA tiket, mutasi stok.
- [ ] **T4.5 E2E Playwright** — 2 alur utama.
- [ ] **T4.6 Infrastruktur production** — `docker-compose.prod.yml`, Caddy, backup + uji restore, monitoring.
- [ ] **T4.7 Skrip migrasi data awal** — stok gudang (`INITIAL_STOCK`) + unit yang sudah di pelanggan (RENTED + placements).
- [ ] **T4.8 UAT & pelatihan per role.**
- [ ] **T4.9 Go-live** — checklist Bagian 12.4.
- [ ] **T4.10 Hypercare 2–4 minggu** — pantau rekonsiliasi & job harian setiap hari.

---

## 14. Backlog / Masukan Lanjutan (setelah go-live)

| Prioritas | Fitur | Catatan desain |
|---|---|---|
| Tinggi | **Tagih di muka (in-advance)** | Invoice periode berjalan untuk placement aktif + baris penyesuaian (kredit/debit) periode lalu dari selisih placement. Kolom `billing_in_advance` sudah ada. |
| Tinggi | **Meter reading / biaya per klik printer** | Umum di sewa printer: tagihan = sewa dasar + kelebihan halaman. Tabel `meter_readings(product_serial_id, reading_date, counter_bw, counter_color, photo_path, recorded_by)` + tarif per klik di `rental_contract_items`. |
| Tinggi | **Notifikasi** | WhatsApp/email: invoice terbit, jatuh tempo, tiket diperbarui, LPB menunggu approval. Tabel `notifications` + worker. |
| Sedang | **Export faktur Coretax** | Generate file impor sesuai format DJP dari invoice ISSUED ber-PPN. |
| Sedang | **Purchase Order supplier** | PO → penerimaan parsial → LPB PURCHASE mereferensikan PO. |
| Sedang | **Portal pelanggan** | Login pelanggan: lihat aset terpasang, invoice, lapor tiket (QR di unit → form tiket). |
| Sedang | **Preventive maintenance** | Jadwal servis berkala per unit → tiket `PREVENTIVE` otomatis. |
| Sedang | **Driver offline-first** | Antrean IndexedDB + Background Sync. |
| Sedang | **Depresiasi & ROI aset** | Dari `purchase_price`, umur, dan pendapatan placement per unit. |
| Rendah | Integrasi akuntansi | Jurnal otomatis (penjualan, PPN keluaran, piutang) ke software akuntansi. |
| Rendah | Denda keterlambatan & deposit | Kolom `deposit_amount` sudah ada di kontrak. |
| Rendah | Cetak label thermal langsung (ZPL) | Alternatif PDF label. |

---

## 15. Glosarium

| Istilah | Arti |
|---|---|
| LPB | Lembar Penerimaan/Pengeluaran Barang — dokumen gudang; satu-satunya pemicu mutasi stok saat APPROVED |
| SN / Asset tag | Serial number pabrikan / label barcode internal perusahaan |
| SO | Sales Order (Kirim, Tarik, Tukar Unit, Consumable) |
| DO | Delivery Order — perintah kerja ke gudang dari SO |
| POD | Proof of Delivery — foto + nama penerima + GPS |
| Placement | Periode sebuah unit terpasang di site pelanggan (dasar tagihan) |
| DPP / DPP Nilai Lain | Dasar Pengenaan Pajak / 11/12 × DPP untuk PPN 12% (efektif 11%) |
| PKP | Pengusaha Kena Pajak |
| PPh 23 | Pajak penghasilan atas sewa yang dipotong pelanggan badan |
| Opname | Penghitungan fisik stok untuk mencocokkan dengan sistem |
| Ledger | Buku besar mutasi stok (append-only) |
