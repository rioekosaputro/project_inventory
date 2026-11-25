# Modul Panduan Penggunaan — Rental ERP

Panduan praktis penggunaan setiap menu pada aplikasi **Rental ERP** (Manajemen Gudang, Sewa Aset, Pengiriman, dan Tagihan).

- **URL Aplikasi**: https://rental-erp-web.vercel.app
- **Akun Demo (ADMIN)**: `admin@contoh.id`
- **Tips Tampilan**:
  - Di layar desktop, sidebar kiri bisa **diciutkan/dibuka** lewat tombol panah `«` / `»` di pojok kanan atas sidebar.
  - Di layar HP, menu dibuka lewat tombol garis tiga (`☰`) di pojok kiri atas.

---

## 1. Gambaran Alur Besar Aplikasi

```text
[1. Data Master]       → Gudang, Rak, Kategori, Produk (SN / Non-SN), Supplier
        ↓
[2. Stok Masuk (LPB)]  → Buat LPB Pembelian / Stok Awal → Scan SN → Ajukan → Setujui (Stok bertambah)
        ↓
[3. Pelanggan & Kontrak] → Tambah Pelanggan + Cabang/Site (Peta) → Buat Kontrak Sewa → Aktifkan
        ↓
[4. Sales Order (SO)]  → Buat SO (Pengiriman Baru / Penarikan / Tukar Unit / Consumable) → Konfirmasi
        ↓
[5. Delivery Order]    → Terbitkan DO ke Gudang → Reservasi Rak/SN → Buat Draft LPB Keluar
        ↓
[6. Pengiriman (POD)]  → Setujui LPB Keluar → Tandai Terkirim (POD) → Unit jadi RENTED (mulai ditagih)
        ↓
[7. Tagihan (Invoice)] → Generate Tagihan Bulanan (Pratinjau / Eksekusi) → Terbitkan → Catat Bayar + PPh 23
```

---

## 2. Penjelasan Setiap Menu

### A. Bagian **UTAMA**

#### 1. **Beranda** (`/beranda`)
Halaman sambutan singkat dan pintasan awal setelah login.

#### 2. **Dashboard** (`/dashboard`)
Ringkasan kondisi bisnis secara real-time dalam satu layar:
- **Komersial** *(khusus Manajemen, Admin Bisnis, & Admin)*:
  - **Utilisasi Aset Sewa**: persentase unit berstatus `RENTED` dibanding total unit aktif.
  - **Omzet Bulan Berjalan**: total tagihan terbit bulan ini & total pembayaran yang sudah tertagih.
  - **Piutang & Overdue**: sisa tagihan belum lunas, jumlah invoice lewat jatuh tempo, dan yang jatuh tempo ≤ 7 hari.
  - **Kontrak Berakhir ≤ 30 Hari**: peringatan kontrak yang mendekati masa habis agar bisa segera diperpanjang.
  - Daftar **Invoice Melewati Jatuh Tempo** & **Kontrak Akan Berakhir**.
- **Gudang**:
  - **Produk Stok Menipis**: produk dengan stok tersedia di bawah `min_stock_alert`.
  - **LPB Menunggu Persetujuan**, **Opname & Antrean DO**, **Unit SN Tersedia**, dan **Mutasi Stok Terbaru**.
  - Mendukung filter per gudang di pojok kanan atas.

---

### B. Bagian **GUDANG**

#### 3. **Data Master** (`/master/*`)
Fondasi semua data transaksi. Terdiri dari 5 tab:
- **Produk**: kelola SKU, kategori (`ASSET` atau `CONSUMABLE`), centang **Memiliki Serial Number** (untuk unit sewa seperti printer/PC), batas stok minimum, dan tarif sewa/harga jual bawaan.
- **Kategori**: kelompokkan produk ke tipe `ASSET` (unit berserial) atau `CONSUMABLE` (toner/kertas per qty).
- **Gudang**: daftar lokasi gudang (kode gudang unik dan dipakai pada nomor dokumen LPB/DO).
- **Rak**: lokasi penyimpanan di dalam gudang. Rak zona `KARANTINA`/`WORKSHOP` otomatis **tidak dapat dijual** (`is_sellable = false`), sehingga stoknya tidak dihitung sebagai stok tersedia.
- **Supplier**: daftar pemasok untuk pembelian barang.

#### 4. **LPB (Laporan Penerimaan & Pengeluaran Barang)** (`/lpb`)
Satu-satunya pintu yang boleh mengubah stok gudang (setelah disetujui):
- **Buat LPB** (`/lpb/baru`): pilih jenis transaksi:
  - `Pembelian (PURCHASE)` & `Stok Awal (INITIAL_STOCK)` → menambah stok & otomatis membuat **Asset Tag** untuk unit SN.
  - `Transfer Keluar (TRANSFER_OUT)` → memindahkan barang antar rak (otomatis selesai jika satu gudang) atau antar gudang (menunggu persetujuan gudang tujuan).
  - `Penghapusan (DISPOSAL)` → menghapus barang/unit berstatus `BROKEN`.
