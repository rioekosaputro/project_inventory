import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { VueWrapper } from '@vue/test-utils'
import { authService } from '@/services/authService'
import { makeSession } from '@/test/helpers'
import { resetLpbMocks } from '@/test/mockLpbService'
import { mountApp, settle, stubRoute } from '@/test/mountApp'

vi.mock('@/services/authService', () => import('@/test/mockAuthService'))
vi.mock('@/services/lpbService', () => import('@/test/mockLpbService'))

const svc = vi.mocked(authService)

let wrapper: VueWrapper | undefined

beforeEach(() => {
  vi.clearAllMocks()
  svc.logout.mockResolvedValue(null)
  resetLpbMocks()
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
})

// Rute fase berikutnya, didaftarkan seolah modulnya sudah dibangun.
const futureRoutes = [
  stubRoute('/admin/users', 'admin-users', 'user.manage'),
  stubRoute('/driver/deliveries', 'driver-deliveries', 'driver'),
]

async function open(session: ReturnType<typeof makeSession>) {
  svc.refresh.mockResolvedValue(session)
  const app = await mountApp('/beranda', futureRoutes)
  wrapper = app.wrapper
  return app
}

const sidebarLinks = (w: VueWrapper) => w.findAll('aside nav a').map((a) => a.text())

describe('sidebar dinamis per permission', () => {
  it('Kepala Gudang melihat Persetujuan LPB tetapi bukan Pengguna', async () => {
    const { wrapper: w } = await open(
      makeSession({ role: 'HEAD_WAREHOUSE', permissions: ['lpb.read', 'lpb.approve'] }),
    )

    expect(sidebarLinks(w)).toEqual(['Beranda', 'LPB', 'Persetujuan LPB'])
  })

  it('di /lpb/approvals hanya Persetujuan LPB yang aktif (bukan LPB yang awalannya sama)', async () => {
    const { wrapper: w, router } = await open(
      makeSession({ role: 'HEAD_WAREHOUSE', permissions: ['lpb.read', 'lpb.approve'] }),
    )
    const current = () => w.findAll('aside nav a[aria-current="page"]').map((a) => a.text())

    await router.push('/lpb/approvals')
    await settle()
    expect(current()).toEqual(['Persetujuan LPB'])

    await router.push('/lpb')
    await settle()
    expect(current()).toEqual(['LPB'])
  })

  it('Admin melihat menu administrasi', async () => {
    const { wrapper: w } = await open(
      makeSession({ role: 'ADMIN', permissions: ['lpb.approve', 'user.manage'] }),
    )

    expect(sidebarLinks(w)).toEqual(['Beranda', 'Persetujuan LPB', 'Pengguna'])
  })

  it('Driver hanya melihat tugas pengiriman', async () => {
    const { wrapper: w } = await open(makeSession({ role: 'DRIVER', permissions: ['driver'] }))
    expect(sidebarLinks(w)).toEqual(['Beranda', 'Tugas Pengiriman'])
  })

  it('tanpa permission apa pun hanya Beranda; menu yang rutenya belum ada tidak tampil', async () => {
    svc.refresh.mockResolvedValue(makeSession({ permissions: ['ticket.read', 'invoice.read'] }))
    const app = await mountApp('/beranda') // tanpa rute fase berikutnya
    wrapper = app.wrapper

    expect(sidebarLinks(app.wrapper)).toEqual(['Beranda'])
  })

  it('tautan aktif ditandai aria-current', async () => {
    const { wrapper: w } = await open(makeSession())
    expect(w.find('aside nav a[aria-current="page"]').text()).toBe('Beranda')
  })
})

