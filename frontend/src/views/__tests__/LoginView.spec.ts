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
  svc.refresh.mockRejectedValue(new ApiError(401, 'INVALID_TOKEN', 'Tidak ada sesi')) // anonim
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
})

async function openLogin(path = '/login') {
  const app = await mountApp(path)
  wrapper = app.wrapper
  return app
}

const email = (w: VueWrapper) => w.find<HTMLInputElement>('#field-email')
const pass = (w: VueWrapper) => w.find<HTMLInputElement>('#field-password')

async function fill(w: VueWrapper, mail: string, pw: string) {
  await email(w).setValue(mail)
  await pass(w).setValue(pw)
}

describe('tampilan awal', () => {
  it('Form (auth): kartu dengan heading, tanpa sidebar', async () => {
    const { wrapper: w } = await openLogin()

    expect(w.find('h1').text()).toBe('Masuk')
    expect(w.find('aside').exists()).toBe(false)
    expect(w.find('main#main').exists()).toBe(true)
    expect(w.find('a.skip-link').attributes('href')).toBe('#main')
  })

  it('field memakai atribut autofill/keyboard yang benar', async () => {
    const { wrapper: w } = await openLogin()

    const mail = email(w).attributes()
    expect(mail.type).toBe('email')
    expect(mail.autocomplete).toBe('username')
    expect(mail.inputmode).toBe('email')
    expect(mail.spellcheck).toBe('false')
    expect(mail).toHaveProperty('required')
    expect(w.find('label[for="field-email"]').text()).toContain('Email')

    const pw = pass(w).attributes()
    expect(pw.type).toBe('password')
    expect(pw.autocomplete).toBe('current-password')
    expect(w.find('form').attributes('novalidate')).toBeDefined()
  })

  it('tidak ada error di awal; aria-invalid="false" dan tidak ada aria-describedby', async () => {
    const { wrapper: w } = await openLogin()

    for (const input of [email(w), pass(w)]) {
      expect(input.attributes('aria-invalid')).toBe('false')
      expect(input.attributes('aria-describedby')).toBeUndefined()
    }
    expect(w.text()).not.toContain('Masukkan email')
  })

  it('tombol submit tidak pernah disabled', async () => {
    const { wrapper: w } = await openLogin()
    expect(w.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })
})

describe('validasi: blur-first, hilang saat fokus', () => {
  it('mengetik di field pristine TIDAK memunculkan error', async () => {
    const { wrapper: w } = await openLogin()

    await email(w).setValue('j')
    await settle()

    expect(w.text()).not.toContain('Tulis email seperti')
    expect(email(w).attributes('aria-invalid')).toBe('false')
  })

  it('error muncul saat blur, tertaut ke input, lalu hilang saat fokus', async () => {
    const { wrapper: w } = await openLogin()

    await email(w).setValue('j@')
    await email(w).trigger('blur')
    await settle()

    const err = w.find('#field-email-error')
    expect(err.text()).toBe('Tulis email seperti nama@perusahaan.id')
    expect(email(w).attributes('aria-invalid')).toBe('true')
    expect(email(w).attributes('aria-describedby')).toBe('field-email-error')

    await email(w).trigger('focus')
    await settle()

    expect(w.find('#field-email-error').exists()).toBe(false)
    expect(email(w).attributes('aria-invalid')).toBe('false')
    expect(email(w).attributes('aria-describedby')).toBeUndefined()
  })

  it('error juga hilang pada ketikan perbaikan pertama', async () => {
    const { wrapper: w } = await openLogin()
    await email(w).trigger('blur') // kosong → wajib
    await settle()
    expect(w.find('#field-email-error').text()).toContain('Masukkan email Anda')

    await email(w).setValue('b')
    await settle()

    expect(w.find('#field-email-error').exists()).toBe(false)
  })

  it('error diumumkan lewat satu live region polite (bukan tiap pesan)', async () => {
    const { wrapper: w } = await openLogin()

    await email(w).trigger('blur')
    await settle()
    await new Promise((r) => setTimeout(r, 650)) // jeda 500 ms agar tidak menimpa label field berikutnya

    const live = w.find('[aria-live="polite"]')
    expect(live.text()).toContain('Email: Masukkan email Anda')
    expect(w.find('#field-email-error').attributes('role')).toBeUndefined()
  })
})

describe('validasi antar-field (regresi race di browser)', () => {
  it('blur di Password TIDAK memunculkan kembali error Email yang baru dihapus saat fokus', async () => {
    const { wrapper: w } = await openLogin()
    await pass(w).trigger('focus')
    await email(w).setValue('j@')
    await email(w).trigger('blur') // Email error
    await settle()
    expect(w.find('#field-email-error').exists()).toBe(true)

    // Pengguna pindah dari Password ke Email: blur Password memulai validasi, lalu fokus Email
    // menghapus error-nya sebelum validasi Password selesai.
    await pass(w).trigger('focus')
    await pass(w).trigger('blur')
    await email(w).trigger('focus')
    await settle()

    expect(w.find('#field-email-error').exists()).toBe(false)
    expect(email(w).attributes('aria-invalid')).toBe('false')
    expect(w.find('#field-password-error').exists()).toBe(true) // errornya sendiri tetap tampil
  })

  it('blur satu field tidak memunculkan error di field lain yang belum disentuh', async () => {
    const { wrapper: w } = await openLogin()

    await email(w).trigger('blur')
    await settle()

    expect(w.find('#field-email-error').exists()).toBe(true)
    expect(w.find('#field-password-error').exists()).toBe(false)
    expect(pass(w).attributes('aria-invalid')).toBe('false')
  })

  it('hasil validasi yang terlambat tidak muncul bila pengguna sudah kembali ke field itu', async () => {
    const { wrapper: w } = await openLogin()

    await email(w).trigger('blur') // validasi dimulai...
    await email(w).trigger('focus') // ...pengguna langsung kembali sebelum selesai
    await settle()

    expect(w.find('#field-email-error').exists()).toBe(false)
  })
})
describe('submit', () => {
  it('data tidak valid: semua error tampil, fokus ke field invalid pertama, API tidak dipanggil', async () => {
    const { wrapper: w } = await openLogin()

    await w.find('form').trigger('submit')
    await settle()

    expect(w.find('#field-email-error').exists()).toBe(true)
    expect(w.find('#field-password-error').exists()).toBe(true)
    expect(document.activeElement).toBe(email(w).element)
    expect(svc.login).not.toHaveBeenCalled()

    // Fokus yang dipindahkan program tidak menghapus error; ketikan pengguna menghapusnya.
    expect(email(w).attributes('aria-invalid')).toBe('true')
    await email(w).setValue('b')
    await settle()
    expect(w.find('#field-email-error').exists()).toBe(false)
  })

  it('berhasil: memanggil login dengan email ter-trim lalu menuju beranda', async () => {
    svc.login.mockResolvedValue(makeSession({ default_home: '/dashboard' }))
    const { wrapper: w, router } = await openLogin()

    await fill(w, '  budi@contoh.id ', 'Rahasia-123')
    await w.find('form').trigger('submit')
    await settle()

    expect(svc.login).toHaveBeenCalledWith('budi@contoh.id', 'Rahasia-123')
    expect(router.currentRoute.value.path).toBe('/beranda') // /dashboard belum ada → fallback
  })

  it('berhasil dengan ?redirect= aman → kembali ke halaman yang diminta', async () => {
    svc.login.mockResolvedValue(makeSession())
    const { wrapper: w, router } = await openLogin('/login?redirect=/akun/password')

    await fill(w, 'budi@contoh.id', 'Rahasia-123')
    await w.find('form').trigger('submit')
    await settle()

    expect(router.currentRoute.value.path).toBe('/akun/password')
  })

  it('?redirect= berbahaya diabaikan', async () => {
    svc.login.mockResolvedValue(makeSession())
    const { wrapper: w, router } = await openLogin('/login?redirect=//evil.example')

    await fill(w, 'budi@contoh.id', 'Rahasia-123')
    await w.find('form').trigger('submit')
    await settle()

    expect(router.currentRoute.value.path).toBe('/beranda')
  })

  it('kredensial salah: pesan di atas form, nilai dipertahankan, tetap di /login', async () => {
    svc.login.mockRejectedValue(
      new ApiError(401, 'INVALID_CREDENTIALS', 'Email atau password salah'),
    )
    const { wrapper: w, router } = await openLogin()

    await fill(w, 'budi@contoh.id', 'salah-banget')
    await w.find('form').trigger('submit')
    await settle()

    const alert = w.find('[role="alert"]')
    expect(alert.text()).toContain('Email atau password salah')
    expect(alert.text()).toContain('hubungi administrator') // arahan pemulihan
    expect(email(w).element.value).toBe('budi@contoh.id')
    expect(pass(w).element.value).toBe('salah-banget')
    expect(router.currentRoute.value.path).toBe('/login')
  })

  it('terlalu banyak percobaan: menyebut berapa menit lagi', async () => {
    svc.login.mockRejectedValue(new ApiError(429, 'TOO_MANY_REQUESTS', 'x', [], undefined, 600))
    const { wrapper: w } = await openLogin()

    await fill(w, 'budi@contoh.id', 'salah')
    await w.find('form').trigger('submit')
    await settle()

    expect(w.find('[role="alert"]').text()).toContain('Coba lagi dalam 10 menit')
  })

  it('gangguan jaringan: pesan berisi arahan, bukan error teknis', async () => {
    svc.login.mockRejectedValue(
      new ApiError(
        0,
        'NETWORK_ERROR',
        'Tidak dapat terhubung ke server. Periksa koneksi internet Anda lalu coba lagi.',
      ),
    )
    const { wrapper: w } = await openLogin()

    await fill(w, 'budi@contoh.id', 'x')
    await w.find('form').trigger('submit')
    await settle()

    expect(w.find('[role="alert"]').text()).toContain('Periksa koneksi')
  })

  it('kirim ganda dicegah tanpa menonaktifkan tombol; tombol menunjukkan progres', async () => {
    let resolveLogin: (s: ReturnType<typeof makeSession>) => void = () => {}
    svc.login.mockReturnValue(new Promise((r) => (resolveLogin = r)))
    const { wrapper: w } = await openLogin()

    await fill(w, 'budi@contoh.id', 'Rahasia-123')
    await w.find('form').trigger('submit')
    await settle()
    await w.find('form').trigger('submit')
    await settle()

    expect(svc.login).toHaveBeenCalledTimes(1)
    const btn = w.find('button[type="submit"]')
    expect(btn.text()).toContain('Memproses')
    expect(btn.attributes('aria-busy')).toBe('true')
    expect(btn.attributes('disabled')).toBeUndefined()

    resolveLogin(makeSession())
    await settle()
  })
})

describe('tampilkan password', () => {
  it('tombol mengganti tipe input dan melaporkan statusnya (aria-pressed)', async () => {
    const { wrapper: w } = await openLogin()
    const toggle = () => w.find('button[aria-pressed]')

    expect(toggle().attributes('aria-label')).toBe('Tampilkan password')
    expect(toggle().attributes('aria-pressed')).toBe('false')

    await toggle().trigger('click')

    expect(pass(w).attributes('type')).toBe('text')
    expect(toggle().attributes('aria-label')).toBe('Sembunyikan password')
    expect(toggle().attributes('aria-pressed')).toBe('true')
    expect(toggle().attributes('type')).toBe('button') // tidak ikut men-submit form
  })
})

describe('sudah login', () => {
  it('membuka /login dialihkan ke beranda', async () => {
    svc.refresh.mockResolvedValue(makeSession())
    const { router } = await openLogin()
    expect(router.currentRoute.value.path).toBe('/beranda')
  })
})

describe('batas sesi', () => {
  it.each([
    ['idle', 'tidak ada aktivitas'],
    ['expired', 'Sesi Anda sudah berakhir'],
  ])('menjelaskan alasan keluar (?reason=%s)', async (reason, text) => {
    const { wrapper: w } = await openLogin(`/login?reason=${reason}`)
    expect(w.text()).toContain(text)
  })

  it('tanpa reason tidak ada pemberitahuan', async () => {
    const { wrapper: w } = await openLogin()
    expect(w.text()).not.toContain('berakhir')
  })
})
