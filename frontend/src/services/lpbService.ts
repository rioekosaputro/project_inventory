import { api, http, idempotencyHeaders } from '@/lib/axios'
import type { ListParams, Page } from '@/services/masterService'
import type {
  AddSerialResult,
  LPBCreate,
  LPBDetail,
  LPBListItem,
  LPBPatch,
  LPBVerifySummary,
  SerialCondition,
} from '@/types/lpb'

function clean(params: ListParams): ListParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ''),
  ) as ListParams
}

const base = '/lpb'

export const lpbService = {
  list: (params: ListParams = {}): Promise<Page<LPBListItem>> =>
    api.page<LPBListItem>(base, { params: clean(params) }),
  /** Antrean menunggu persetujuan (lpb.approve). */
  approvals: (params: ListParams = {}): Promise<Page<LPBListItem>> =>
    api.page<LPBListItem>(`${base}/approvals`, { params: clean(params) }),
  get: (id: number): Promise<LPBDetail> => api.get<LPBDetail>(`${base}/${id}`),
  create: (body: LPBCreate): Promise<LPBDetail> => api.post<LPBDetail>(base, body),
  update: (id: number, body: LPBPatch): Promise<LPBDetail> =>
    api.patch<LPBDetail>(`${base}/${id}`, body),

  /** Scan satu serial number (atau asset tag untuk DISPOSAL) ke sebuah item. */
  addSerial: (
    id: number,
    itemId: number,
    body: { serial_number: string; condition?: SerialCondition },
  ): Promise<AddSerialResult> =>
    api.post<AddSerialResult>(`${base}/${id}/items/${itemId}/serials`, body),
  removeSerial: (id: number, itemId: number, serialId: number): Promise<void> =>
    api.delete<void>(`${base}/${id}/items/${itemId}/serials/${serialId}`),

  submit: (id: number): Promise<LPBDetail> => api.post<LPBDetail>(`${base}/${id}/submit`),
  cancel: (id: number): Promise<LPBDetail> => api.post<LPBDetail>(`${base}/${id}/cancel`),
  revise: (id: number): Promise<LPBDetail> => api.post<LPBDetail>(`${base}/${id}/revise`),
  /**
   * Aksi kritis: wajib `Idempotency-Key`. Pemanggil membuat kunci SEKALI per niat pengguna dan
   * memakainya lagi bila mengulang setelah galat jaringan, sehingga stok tidak bertambah dua kali.
   */
  approve: (id: number, idempotencyKey: string): Promise<LPBDetail> =>
    api.post<LPBDetail>(`${base}/${id}/approve`, undefined, {
      headers: idempotencyHeaders(idempotencyKey),
    }),
  reject: (id: number, reason: string): Promise<LPBDetail> =>
    api.post<LPBDetail>(`${base}/${id}/reject`, { reason }),
  /** Mengambil PDF Surat Jalan (hanya LPB APPROVED, memakai Bearer token di memori). */
  fetchPdf: async (id: number, download = false): Promise<Blob> => {
    const res = await http.get<Blob>(`${base}/${id}/pdf`, {
      params: download ? { download: 1, fallback: 'html' } : { fallback: 'html' },
      responseType: 'blob',
      headers: { Accept: 'application/pdf, text/html' },
    })
    return res.data
  },
  /** Verifikasi QR publik (tanpa login). */
  verifyPublic: (token: string): Promise<LPBVerifySummary> =>
    api.get<LPBVerifySummary>(`/public/lpb/verify/${encodeURIComponent(token)}`),
  /** Cetak label stiker asset tag batch (PDF A4, POST /inventory/serials/labels/pdf). */
  fetchSerialLabelsPdf: async (serialIds: number[], download = false): Promise<Blob> => {
    const res = await http.post<Blob>(
      '/inventory/serials/labels/pdf',
      { serial_ids: serialIds },
      {
        params: download ? { download: 1, fallback: 'html' } : { fallback: 'html' },
        responseType: 'blob',
        headers: { Accept: 'application/pdf, text/html' },
      },
    )
    return res.data
  },
  /** Unduh template Excel (.xlsx) untuk impor stok awal. */
  downloadInitialStockTemplate: async (): Promise<Blob> => {
    const res = await http.get<Blob>(`${base}/import-initial-stock/template`, {
      responseType: 'blob',
      headers: { Accept: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' },
    })
    return res.data
  },
  /** Unggah Excel/CSV stok awal → membuat draft LPB INITIAL_STOCK. */
  importInitialStock: async (
    warehouseId: number,
    file: File,
    opts?: { docDate?: string; notes?: string },
  ): Promise<LPBDetail> => {
    const form = new FormData()
    form.append('warehouse_id', String(warehouseId))
    form.append('file', file)
    if (opts?.docDate) form.append('doc_date', opts.docDate)
    if (opts?.notes) form.append('notes', opts.notes)
    const res = await http.post<{ data: LPBDetail }>(`${base}/import-initial-stock`, form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    return res.data.data
  },
  /** Tandai LPB pengiriman terkirim (POD manual, multipart jika ada foto atau JSON). */
  deliver: async (
    id: number,
    payload: {
      receiverName: string
      deliveredDate?: string
      notes?: string
      photo?: File | null
    },
  ): Promise<LPBDetail> => {
    if (payload.photo) {
      const form = new FormData()
      form.append('receiver_name', payload.receiverName)
      if (payload.deliveredDate) form.append('delivered_date', payload.deliveredDate)
      if (payload.notes) form.append('notes', payload.notes)
      form.append('photo', payload.photo)
      const res = await http.post<{ data: LPBDetail }>(`${base}/${id}/deliver`, form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
      return res.data.data
    }
    return api.post<LPBDetail>(`${base}/${id}/deliver`, {
      receiver_name: payload.receiverName,
      delivered_date: payload.deliveredDate || undefined,
      notes: payload.notes || undefined,
    })
  },
  /** Tandai unit penarikan (LPB RENTAL_RETURN) sudah dijemput di pelanggan. */
  pickup: (id: number, payload?: { pickupDate?: string; notes?: string }): Promise<LPBDetail> =>
    api.post<LPBDetail>(`${base}/${id}/pickup`, {
      pickup_date: payload?.pickupDate || undefined,
      notes: payload?.notes || undefined,
    }),
}
