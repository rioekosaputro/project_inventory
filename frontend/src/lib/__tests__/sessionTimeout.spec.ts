import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { ApiError } from '@/lib/apiError'
import { installSessionTimeout } from '@/lib/sessionTimeout'
import { authService } from '@/services/authService'
import { useAuthStore } from '@/stores/auth'
import { makeSession } from '@/test/helpers'

vi.mock('@/services/authService', () => import('@/test/mockAuthService'))

const svc = vi.mocked(authService)
const MINUTE = 60_000
const blank = { render: () => null }

let uninstall: () => void
let router: Router

async function loggedIn() {
  const auth = useAuthStore()
  await auth.login('a@x.id', 'rahasia')
  uninstall = installSessionTimeout(router, auth)
  return auth
}

const type = () => window.dispatchEvent(new Event('keydown'))

beforeEach(async () => {
  vi.useFakeTimers({ now: new Date('2026-11-28T03:00:00Z') })
  vi.clearAllMocks()
  setActivePinia(createPinia())
  localStorage.clear()
  svc.login.mockResolvedValue(makeSession())
  svc.refresh.mockResolvedValue(makeSession({ token: 'access-token-2' }))
  svc.logout.mockResolvedValue(null)
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/dashboard', name: 'dashboard', component: blank },
      { path: '/login', name: 'login', component: blank, meta: { public: true } },
    ],
  })
  await router.push('/dashboard')
})

afterEach(() => {
  uninstall?.()
  vi.useRealTimers()
})

describe('batas tidak aktif', () => {
  it('keluar otomatis setelah 30 menit tanpa aktivitas, dengan alasan di halaman login', async () => {
    const auth = await loggedIn()

    await vi.advanceTimersByTimeAsync(29 * MINUTE)
    expect(auth.isAuthenticated).toBe(true)

    await vi.advanceTimersByTimeAsync(2 * MINUTE)
    expect(auth.isAuthenticated).toBe(false)
    expect(svc.logout).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query).toEqual({ reason: 'idle', redirect: '/dashboard' })
  })

  it('selama ada aktivitas sesi tetap hidup dan refresh token dirotasi berkala', async () => {
    const auth = await loggedIn()

    for (let i = 0; i < 8; i++) {
      await vi.advanceTimersByTimeAsync(10 * MINUTE)
      type()
      await vi.advanceTimersByTimeAsync(0)
    }

    expect(auth.isAuthenticated).toBe(true)
    expect(svc.logout).not.toHaveBeenCalled()
    expect(svc.refresh).toHaveBeenCalledTimes(8)
  })

  it('gerakan pertama setelah perangkat lama tidur tidak menghidupkan sesi lagi', async () => {
    const auth = await loggedIn()

    vi.setSystemTime(Date.now() + 40 * MINUTE) // jam maju tanpa timer berjalan
    type()
    await vi.advanceTimersByTimeAsync(0)

    expect(auth.isAuthenticated).toBe(false)
    expect(svc.refresh).not.toHaveBeenCalled()
  })

  it('aktivitas di tab lain (localStorage) ikut dihitung', async () => {
    const auth = await loggedIn()

    await vi.advanceTimersByTimeAsync(20 * MINUTE)
    localStorage.setItem('erp.last-activity', String(Date.now()))
    await vi.advanceTimersByTimeAsync(20 * MINUTE)

    expect(auth.isAuthenticated).toBe(true)
  })
})

describe('batas maksimum & kegagalan refresh', () => {
  it('keluar saat batas absolut tercapai walau terus aktif', async () => {
    svc.login.mockResolvedValue({ ...makeSession(), session_expires_in: 20 * 60 })
    svc.refresh.mockResolvedValue({ ...makeSession(), session_expires_in: 10 * 60 })
    const auth = await loggedIn()

    await vi.advanceTimersByTimeAsync(10 * MINUTE)
    type()
    await vi.advanceTimersByTimeAsync(11 * MINUTE)

    expect(auth.isAuthenticated).toBe(false)
    expect(router.currentRoute.value.query.reason).toBe('expired')
  })

  it('refresh ditolak server → keluar; gangguan jaringan → tetap masuk', async () => {
    const auth = await loggedIn()

    svc.refresh.mockRejectedValueOnce(new ApiError(0, 'NETWORK_ERROR', 'offline'))
    await vi.advanceTimersByTimeAsync(6 * MINUTE)
    type()
    await vi.advanceTimersByTimeAsync(0)
    expect(auth.isAuthenticated).toBe(true)

    svc.refresh.mockRejectedValueOnce(new ApiError(401, 'INVALID_TOKEN', 'habis'))
    await vi.advanceTimersByTimeAsync(MINUTE)
    type()
    await vi.advanceTimersByTimeAsync(0)
    expect(auth.isAuthenticated).toBe(false)
  })
})

describe('memulihkan sesi saat dibuka lagi', () => {
  it('tidak memulihkan bila aktivitas terakhir sudah melewati batas', async () => {
    localStorage.setItem('erp.last-activity', String(Date.now() - 31 * MINUTE))
    const auth = useAuthStore()

    await auth.bootstrap()

    expect(auth.isAuthenticated).toBe(false)
    expect(svc.logout).toHaveBeenCalledTimes(1)
  })

  it('balasan refresh yang terlambat tidak menghidupkan sesi setelah keluar', async () => {
    const auth = await loggedIn()
    let finish!: () => void
    svc.refresh.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = () => resolve(makeSession())
      }),
    )

    const pending = auth.refreshSession().catch(() => 'ditolak')
    await auth.logout()
    finish()

    expect(await pending).toBe('ditolak')
    expect(auth.isAuthenticated).toBe(false)
  })
})
