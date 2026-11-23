import { describe, expect, it } from 'vitest'
import { formatDate, formatDateTime, formatRupiah, todayWIB } from '@/lib/format'

const norm = (s: string) => s.replace(/\s/g, ' ')

describe('formatRupiah', () => {
  it('memformat string desimal dari API sebagai Rupiah dengan 2 desimal', () => {
    expect(norm(formatRupiah('550000.00'))).toMatch(/^Rp ?550\.000,00$/)
    expect(norm(formatRupiah('1250000.5'))).toMatch(/^Rp ?1\.250\.000,50$/)
    expect(norm(formatRupiah('0'))).toMatch(/^Rp ?0,00$/)
  })

  it('nilai besar NUMERIC(15,2) tidak kehilangan digit', () => {
    expect(norm(formatRupiah('9999999999999.99'))).toMatch(/^Rp ?9\.999\.999\.999\.999,99$/)
  })

  it('kosong → tanda strip', () => {
    expect(formatRupiah(null)).toBe('—')
    expect(formatRupiah(undefined)).toBe('—')
    expect(formatRupiah('')).toBe('—')
  })
})

describe('tanggal WIB', () => {
  it('todayWIB memakai zona Asia/Jakarta: 23.30 UTC sudah hari berikutnya di WIB', () => {
    expect(todayWIB(new Date('2026-11-21T23:30:00Z'))).toBe('2026-11-22')
    expect(todayWIB(new Date('2026-11-21T16:59:00Z'))).toBe('2026-11-21')
  })

  it('formatDate: tanggal murni tidak bergeser hari; kosong dan tidak valid menjadi strip', () => {
    expect(formatDate('2026-11-21')).toMatch(/21\s+Nov(?:ember)?\s+2026/)
    expect(formatDate(null)).toBe('—')
    expect(formatDate('bukan tanggal')).toBe('—')
  })

  it('formatDateTime menampilkan jam WIB', () => {
    expect(formatDateTime('2026-11-21T10:05:00Z')).toMatch(/17[.:]05/)
    expect(formatDateTime('')).toBe('—')
  })
})