- **Import Stok Awal (Excel/CSV)**: tombol di kanan atas daftar LPB untuk mengunduh template `.xlsx` dan mengunggah stok awal sekaligus ke dalam draft LPB.
- **Scan SN (Scanner POS)**: di halaman Detail LPB draft, masukkan/scan nomor seri unit sebelum klik **Ajukan untuk Persetujuan**.
- **POD / Pickup**: untuk LPB pengiriman/penarikan sewa yang sudah disetujui, tersedia tombol **Tandai Terkirim (POD)** (mengubah unit menjadi `RENTED` dan memulai penagihan) dan **Tandai Dijemput (Pickup)** (menghentikan penagihan saat penarikan).

#### 5. **Persetujuan LPB** (`/lpb/approvals`)
Antrean LPB berstatus `Menunggu Persetujuan` (diurutkan dari yang paling lama).
- Klik nomor LPB untuk melihat **dampak saldo sebelum → sesudah** setiap rak sebelum menekan **Setujui** atau **Tolak** (dengan alasan wajib).
- *Catatan aturan kontrol*: secara bawaan, pembuat LPB tidak bisa menyetujui LPB buatannya sendiri kecuali akun lain / pengaturan self-approve diaktifkan.

#### 6. **Stok** (`/stok`)
Menampilkan saldo stok per produk & rak (`On Hand`, `Reserved`, dan `Tersedia`), lengkap dengan filter **Stok Menipis** dan pencarian cepat (`/`).

#### 7. **Serial Number** (`/stok/serial`)
Melacak setiap unit aset individu berdasarkan **Serial Number** dan **Asset Tag** (`AST-YYMM-NNNNN`), beserta statusnya (`AVAILABLE`, `RESERVED`, `IN_TRANSIT`, `RENTED`, `BROKEN`, `RETIRED`, `LOST`).
- Klik salah satu unit untuk melihat **Timeline Riwayat Lengkap** pergerakan unit sejak pertama diterima hingga terpasang di pelanggan.

#### 8. **Buku Besar** (`/stok/buku-besar`) & **Kartu Stok** (`/stok/kartu-stok`)
- **Buku Besar**: catatan mutasi stok keluar-masuk (append-only) lengkap dengan saldo sebelum/sesudah dan tombol **Export Excel (`.xlsx`)**.
- **Kartu Stok**: rincian saldo awal, seluruh mutasi dalam rentang tanggal, dan saldo akhir per produk & rak, juga mendukung **Export Excel**.

#### 9. **Stock Opname** (`/opname`)
Proses hitung fisik stok berkala:
- Klik **Mulai Opname**, pilih gudang dan rak yang akan dihitung (rak otomatis **dikunci** agar tidak ada mutasi LPB selama opname berlangsung).
- Tersedia opsi **Blind Count** (menyembunyikan qty sistem dari staf saat menghitung).
- Di halaman Detail Opname, isi hitungan **Non-SN** dan lakukan **Scan SN**, lalu **Ajukan** → **Finalisasi** (sistem otomatis membuat LPB penyesuaian jika ada selisih dan membuka kunci rak).

---

### C. Bagian **KOMERSIAL**

#### 10. **Pelanggan** (`/pelanggan`)
Mengelola data perusahaan pelanggan, NPWP & status PKP, termin pembayaran, serta **Cabang/Site**:
- Klik nama pelanggan untuk membuka **Detail Pelanggan** yang memiliki 3 tab:
  1. **Cabang / Site**: tambah/ubah lokasi pemasangan unit. Dilengkapi **Peta Interaktif (Leaflet)** — cukup klik titik di peta atau isi koordinat Latitude/Longitude manual. Hanya ada 1 site utama per pelanggan.
  2. **Aset Terpasang**: daftar seluruh unit berstatus `RENTED` yang sedang aktif di pelanggan tersebut.
  3. **Profil & Perpajakan**: data NPWP, alamat faktur, dan alamat penagihan.

#### 11. **Kontrak** (`/kontrak`)
Mengelola perjanjian sewa bulanan per pelanggan:
- Isi tanggal mulai, durasi bulan (tanggal berakhir terhitung otomatis), tanggal tagih bulanan (1–28), mode tagih (`PRORATA` atau `FULL_MONTH`), status PPN, serta daftar produk & tarif sewa bulanan per site.
- Siklus status kontrak: `DRAFT` → **Aktifkan** (`ACTIVE`) → bisa **Perpanjang** (membuat draft kontrak baru menyalin seluruh tarif) atau **Terminasi** (dengan alasan wajib).

