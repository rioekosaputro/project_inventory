import { describe, expect, it } from 'vitest'
import { lpbItem, lpbSerial } from '@/test/fixtures'
import {
  CREATABLE_SUB_TYPES,
  eventLabel,
  normalizeCode,
  remainingScans,
  scansRegisteredUnits,
  stockImpact,
  subTypeLabel,
} from '@/lib/lpb'

describe('normalizeCode', () => {
  it('memangkas spasi tepi dan membesarkan huruf (sama dengan backend)', () => {
    expect(normalizeCode('  sn-abc 1 \n')).toBe('SN-ABC 1')
    expect(normalizeCode('   ')).toBe('')
  })
})

describe('stockImpact', () => {
  it('penerimaan yang belum disetujui: prakiraan saldo bertambah', () => {
    const item = lpbItem({ qty: 3, current_qty: 5 })
    expect(stockImpact(item, 'INBOUND', 'PENDING_APPROVAL')).toEqual({
      before: 5,
      after: 8,
      final: false,
    })
    expect(stockImpact(item, 'INBOUND', 'DRAFT')?.after).toBe(8)
  })

  it('pengeluaran: prakiraan saldo berkurang', () => {
    const item = lpbItem({ qty: 2, current_qty: 5 })
    expect(stockImpact(item, 'OUTBOUND', 'PENDING_APPROVAL')).toEqual({
      before: 5,
      after: 3,
      final: false,
    })
  })

  it('setelah disetujui memakai angka ledger, bukan saldo saat ini', () => {
    const item = lpbItem({ qty: 3, current_qty: 99, balance_before: 5, balance_after: 8 })
    expect(stockImpact(item, 'INBOUND', 'APPROVED')).toEqual({ before: 5, after: 8, final: true })
  })

  it('dibatalkan tidak punya dampak; disetujui tanpa ledger tidak mengarang angka', () => {
    expect(stockImpact(lpbItem(), 'INBOUND', 'CANCELLED')).toBeNull()
    expect(stockImpact(lpbItem(), 'INBOUND', 'APPROVED')).toBeNull()
  })

  it('ditolak masih menampilkan prakiraan (dokumen dapat direvisi)', () => {
    expect(stockImpact(lpbItem({ qty: 1, current_qty: 0 }), 'INBOUND', 'REJECTED')?.after).toBe(1)
  })
})

describe('remainingScans', () => {
  it('menghitung sisa scan; produk tanpa serial selalu 0; tidak pernah negatif', () => {
    expect(remainingScans(lpbItem({ qty: 3, serials: [lpbSerial()] }))).toBe(2)
    expect(remainingScans(lpbItem({ qty: 1, serials: [lpbSerial(), lpbSerial({ id: 2 })] }))).toBe(
      0,
    )
    expect(remainingScans(lpbItem({ has_serial_number: false, qty: 5 }))).toBe(0)
  })
})

describe('label', () => {
  it('jenis dan aksi dikenal diterjemahkan; yang tidak dikenal ditampilkan apa adanya', () => {
    expect(subTypeLabel('PURCHASE')).toBe('Pembelian')
    expect(subTypeLabel('LAIN')).toBe('LAIN')
    expect(eventLabel('LPB_APPROVE')).toContain('Disetujui')
    expect(eventLabel('X')).toBe('X')
  })

  it('form menawarkan jenis yang dapat dibuat manual dan disetujui', () => {
    expect(CREATABLE_SUB_TYPES).toEqual(['PURCHASE', 'INITIAL_STOCK', 'TRANSFER_OUT', 'DISPOSAL'])
  })

  it('hanya penghapusan dan transfer keluar yang memindahkan unit terdaftar (scan asset tag atau SN)', () => {
    expect(scansRegisteredUnits('DISPOSAL')).toBe(true)
    expect(scansRegisteredUnits('TRANSFER_OUT')).toBe(true)
    expect(scansRegisteredUnits('PURCHASE')).toBe(false)
    expect(scansRegisteredUnits('INITIAL_STOCK')).toBe(false)
  })
})
