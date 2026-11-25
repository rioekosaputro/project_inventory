import {
  createRouter,
  createWebHistory,
  type RouteComponent,
  type RouteRecordRaw,
  type RouterHistory,
} from 'vue-router'
import { installGuards } from '@/router/guards'

declare module 'vue-router' {
  interface RouteMeta {
    /** Layout pembungkus. `auto` = app bila sudah login, selain itu public. */
    layout?: 'app' | 'mobile' | 'public' | 'auto'
    /** Dapat dibuka tanpa login. */
    public?: boolean
    /** Rute publik yang tidak relevan bagi yang sudah login (mis. /login) → dialihkan ke beranda. */
    guestOnly?: boolean
    /** Kunci permission (lihat backend internal/domain/permission.go). Hanya kenyamanan UI. */
    permission?: string
    /** Judul halaman (tab browser). */
    title?: string
  }
}

/**
 * Rute data master (plan Bagian 9.4): daftar + form buat + form ubah. Rute statis `/baru`
 * didaftarkan sebelum `/:id`. Permission baca untuk daftar, permission tulis untuk form.
 */
function masterCrud(
  slug: string,
  title: string,
  read: string,
  write: string,
  views: { list: () => Promise<RouteComponent>; form: () => Promise<RouteComponent> },
): RouteRecordRaw[] {
  const base = `/master/${slug}`
  return [
    {
      path: base,
      name: `master-${slug}`,
      component: views.list,
      meta: { layout: 'app', permission: read, title },
    },
    {
      path: `${base}/baru`,
      name: `master-${slug}-baru`,
      component: views.form,
      meta: { layout: 'app', permission: write, title: `Tambah ${title}` },
    },
    {
      path: `${base}/:id(\\d+)`,
      name: `master-${slug}-ubah`,
      component: views.form,
      meta: { layout: 'app', permission: write, title: `Ubah ${title}` },
    },
  ]
}

export const routes: RouteRecordRaw[] = [
  {
    // Dialihkan oleh guard ke beranda sesuai peran (atau ke /login).
    path: '/',
    name: 'home',
    component: { render: () => null },
  },
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { layout: 'public', public: true, guestOnly: true, title: 'Masuk' },
  },
  {
    path: '/beranda',
    name: 'beranda',
    component: () => import('@/views/HomeView.vue'),
    meta: { layout: 'app', title: 'Beranda' },
  },
  {
    path: '/akun/password',
    name: 'change-password',
    component: () => import('@/views/ChangePasswordView.vue'),
    meta: { layout: 'app', title: 'Ganti Password' },
  },
  ...masterCrud('produk', 'Produk', 'product.read', 'master.write', {
    list: () => import('@/views/master/ProductListView.vue'),
    form: () => import('@/views/master/ProductFormView.vue'),
  }),
  {
    path: '/master/kategori',
    name: 'master-kategori',
    component: () => import('@/views/master/CategoryListView.vue'),
    meta: { layout: 'app', permission: 'master.read', title: 'Kategori' },
  },
  ...masterCrud('gudang', 'Gudang', 'master.read', 'master.write', {
    list: () => import('@/views/master/WarehouseListView.vue'),
    form: () => import('@/views/master/WarehouseFormView.vue'),
  }),
  ...masterCrud('rak', 'Rak', 'master.read', 'rack.write', {
    list: () => import('@/views/master/RackListView.vue'),
    form: () => import('@/views/master/RackFormView.vue'),
  }),
  ...masterCrud('supplier', 'Supplier', 'master.read', 'master.write', {
    list: () => import('@/views/master/SupplierListView.vue'),
    form: () => import('@/views/master/SupplierFormView.vue'),
  }),
  // LPB (plan Bagian 9.4). Rute statis (/baru, /approvals) lebih dulu dari /:id.
  {
    path: '/lpb',
    name: 'lpb',
    component: () => import('@/views/lpb/LpbListView.vue'),
    meta: { layout: 'app', permission: 'lpb.read', title: 'LPB' },
  },
  {
    path: '/lpb/baru',
    name: 'lpb-baru',
    component: () => import('@/views/lpb/LpbFormView.vue'),
    meta: { layout: 'app', permission: 'lpb.create', title: 'Buat LPB' },
  },
  {
    path: '/lpb/approvals',
    name: 'lpb-approvals',
    component: () => import('@/views/lpb/LpbApprovalsView.vue'),
    meta: { layout: 'app', permission: 'lpb.approve', title: 'Persetujuan LPB' },
  },
  {
    path: '/lpb/:id(\\d+)',
    name: 'lpb-detail',
    component: () => import('@/views/lpb/LpbDetailView.vue'),
    meta: { layout: 'app', permission: 'lpb.read', title: 'Rincian LPB' },
  },
  {
    path: '/lpb/:id(\\d+)/ubah',
    name: 'lpb-ubah',
    component: () => import('@/views/lpb/LpbFormView.vue'),
    meta: { layout: 'app', permission: 'lpb.create', title: 'Ubah Draft LPB' },
  },
  // Verifikasi QR publik Surat Jalan / LPB (plan Bagian 7.6 & 9.4).
  {
    path: '/verify/lpb/:token',
    name: 'lpb-verify-public',
    component: () => import('@/views/public/LPBVerifyView.vue'),
    meta: { layout: 'public', public: true, title: 'Verifikasi Surat Jalan' },
  },
  // Stock Opname (plan Bagian 9.4, T1.12).
  {
    path: '/opname',
    name: 'opname',
    component: () => import('@/views/opname/OpnameListView.vue'),
    meta: { layout: 'app', permission: 'opname.read', title: 'Stock Opname' },
  },
  {
    path: '/opname/:id(\\d+)',
    name: 'opname-detail',
    component: () => import('@/views/opname/OpnameDetailView.vue'),
    meta: { layout: 'app', permission: 'opname.read', title: 'Rincian Stock Opname' },
  },
  {
    path: '/403',
    name: 'forbidden',
    component: () => import('@/views/ForbiddenView.vue'),
    meta: { layout: 'app', title: 'Akses ditolak' },
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/NotFoundView.vue'),
    meta: { layout: 'auto', title: 'Halaman tidak ditemukan' },
  },
]

/** Pabrik router (dipakai main.ts dan test dengan history memori). */
export function createAppRouter(history: RouterHistory = createWebHistory()) {
  const router = createRouter({ history, routes })
  installGuards(router)
  return router
}
