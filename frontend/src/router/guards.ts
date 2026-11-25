import type { NavigationGuardReturn, RouteLocationNormalized, Router } from 'vue-router'
import { FALLBACK_HOME, useAuthStore } from '@/stores/auth'

const APP_NAME = 'Rental ERP'

/** true bila path cocok dengan route yang terdaftar (bukan catch-all 404). */
export function routeExists(router: Router, path: string): boolean {
  const resolved = router.resolve(path)
  return resolved.matched.length > 0 && resolved.name !== 'not-found'
}

/** Beranda pengguna: default_home dari backend bila halamannya sudah ada dan boleh diakses, selain itu /beranda. */
export function homeFor(
  router: Router,
  defaultHome: string,
  can?: (permission: string) => boolean,
): string {
  if (!routeExists(router, defaultHome)) return FALLBACK_HOME
  const perm = router.resolve(defaultHome).meta.permission
  if (perm && can && !can(perm)) return FALLBACK_HOME
  return defaultHome
}

/**
 * Validasi parameter `?redirect=` (cegah open redirect): hanya path internal yang diawali
 * satu "/" (bukan "//" atau "/\"), menuju route yang ada, dan bukan /login. Selain itu → null.
 */
export function safeRedirectTarget(router: Router, raw: unknown): string | null {
  if (typeof raw !== 'string' || !/^\/(?![/\\])/.test(raw)) return null
  if (!routeExists(router, raw)) return null
  return router.resolve(raw).name === 'login' ? null : raw
}

/**
 * Guard global. Urutan: pulihkan sesi (sekali) → rute publik → wajib login →
 * wajib ganti password → permission. Hanya kenyamanan UI: otorisasi sebenarnya di backend.
 */
export function installGuards(router: Router): void {
  router.beforeEach(async (to: RouteLocationNormalized): Promise<NavigationGuardReturn> => {
    const auth = useAuthStore()
    await auth.bootstrap()

    if (to.name === 'home') {
      return auth.isAuthenticated
        ? homeFor(router, auth.defaultHome, (p) => auth.can(p))
        : { name: 'login' }
    }

    if (to.meta.public) {
      if (to.meta.guestOnly && auth.isAuthenticated) {
        return (
          safeRedirectTarget(router, to.query.redirect) ??
          homeFor(router, auth.defaultHome, (p) => auth.can(p))
        )
      }
      return true
    }

    if (!auth.isAuthenticated) {
      return {
        name: 'login',
        query: to.fullPath === '/' ? undefined : { redirect: to.fullPath },
      }
    }

    if (auth.mustChangePassword && to.name !== 'change-password') {
      return { name: 'change-password' }
    }

    if (to.meta.permission && !auth.can(to.meta.permission)) {
      return { name: 'forbidden' }
    }

    return true
  })

  router.afterEach((to) => {
    document.title = to.meta.title ? `${to.meta.title} · ${APP_NAME}` : APP_NAME
  })
}
