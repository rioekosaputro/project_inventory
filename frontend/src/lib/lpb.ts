import type { LPBItem, LPBStatus, LPBSubType, LPBType } from '@/types/lpb'

export const SUB_TYPE_LABELS: Record<LPBSubType, string> = {
  PURCHASE: 'Pembelian',
  INITIAL_STOCK: 'Stok awal',
  RENTAL_RETURN: 'Retur sewa',
  FAILED_DELIVERY_RETURN: 'Retur gagal kirim',
  TRANSFER_IN: 'Transfer masuk',
  OPNAME_ADJUSTMENT_IN: 'Selisih opname (tambah)',
  RENTAL_DELIVERY: 'Pengiriman sewa',
  CONSUMABLE_DELIVERY: 'Pengiriman barang habis pakai',
  SPARE_PART_ISSUE: 'Pengeluaran suku cadang',
  TRANSFER_OUT: 'Transfer keluar',
  OPNAME_ADJUSTMENT_OUT: 'Selisih opname (kurang)',
  DISPOSAL: 'Penghapusan barang rusak',
}

export function subTypeLabel(value: string): string {
  return SUB_TYPE_LABELS[value as LPBSubType] ?? value
}

/**
 * Jenis yang ditawarkan form buat LPB. Backend mengizinkan juga DISPOSAL, tetapi persetujuannya
 * baru ada di T1.8 — menawarkannya sekarang akan menghasilkan dokumen yang tidak bisa selesai.
 */
export const CREATABLE_SUB_TYPES: LPBSubType[] = ['PURCHASE', 'INITIAL_STOCK']

export const STATUS_LABELS: Record<LPBStatus, string> = {
  DRAFT: 'Draft',
  PENDING_APPROVAL: 'Menunggu persetujuan',
  APPROVED: 'Disetujui',
  REJECTED: 'Ditolak',
  CANCELLED: 'Dibatalkan',
}

/** Aksi audit LPB → kalimat riwayat. */
export const EVENT_LABELS: Record<string, string> = {
  LPB_CREATE: 'Draft dibuat',
  LPB_UPDATE: 'Draft diubah',
  LPB_SUBMIT: 'Diajukan untuk persetujuan',
  LPB_CANCEL: 'Draft dibatalkan',
  LPB_REVISE: 'Dibuka kembali untuk revisi',
  LPB_APPROVE: 'Disetujui, stok diperbarui',
  LPB_REJECT: 'Ditolak',
}

export function eventLabel(action: string): string {
  return EVENT_LABELS[action] ?? action
}

/** Seragam dengan backend: pangkas spasi tepi dan huruf besar (dibandingkan tanpa peduli huruf). */
export function normalizeCode(raw: string): string {
  return raw.trim().toUpperCase()
}

/** Tanda efek stok: penerimaan menambah, pengeluaran mengurangi. */
export function stockSign(type: LPBType): 1 | -1 {
  return type === 'INBOUND' ? 1 : -1
}

export interface StockImpact {
  before: number
  after: number
  /** true = angka dari ledger (dokumen sudah disetujui); false = prakiraan dari saldo saat ini. */
  final: boolean
}

/**
 * Saldo rak sebelum → sesudah untuk satu item. Setelah APPROVED angkanya milik ledger;
 * sebelum itu hanya prakiraan dari saldo saat ini (dapat berubah bila ada dokumen lain disetujui lebih dulu).
 * Dokumen dibatalkan tidak punya dampak.
 */
export function stockImpact(item: LPBItem, type: LPBType, status: LPBStatus): StockImpact | null {
  if (status === 'CANCELLED') return null
  if (item.balance_before !== null && item.balance_after !== null) {
    return { before: item.balance_before, after: item.balance_after, final: true }
  }
  if (status === 'APPROVED') return null
  return {
    before: item.current_qty,
    after: item.current_qty + stockSign(type) * item.qty,
    final: false,
  }
}

/** Item produk ber-serial yang masih perlu scan (qty belum terpenuhi). */
export function remainingScans(item: LPBItem): number {
  return item.has_serial_number ? Math.max(item.qty - item.serials.length, 0) : 0
}
