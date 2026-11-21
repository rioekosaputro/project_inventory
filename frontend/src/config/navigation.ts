import type { Permission } from '@/types/auth'

export interface NavItem {
  label: string
  to: string
  /** Kelas ikon PrimeIcons. */
  icon: string
  /** Awalan path yang dianggap "sedang di menu ini" (default: `to`). */
  match?: string
  /** Tanpa permission = semua pengguna yang login. */
  permission?: Permission
}

export interface NavGroup {
  label: string
  items: NavItem[]
}

/**
 * Seluruh menu yang direncanakan (plan Bagian 9.4). Item tampil hanya bila pengguna punya
 * permission-nya DAN rutenya sudah terdaftar — jadi halaman fase berikutnya muncul otomatis
 * saat rutenya ditambahkan, tanpa mengubah file ini.
 */
export const navGroups: NavGroup[] = [
  {
    label: 'Utama',
    items: [
      { label: 'Beranda', to: '/beranda', icon: 'pi pi-home' },
      {
        label: 'Dashboard',
        to: '/dashboard',
        icon: 'pi pi-chart-bar',
        permission: 'dashboard.read',
      },
    ],
  },
  {
    label: 'Gudang',
    items: [
      { label: 'Stok', to: '/stok', icon: 'pi pi-box', permission: 'inventory.read' },
      {
        label: 'Serial Number',
        to: '/stok/serial',
        icon: 'pi pi-barcode',
        permission: 'inventory.read',
      },
      {
        label: 'Buku Besar',
        to: '/stok/buku-besar',
        icon: 'pi pi-book',
        permission: 'ledger.read',
      },
      { label: 'LPB', to: '/lpb', icon: 'pi pi-file-edit', permission: 'lpb.read' },
      {
        label: 'Persetujuan LPB',
        to: '/lpb/approvals',
        icon: 'pi pi-check-square',
        permission: 'lpb.approve',
      },
      { label: 'Stock Opname', to: '/opname', icon: 'pi pi-list-check', permission: 'opname.read' },
      {
        label: 'Data Master',
        to: '/master/produk',
        match: '/master',
        icon: 'pi pi-database',
        permission: 'product.read',
      },
    ],
  },
  {
    label: 'Komersial',
    items: [
      { label: 'Pelanggan', to: '/pelanggan', icon: 'pi pi-users', permission: 'customer.read' },
      { label: 'Kontrak', to: '/kontrak', icon: 'pi pi-file', permission: 'contract.read' },
      {
        label: 'Sales Order',
        to: '/sales-orders',
        icon: 'pi pi-shopping-cart',
        permission: 'so.read',
      },
      {
        label: 'Delivery Order',
        to: '/delivery-orders',
        icon: 'pi pi-truck',
        permission: 'do.read',
      },
      { label: 'Tagihan', to: '/invoices', icon: 'pi pi-wallet', permission: 'invoice.read' },
    ],
  },
  {
    label: 'Lapangan',
    items: [
      {
        label: 'Tugas Pengiriman',
        to: '/driver/deliveries',
        icon: 'pi pi-truck',
        permission: 'driver',
      },
      { label: 'Peta Aset', to: '/tracking/map', icon: 'pi pi-map', permission: 'map.read' },
      {
        label: 'Tiket Maintenance',
        to: '/tickets',
        icon: 'pi pi-wrench',
        permission: 'ticket.read',
      },
    ],
  },
  {
    label: 'Laporan',
    items: [
      { label: 'Laporan', to: '/laporan', icon: 'pi pi-chart-line', permission: 'report.read' },
    ],
  },
  {
    label: 'Administrasi',
    items: [
      { label: 'Pengguna', to: '/admin/users', icon: 'pi pi-user-edit', permission: 'user.manage' },
      {
        label: 'Pengaturan',
        to: '/admin/settings',
        icon: 'pi pi-cog',
        permission: 'settings.manage',
      },
      { label: 'Log Audit', to: '/admin/audit', icon: 'pi pi-history', permission: 'audit.read' },
      {
        label: 'Rekonsiliasi',
        to: '/admin/rekonsiliasi',
        icon: 'pi pi-sync',
        permission: 'reconcile.read',
      },
      { label: 'Jobs', to: '/admin/jobs', icon: 'pi pi-clock', permission: 'jobs.read' },
    ],
  },
]

/**
 * Menyaring menu: buang item tanpa permission / tanpa rute, lalu buang grup yang kosong.
 * Fungsi murni agar mudah diuji.
 */
export function visibleNav(
  groups: NavGroup[],
  opts: { can: (p: Permission) => boolean; routeExists: (path: string) => boolean },
): NavGroup[] {
  return groups
    .map((g) => ({
      ...g,
      items: g.items.filter(
        (i) => (!i.permission || opts.can(i.permission)) && opts.routeExists(i.to),
      ),
    }))
    .filter((g) => g.items.length > 0)
}
