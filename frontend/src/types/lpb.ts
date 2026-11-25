// Bentuk data LPB (plan Bagian 8.3). Ditulis tangan sampai openapi.yaml ada.
// Uang selalu string desimal; tanggal dokumen "YYYY-MM-DD" (WIB), stempel waktu ISO 8601.

export type LPBType = 'INBOUND' | 'OUTBOUND'

export type LPBStatus = 'DRAFT' | 'PENDING_APPROVAL' | 'APPROVED' | 'REJECTED' | 'CANCELLED'

export type LPBSubType =
  | 'PURCHASE'
  | 'INITIAL_STOCK'
  | 'RENTAL_RETURN'
  | 'FAILED_DELIVERY_RETURN'
  | 'TRANSFER_IN'
  | 'OPNAME_ADJUSTMENT_IN'
  | 'RENTAL_DELIVERY'
  | 'CONSUMABLE_DELIVERY'
  | 'SPARE_PART_ISSUE'
  | 'TRANSFER_OUT'
  | 'OPNAME_ADJUSTMENT_OUT'
  | 'DISPOSAL'

export type SerialCondition = 'GOOD' | 'MINOR_DEFECT' | 'BROKEN'

export interface LPB {
  id: number
  lpb_number: string
  type: LPBType
  sub_type: LPBSubType
  status: LPBStatus
  warehouse_id: number
  warehouse_code: string
  doc_date: string
  reference_type: string | null
  reference_number: string | null
  supplier_id: number | null
  supplier_name: string | null
  notes: string | null
  /** Pasangan transfer (TRANSFER_OUT ↔ TRANSFER_IN); null bila bukan transfer. */
  counterpart_id: number | null
  counterpart_number: string | null
  created_by: number
  created_by_name: string
  submitted_by: number | null
  submitted_at: string | null
  approved_by: number | null
  approved_at: string | null
  rejected_by: number | null
  rejected_at: string | null
  rejection_reason: string | null
  created_at: string
  updated_at: string
}

/** Baris daftar: header + ringkasan item. */
export interface LPBListItem extends LPB {
  item_count: number
  total_qty: number
}

export interface LPBItemSerial {
  /** id baris scan (bukan id unit) — dipakai untuk menghapus scan. */
  id: number
  product_serial_id: number | null
  serial_number: string
  asset_tag: string | null
  condition: SerialCondition
  scanned_by: number | null
  scanned_at: string
}

export interface LPBItem {
  id: number
  product_id: number
  sku: string
  product_name: string
  unit: string
  has_serial_number: boolean
  rack_id: number
  rack_code: string
  rack_is_sellable: boolean
  /** Tujuan transfer (hanya TRANSFER_OUT); rak tujuan boleh di gudang lain. */
  dest_rack_id: number | null
  dest_rack_code: string | null
  dest_warehouse_id: number | null
  dest_warehouse_code: string | null
  qty: number
  unit_cost: string | null
  notes: string | null
  scanned_count: number
  /** Saldo rak saat ini (sebelum dokumen disetujui). */
  current_qty: number
  /** Saldo menurut ledger; hanya terisi setelah APPROVED. */
  balance_before: number | null
  balance_after: number | null
  serials: LPBItemSerial[]
}

export interface LPBEvent {
  action: string
  user_id: number | null
  user_name: string | null
  /** after_data audit (mis. {status, reason}); null bila tidak ada. */
  data: Record<string, unknown> | null
  created_at: string
}

export interface LPBDetail extends LPB {
  items: LPBItem[]
  timeline: LPBEvent[]
}

/** Respons POST .../serials: unit yang tersimpan + progres item. */
export interface AddSerialResult extends LPBItemSerial {
  scanned_count: number
  qty: number
}

// ---- payload ----

export interface LPBItemInput {
  product_id: number
  rack_id: number
  /** Hanya untuk TRANSFER_OUT; tidak dikirim untuk jenis lain. */
  dest_rack_id?: number
  qty: number
  unit_cost: string | null
  notes: string | null
}

export interface LPBCreate {
  sub_type: LPBSubType
  warehouse_id: number
  doc_date: string
  supplier_id: number | null
  reference_number: string | null
  notes: string | null
  items: LPBItemInput[]
}

/** PATCH: jenis dan gudang tidak dapat diubah; `items` menggantikan seluruh daftar. */
export type LPBPatch = Partial<Omit<LPBCreate, 'sub_type' | 'warehouse_id'>>

/** Ringkasan verifikasi QR publik GET /public/lpb/verify/:token (tanpa harga/SN/ID). */
export interface LPBVerifySummary {
  lpb_number: string
  type: LPBType
  sub_type: LPBSubType
  status: LPBStatus
  doc_date: string
  warehouse_code: string
  warehouse_name: string
  supplier_name: string | null
  item_count: number
  total_qty: number
  approved_by_name: string | null
  approved_at: string | null
}