#### 12. **Sales Order (SO)** (`/sales-orders`)
Perintah kerja komersial ke gudang. Mendukung **4 tipe**:
1. **Pengiriman Baru (`NEW_DELIVERY`)**: mengirim unit sewa baru berdasarkan kontrak aktif.
2. **Penarikan (`WITHDRAWAL`)**: menarik kembali unit yang sedang `RENTED` di pelanggan.
3. **Tukar Unit (`UNIT_SWAP`)**: mengirim unit pengganti (`OUT`) sekaligus menarik unit lama (`IN`).
4. **Penjualan Consumable (`CONSUMABLE`)**: penjualan toner/kertas/sparepart tanpa kontrak sewa.
- Pajak **PMK 131/2024** (DPP, DPP Nilai Lain 11/12, PPN 12%) dihitung otomatis oleh server.
- Alur: Buat Draft → **Konfirmasi** → **Terbitkan Delivery Order (DO)** ke gudang tujuan.

#### 13. **Delivery Order (DO)** (`/delivery-orders`)
Antrean perintah pengiriman/penarikan di gudang:
1. Buka DO berstatus `OPEN` → klik **Reservasi Stok**: pilih rak sellable dan unit SN `AVAILABLE` (stok otomatis ditahan di `qty_reserved` dan unit menjadi `RESERVED` agar tidak diambil DO lain).
2. Bila perlu batal, klik **Lepas Reservasi** atau **Batalkan DO** (stok otomatis kembali tersedia).
3. Setelah semua item terreservasi (`RESERVED`), klik **Buat Draft LPB** → sistem langsung membuka halaman LPB keluar yang sudah terisi otomatis, siap diajukan & disetujui!

#### 14. **Tagihan / Invoice** (`/invoices`)
Pusat penagihan sewa bulanan dan pencatatan pembayaran:
- **Generate Tagihan Bulanan**: klik tombol di kanan atas daftar Tagihan → pilih tanggal tagih → klik **Pratinjau** (mode simulasi aman, belum menulis ke database) untuk melihat rincian prorata/full-month dan PPN PMK 131/2024, lalu klik **Generate Sekarang** untuk membuat draft invoice (aman dijalankan berulang, tidak membuat tagihan ganda).
- **Export Excel**: mengunduh file `.xlsx` berisi 2 sheet (**Daftar Tagihan** & **Rekap PPN**).
- Di **Detail Invoice** (`/invoices/:id`):
  - **Terbitkan Invoice**: ubah status `DRAFT` → `ISSUED` (bisa isi nomor Faktur Pajak kode `04`).
  - **Catat Pembayaran (+ PPh 23)**: masukkan nominal transfer diterima + potongan PPh 23 + nomor bukti potong. Status invoice otomatis berubah ke `PARTIALLY_PAID` atau `PAID` saat lunas, dan dilindungi `Idempotency-Key` agar klik ganda tidak mencatat pembayaran dua kali.
  - **Void (Batalkan)**: membatalkan invoice yang belum dibayar (wajib isi alasan).

---

## 3. Panduan Uji Coba Cepat (5 Menit End-to-End)

Jika ingin langsung mencoba seluruh alur dari nol:
1. **Login** → ganti password saat login pertama → cek **Dashboard**.
2. **Data Master**: buat 1 Gudang, 1 Rak (zona `PRINTER`), 1 Produk berserial (`ASSET`), dan 1 Supplier.
3. **LPB Masuk**: buka **LPB → Buat LPB** (jenis `Stok Awal`, qty 2) → di Detail LPB scan 2 SN (mis. `SN-001`, `SN-002`) → **Ajukan** → buka **Persetujuan LPB** *(jika memakai akun lain / approver)* → **Setujui**. Cek menu **Stok** & **Serial Number**.
4. **Pelanggan & Kontrak**: buat 1 Pelanggan + 1 Site (klik titik di peta) → buat 1 Kontrak Sewa untuk produk tersebut → klik **Aktifkan**.
5. **SO → DO → LPB → POD**: buat **Sales Order** (`NEW_DELIVERY`) → **Konfirmasi** → **Terbitkan DO** → buka **Delivery Order** → **Reservasi** `SN-001` → **Buat Draft LPB** → **Ajukan & Setujui** → klik **Tandai Terkirim (POD)**. Cek di **Detail Pelanggan → Tab Aset Terpasang**, `SN-001` kini tampil sebagai `RENTED`.
6. **Tagihan**: buka **Tagihan → Generate Tagihan Bulanan** → pilih tanggal 1 bulan berikutnya → klik **Pratinjau** lalu **Generate** → buka Detail Invoice → **Terbitkan** → **Catat Pembayaran (+ PPh 23)** → cek **Dashboard** (utilisasi aset, omzet, & tertagih otomatis terupdate!).

---

## 4. Catatan Lingkungan Staging
- **Cetak PDF** (Surat Jalan, Label, Invoice PDF) membutuhkan layanan **Gotenberg** (`GOTENBERG_URL`) yang terhubung di server staging. Bila Gotenberg belum dipasang di staging, seluruh fitur lain (CRUD, Stok, LPB, Opname, SO/DO, Billing, Pembayaran, Export Excel `.xlsx`) tetap berfungsi 100% normal.
- **Database Supabase Free** dijaga tetap aktif oleh jadwal ping otomatis (`Keep Supabase Alive` setiap 3 hari + cron harian).
