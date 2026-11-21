import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, type Router } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import { createAppRouter } from '@/router'
import { homeFor, safeRedirectTarget } from '@/router/guards'
import { authService } from '@/services/authService'
import { ApiError } from '@/lib/apiError'
import { makeSession } from '@/test/helpers'

vi.mock('@/services/authService', () => import('@/test/mockAuthService'))

const svc = vi.mocked(authService)

function setup(session: ReturnType<typeof makeSession> | null): Router {
  setActivePinia(createPinia())
  if (session) svc.refresh.mockResolvedValue(session)
  else svc.refresh.mockRejectedValue(new ApiError(401, 'INVALID_TOKEN', 'Tidak ada sesi'))
  return createAppRouter(createMemoryHistory())
}

async function go(router: Router, to: string): Promise<string> {
  await router.push(to)
  await router.isReady()
  return router.currentRoute.value.fullPath
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('pengguna anonim', () => {
  it('halaman terproteksi → /login dengan ?redirect= ke halaman tujuan', async () => {
    const router = setup(null)
    expect(await go(router, '/beranda')).toBe('/login?redirect=/beranda')
  })

  it('/ → /login tanpa redirect', async () => {
    const router = setup(null)
    expect(await go(router, '/')).toBe('/login')
  })

  it('/login terbuka', async () => {
    const router = setup(null)
    expect(await go(router, '/login')).toBe('/login')
  })

  it('URL acak (404) juga mengarah ke login lebih dulu, tanpa membocorkan isi aplikasi', async () => {
    const router = setup(null)
    expect(await go(router, '/halaman-ngawur')).toBe('/login?redirect=/halaman-ngawur')
  })
})

describe('pengguna yang sudah login', () => {
  it('/login dialihkan ke beranda (fallback /beranda bila default_home belum ada halamannya)', async () => {
    const router = setup(makeSession({ default_home: '/dashboard' }))
    expect(await go(router, '/login')).toBe('/beranda')
  })

  it('/ menuju default_home bila halamannya ada', async () => {
    const router = setup(makeSession({ default_home: '/akun/password' }))
    expect(await go(router, '/')).toBe('/akun/password')
  })

  it('/login?redirect= menuju halaman aman yang diminta', async () => {
    const router = setup(makeSession())
    expect(await go(router, '/login?redirect=/akun/password')).toBe('/akun/password')
  })

  it('halaman 404 tetap tampil untuk user login (bukan dialihkan)', async () => {
    const router = setup(makeSession())
    expect(await go(router, '/tidak-ada')).toBe('/tidak-ada')
    expect(router.currentRoute.value.name).toBe('not-found')
  })

  it('sesi dipulihkan SEKALI untuk seluruh navigasi', async () => {
    const router = setup(makeSession())
    await go(router, '/beranda')
    await go(router, '/akun/password')
    await go(router, '/beranda')
    expect(svc.refresh).toHaveBeenCalledTimes(1)
  })
})

describe('wajib ganti password', () => {
  it('semua halaman dialihkan ke /akun/password', async () => {
    const router = setup(makeSession({ user: { must_change_password: true } }))

    expect(await go(router, '/beranda')).toBe('/akun/password')
    expect(await go(router, '/')).toBe('/akun/password')
    expect(await go(router, '/403')).toBe('/akun/password')
  })

  it('halaman ganti password sendiri tetap bisa dibuka', async () => {
    const router = setup(makeSession({ user: { must_change_password: true } }))
    expect(await go(router, '/akun/password')).toBe('/akun/password')
  })
})

describe('permission pada rute', () => {
  function withProtectedRoute(router: Router): void {
    router.addRoute({
      path: '/lpb/approvals',
      name: 'lpb-approvals',
      component: { render: () => null },
      meta: { layout: 'app', permission: 'lpb.approve' },
    })
  }

  it('tanpa permission → /403', async () => {
    const router = setup(makeSession({ permissions: ['lpb.read'] }))
    withProtectedRoute(router)
    expect(await go(router, '/lpb/approvals')).toBe('/403')
  })

  it('dengan permission → boleh', async () => {
    const router = setup(makeSession({ permissions: ['lpb.read', 'lpb.approve'] }))
    withProtectedRoute(router)
    expect(await go(router, '/lpb/approvals')).toBe('/lpb/approvals')
  })

  it('halaman 403 sendiri tidak membutuhkan permission', async () => {
    const router = setup(makeSession({ permissions: [] }))
    expect(await go(router, '/403')).toBe('/403')
  })
})

describe('safeRedirectTarget (cegah open redirect)', () => {
  const router = setup(null)

  it.each([
    ['//evil.example', 'protocol-relative'],
    ['/\\evil.example', 'backslash'],
    ['https://evil.example/x', 'URL absolut'],
    ['javascript:alert(1)', 'skema javascript'],
    ['evil', 'tanpa slash awal'],
    ['/login', 'loop ke login'],
    ['/halaman-tidak-ada', 'route tak ada'],
    ['', 'kosong'],
  ])('menolak %s (%s)', (raw) => {
    expect(safeRedirectTarget(router, raw)).toBeNull()
  })

  it('menolak non-string', () => {
    expect(safeRedirectTarget(router, ['/beranda'])).toBeNull()
    expect(safeRedirectTarget(router, undefined)).toBeNull()
    expect(safeRedirectTarget(router, 42)).toBeNull()
  })

  it('menerima path internal yang valid (termasuk query)', () => {
    expect(safeRedirectTarget(router, '/beranda')).toBe('/beranda')
    expect(safeRedirectTarget(router, '/akun/password?x=1')).toBe('/akun/password?x=1')
  })
})

describe('homeFor', () => {
  it('memakai default_home bila ada, selain itu /beranda', () => {
    const router = setup(null)
    expect(homeFor(router, '/akun/password')).toBe('/akun/password')
    expect(homeFor(router, '/driver/deliveries')).toBe('/beranda')
  })
})