describe('kerangka halaman', () => {
  it('skip link, konten utama, dan lebar konten dibatasi (bukan shell)', async () => {
    const { wrapper: w } = await open(makeSession())

    expect(w.find('a.skip-link').attributes('href')).toBe('#main')
    const main = w.find('main#main')
    expect(main.classes()).toContain('max-w-[1280px]')
    expect(w.find('aside').classes()).not.toContain('max-w-[1280px]')
  })

  it('panel pengguna: nama, peran, ganti password, keluar', async () => {
    const { wrapper: w } = await open(
      makeSession({ role: 'HEAD_WAREHOUSE', user: { name: 'Sari Kepala' } }),
    )
    const panel = w.find('aside')

    expect(panel.text()).toContain('Sari Kepala')
    expect(panel.text()).toContain('Kepala Gudang')
    expect(panel.text()).toContain('Ganti Password')
    expect(panel.text()).toContain('Keluar')
  })

  it('Keluar: memanggil API, membersihkan sesi, kembali ke /login', async () => {
    const { wrapper: w, router, auth } = await open(makeSession())

    const logout = w.findAll('aside button').find((b) => b.text() === 'Keluar')!
    await logout.trigger('click')
    await settle()

    expect(svc.logout).toHaveBeenCalled()
    expect(auth.isAuthenticated).toBe(false)
    // LoginView di-import lazy; tunggu navigasi selesai, bukan menebak durasinya.
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('login'))
  })
})

describe('header', () => {
  it('layar lebar dengan sidebar: header disembunyikan (tidak ada bar kosong)', async () => {
    const { wrapper: w } = await open(makeSession())
    expect(w.find('header').classes()).toContain('md:hidden')
  })

  it('wajib ganti password: header tetap tampil dengan panel pengguna SATU BARIS (tidak menumpuk)', async () => {
    svc.refresh.mockResolvedValue(makeSession({ user: { must_change_password: true } }))
    const app = await mountApp('/beranda')
    wrapper = app.wrapper

    const header = app.wrapper.find('header')
    expect(header.classes()).not.toContain('md:hidden')
    const panel = header.find('div.flex.items-center')
    expect(panel.exists()).toBe(true)
    expect(panel.classes()).not.toContain('space-y-1')
    expect(header.findAll('button').map((b) => b.text())).toContain('Keluar')
    expect(header.find('a[href="/akun/password"]').exists()).toBe(false) // tidak perlu tautan ke halaman ini
  })
})
describe('laci menu (layar sempit)', () => {
  it('tombol menu terhubung ke dialog dan melaporkan statusnya', async () => {
    const { wrapper: w } = await open(makeSession())
    const open_ = w.find('button[aria-label="Buka menu"]')
    const dialog = w.find('dialog#nav-drawer')

    expect(open_.attributes('aria-controls')).toBe('nav-drawer')
    expect(open_.attributes('aria-expanded')).toBe('false')
    expect(dialog.attributes('aria-label')).toBe('Menu navigasi')
    expect(dialog.attributes('open')).toBeUndefined()

    await open_.trigger('click')
    expect(dialog.attributes('open')).toBeDefined()
    expect(open_.attributes('aria-expanded')).toBe('true')

    await w.find('button[aria-label="Tutup menu"]').trigger('click')
    expect(dialog.attributes('open')).toBeUndefined()
    expect(open_.attributes('aria-expanded')).toBe('false')
  })

  it('laci berisi menu yang sama dengan sidebar', async () => {
    const { wrapper: w } = await open(
      makeSession({ role: 'ADMIN', permissions: ['lpb.approve', 'user.manage'] }),
    )

    const drawerLinks = w.findAll('dialog nav a').map((a) => a.text())
    expect(drawerLinks).toEqual(sidebarLinks(w))
  })

  it('tombol menu dan tutup memenuhi target sentuh 44px', async () => {
    const { wrapper: w } = await open(makeSession())

    for (const label of ['Buka menu', 'Tutup menu']) {
      const classes = w.find(`button[aria-label="${label}"]`).classes()
      expect(classes).toContain('min-h-11')
      expect(classes).toContain('min-w-11')
    }
  })

  it('klik backdrop (dialog itu sendiri) menutup laci; klik di dalam konten tidak', async () => {
    const { wrapper: w } = await open(makeSession())
    const dialog = w.find('dialog#nav-drawer')
    await w.find('button[aria-label="Buka menu"]').trigger('click')

    await dialog.find('nav').trigger('click')
    expect(dialog.attributes('open')).toBeDefined()

    await dialog.trigger('click')
    expect(dialog.attributes('open')).toBeUndefined()
  })

  it('berpindah halaman menutup laci', async () => {
    const { wrapper: w, router } = await open(makeSession({ permissions: ['lpb.approve'] }))
    await w.find('button[aria-label="Buka menu"]').trigger('click')

    await router.push('/akun/password')
    await settle()

    expect(w.find('dialog#nav-drawer').attributes('open')).toBeUndefined()
  })
})
