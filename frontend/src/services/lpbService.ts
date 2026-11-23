import { api, idempotencyHeaders } from '@/lib/axios'
import type { ListParams, Page } from '@/services/masterService'
import type {
  AddSerialResult,
  LPBCreate,
  LPBDetail,
  LPBListItem,
  LPBPatch,
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
}
