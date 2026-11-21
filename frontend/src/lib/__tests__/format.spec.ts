import { describe, expect, it } from 'vitest'
import { formatRupiah } from '@/lib/format'

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
