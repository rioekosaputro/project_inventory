import { createRouter, createWebHistory, type RouteRecordRaw, type RouterHistory } from 'vue-router'
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
