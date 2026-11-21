import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { authService } from '@/services/authService'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/lib/apiError'
import { makeSession } from '@/test/helpers'

vi.mock('@/services/authService', () => import('@/test/mockAuthService'))

const svc = vi.mocked(authService)

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
  localStorage.clear()
  sessionStorage.clear()
})

describe('login', () => {
  it('menyimpan token, user, permission, dan home di memori', async () => {
    svc.login.mockResolvedValue(
      makeSession({
        role: 'HEAD_WAREHOUSE',
        permissions: ['lpb.approve', 'lpb.read'],
        default_home: '/lpb/approvals',
      }),
    )
    const auth = useAuthStore()

    await auth.login('a@x.id', 'rahasia')

    expect(svc.login).toHaveBeenCalledWith('a@x.id', 'rahasia')
    expect(auth.isAuthenticated).toBe(true)
    expect(auth.accessToken).toBe('access-token-1')
    expect(auth.user?.role).toBe('HEAD_WAREHOUSE')
    expect(auth.defaultHome).toBe('/lpb/approvals')
  })

  it('TIDAK menyimpan token di localStorage/sessionStorage', async () => {
    svc.login.mockResolvedValue(makeSession())
    await useAuthStore().login('a@x.id', 'rahasia')

    expect(localStorage.length).toBe(0)
    expect(sessionStorage.length).toBe(0)
  })

  it('gagal login tidak mengubah state dan meneruskan ApiError', async () => {
    svc.login.mockRejectedValue(
      new ApiError(401, 'INVALID_CREDENTIALS', 'Email atau password salah'),
    )
    const auth = useAuthStore()

    await expect(auth.login('a@x.id', 'salah')).rejects.toMatchObject({
      code: 'INVALID_CREDENTIALS',
    })
    expect(auth.isAuthenticated).toBe(false)
  })
})

describe('can / canAny', () => {
  it('memeriksa permission dari sesi', async () => {
    svc.login.mockResolvedValue(makeSession({ permissions: ['lpb.read', 'lpb.create'] }))
    const auth = useAuthStore()
    await auth.login('a@x.id', 'x')

    expect(auth.can('lpb.read')).toBe(true)
    expect(auth.can('lpb.approve')).toBe(false)
    expect(auth.canAny('lpb.approve', 'lpb.create')).toBe(true)
    expect(auth.canAny('lpb.approve', 'user.manage')).toBe(false)
  })

  it('tanpa sesi tidak punya permission apa pun', () => {
    expect(useAuthStore().can('lpb.read')).toBe(false)
  })
})

describe('bootstrap', () => {
  it('memulihkan sesi dari cookie refresh', async () => {
    svc.refresh.mockResolvedValue(makeSession({ token: 'dari-cookie' }))
    const auth = useAuthStore()

    await auth.bootstrap()

    expect(auth.ready).toBe(true)
    expect(auth.isAuthenticated).toBe(true)
    expect(auth.accessToken).toBe('dari-cookie')
  })

  it('tanpa cookie/kedaluwarsa → anonim (bukan error) dan tetap ready', async () => {
    svc.refresh.mockRejectedValue(new ApiError(401, 'INVALID_TOKEN', 'Sesi berakhir'))
    const auth = useAuthStore()

    await expect(auth.bootstrap()).resolves.toBeUndefined()

    expect(auth.ready).toBe(true)
    expect(auth.isAuthenticated).toBe(false)
  })

  it('hanya berjalan sekali walau dipanggil berkali-kali (satu refresh, bukan banyak)', async () => {
    svc.refresh.mockResolvedValue(makeSession())
    const auth = useAuthStore()

    await Promise.all([auth.bootstrap(), auth.bootstrap(), auth.bootstrap()])
    await auth.bootstrap()

    expect(svc.refresh).toHaveBeenCalledTimes(1)
  })
})

describe('refreshSession', () => {
  it('memperbarui token & user dan mengembalikan access token baru', async () => {
    svc.refresh.mockResolvedValue(makeSession({ token: 'token-baru', role: 'HEAD_WAREHOUSE' }))
    const auth = useAuthStore()

    const token = await auth.refreshSession()

    expect(token).toBe('token-baru')
    expect(auth.user?.role).toBe('HEAD_WAREHOUSE')
  })

  it('menolak (reject) saat gagal agar antrean axios tahu sesi tidak pulih', async () => {
    svc.refresh.mockRejectedValue(new ApiError(401, 'INVALID_TOKEN', 'x'))
    await expect(useAuthStore().refreshSession()).rejects.toBeInstanceOf(ApiError)
  })
})

describe('logout', () => {
  it('membersihkan state setelah memanggil API', async () => {
    svc.login.mockResolvedValue(makeSession())
    svc.logout.mockResolvedValue(null)
    const auth = useAuthStore()
    await auth.login('a@x.id', 'x')

    await auth.logout()

    expect(svc.logout).toHaveBeenCalled()
    expect(auth.isAuthenticated).toBe(false)
    expect(auth.accessToken).toBeNull()
    expect(auth.permissions).toEqual([])
  })

  it('tetap keluar secara lokal walau server tidak terjangkau', async () => {
    svc.login.mockResolvedValue(makeSession())
    svc.logout.mockRejectedValue(new ApiError(0, 'NETWORK_ERROR', 'offline'))
    const auth = useAuthStore()
    await auth.login('a@x.id', 'x')

    await expect(auth.logout()).resolves.toBeUndefined()
    expect(auth.isAuthenticated).toBe(false)
  })
})

describe('changePassword', () => {
  it('mengganti sesi dengan yang baru dan menurunkan flag must_change_password', async () => {
    svc.login.mockResolvedValue(
      makeSession({ user: { must_change_password: true }, token: 'lama' }),
    )
    svc.changePassword.mockResolvedValue(
      makeSession({ user: { must_change_password: false }, token: 'baru' }),
    )
    const auth = useAuthStore()
    await auth.login('a@x.id', 'temp')
    expect(auth.mustChangePassword).toBe(true)

    await auth.changePassword('temp', 'Sandi-Baru-2026')

    expect(svc.changePassword).toHaveBeenCalledWith('temp', 'Sandi-Baru-2026')
    expect(auth.mustChangePassword).toBe(false)
    expect(auth.accessToken).toBe('baru')
  })
})
