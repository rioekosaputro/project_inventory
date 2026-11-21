import { describe, expect, it } from 'vitest'
import { navGroups, visibleNav } from '@/config/navigation'

const allRoutes = () => true

const labels = (groups: ReturnType<typeof visibleNav>) =>
  groups.flatMap((g) => g.items.map((i) => i.label))

describe('visibleNav', () => {
  it('hanya menampilkan item yang permission-nya dimiliki', () => {
    const can = (p: string) => ['dashboard.read', 'lpb.read', 'lpb.approve'].includes(p)

    const out = labels(visibleNav(navGroups, { can, routeExists: allRoutes }))

    expect(out).toEqual(['Beranda', 'Dashboard', 'LPB', 'Persetujuan LPB'])
  })

  it('item tanpa permission (Beranda) tampil untuk semua yang login', () => {
    expect(labels(visibleNav(navGroups, { can: () => false, routeExists: allRoutes }))).toEqual([
      'Beranda',
    ])
  })

  it('item yang rutenya belum ada disembunyikan (halaman fase berikutnya muncul otomatis)', () => {
    const out = labels(
      visibleNav(navGroups, { can: () => true, routeExists: (p) => p === '/beranda' }),
    )
    expect(out).toEqual(['Beranda'])
  })

  it('grup yang kosong dibuang', () => {
    const out = visibleNav(navGroups, { can: (p) => p === 'user.manage', routeExists: allRoutes })
    expect(out.map((g) => g.label)).toEqual(['Utama', 'Administrasi'])
  })

  it('driver hanya melihat tugas pengiriman', () => {
    const out = labels(
      visibleNav(navGroups, {
        can: (p) => p === 'driver' || p === 'lpb.print',
        routeExists: allRoutes,
      }),
    )
    expect(out).toEqual(['Beranda', 'Tugas Pengiriman'])
  })

  it('tidak ada path atau label ganda di konfigurasi', () => {
    const items = navGroups.flatMap((g) => g.items)
    expect(new Set(items.map((i) => i.to)).size).toBe(items.length)
  })

  it('setiap item memakai kunci permission dengan format yang dikenal (hindari salah ketik)', () => {
    const keys = navGroups
      .flatMap((g) => g.items)
      .flatMap((i) => (i.permission ? [i.permission] : []))

    expect(keys.length).toBeGreaterThan(0)
    expect(keys.filter((k) => !/^[a-z]+(\.[a-z]+)?$/.test(k))).toEqual([])
  })
})
