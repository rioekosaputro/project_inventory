import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { VueWrapper } from '@vue/test-utils'
import { authService } from '@/services/authService'
import { ApiError } from '@/lib/apiError'
import { makeSession } from '@/test/helpers'
import { mountApp, settle } from '@/test/mountApp'

vi.mock('@/services/authService', () => import('@/test/mockAuthService'))

const svc = vi.mocked(authService)

let wrapper: VueWrapper | undefined

beforeEach(() => {
  vi.clearAllMocks()
  svc.refresh.mockResolvedValue(makeSession()) // sudah login
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
})

async function open(session = makeSession()) {
  svc.refresh.mockResolvedValue(session)
  const app = await mountApp('/akun/password')
  wrapper = app.wrapper
  return app
}

const current = (w: VueWrapper) => w.find<HTMLInputElement>('#field-current_password')
const next = (w: VueWrapper) => w.find<HTMLInputElement>('#field-new_password')

async function fill(w: VueWrapper, cur: string, nxt: string) {
  await current(w).setValue(cur)
  await next(w).setValue(nxt)
}

describe('tampilan', () => {
  it('Form: heading, atribut autofill password manager yang benar', async () => {
    const { wrapper: w } = await open()

    expect(w.find('h1').text()).toBe('Ganti Password')
    expect(current(w).attributes('autocomplete')).toBe('current-password')
    expect(next(w).attributes('autocomplete')).toBe('new-password')
    expect(w.findAll('button[aria-pressed]')).toHaveLength(2) // keduanya bisa ditampilkan
    expect(w.find('button[type="submit"]').text()).toBe('Simpan Password')
  })

  it('bantuan ditampilkan lalu digantikan error (bukan ditumpuk)', async () => {
    const { wrapper: w } = await open()
    expect(w.text()).toContain('Minimal 8 karakter')
    expect(next(w).attributes('aria-describedby')).toBe('field-new_password-hint')

    await next(w).setValue('abc')
    await next(w).trigger('blur')
    await settle()

    expect(w.find('#field-new_password-error').text()).toBe('Gunakan minimal 8 karakter')
    expect(w.find('#field-new_password-hint').exists()).toBe(false)
    expect(next(w).attributes('aria-describedby')).toBe('field-new_password-error')
  })

  it('password baru sama dengan yang lama ditolak di sisi klien', async () => {
    const { wrapper: w } = await open()

    await current(w).setValue('Sandi-Lama-123')
    await next(w).setValue('Sandi-Lama-123')
    await next(w).trigger('blur')
    await settle()

    expect(w.find('#field-new_password-error').text()).toContain('berbeda')
  })

  it('tidak ada banner wajib-ganti untuk pengguna biasa', async () => {
    const { wrapper: w } = await open()
    expect(w.text()).not.toContain('wajib mengganti password sementara')
    expect(w.find('aside').exists()).toBe(true)
  })
})

describe('submit', () => {
  it('kosong: error di kedua field, fokus ke yang pertama, API tidak dipanggil', async () => {
    const { wrapper: w } = await open()

    await w.find('form').trigger('submit')
    await settle()

    expect(w.find('#field-current_password-error').exists()).toBe(true)
    expect(w.find('#field-new_password-error').exists()).toBe(true)
    expect(document.activeElement).toBe(current(w).element)
    expect(svc.changePassword).not.toHaveBeenCalled()
  })

  it('berhasil (sukarela): toast, form dikosongkan, tetap di halaman', async () => {
    svc.changePassword.mockResolvedValue(makeSession({ token: 'token-baru' }))
    const { wrapper: w, router, auth } = await open()

    await fill(w, 'Sandi-Lama-123', 'Sandi-Baru-2026')
    await w.find('form').trigger('submit')
    await settle()

    expect(svc.changePassword).toHaveBeenCalledWith('Sandi-Lama-123', 'Sandi-Baru-2026')
    expect(auth.accessToken).toBe('token-baru')
    expect(router.currentRoute.value.path).toBe('/akun/password')
    expect(document.body.textContent).toContain('Password berhasil diganti')
    expect(current(w).element.value).toBe('')
    expect(next(w).element.value).toBe('')
  })

  it('password saat ini salah (422 dari server): error di field itu, fokus ke sana, nilai lain tetap', async () => {
    svc.changePassword.mockRejectedValue(
      new ApiError(422, 'INVALID_DATA', 'Password saat ini salah', [
        { field: 'current_password', message: 'salah' },
      ]),
    )
    const { wrapper: w } = await open()

    await fill(w, 'salah-banget', 'Sandi-Baru-2026')
    await w.find('form').trigger('submit')
    await settle()

    expect(w.find('#field-current_password-error').text()).toContain('Password saat ini salah')
    expect(document.activeElement).toBe(current(w).element)
    expect(next(w).element.value).toBe('Sandi-Baru-2026')
  })

  it('rate limit: pesan menyebut berapa menit lagi', async () => {
    svc.changePassword.mockRejectedValue(
      new ApiError(429, 'TOO_MANY_REQUESTS', 'x', [], undefined, 300),
    )
    const { wrapper: w } = await open()

    await fill(w, 'salah-banget', 'Sandi-Baru-2026')
    await w.find('form').trigger('submit')
    await settle()

    expect(w.find('[role="alert"]').text()).toContain('Coba lagi dalam 5 menit')
  })
})

describe('akun wajib ganti password', () => {
  const forced = () =>
    makeSession({ user: { must_change_password: true }, default_home: '/akun/password' })

  it('banner tampil, tanpa menu navigasi, dan semua halaman lain dialihkan ke sini', async () => {
    const { wrapper: w, router } = await open(forced())
    await router.push('/beranda')
    await settle()

    expect(router.currentRoute.value.path).toBe('/akun/password')
    expect(w.text()).toContain('wajib mengganti password sementara')
    expect(w.find('aside').exists()).toBe(false)
    expect(w.find('dialog').exists()).toBe(false)
    expect(w.find('button[aria-label="Buka menu"]').exists()).toBe(false)
  })

  it('setelah berhasil → menuju beranda dan menu muncul', async () => {
    svc.changePassword.mockResolvedValue(makeSession({ user: { must_change_password: false } }))
    const { wrapper: w, router } = await open(forced())

    await fill(w, 'Sandi-Sementara', 'Sandi-Baru-2026')
    await w.find('form').trigger('submit')
    await settle()

    expect(router.currentRoute.value.path).toBe('/beranda')
    expect(w.find('aside').exists()).toBe(true)
  })
})
