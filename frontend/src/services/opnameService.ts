import { api, http } from '@/lib/axios'
import type { ListParams, Page } from '@/services/masterService'
import type { ApiEnvelope } from '@/types/api'
import type {
  OpnameCountInput,
  OpnameDetail,
  OpnameListItem,
  OpnameSerial,
  StartOpnameInput,
} from '@/types/opname'

function clean(params: ListParams): ListParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ''),
  ) as ListParams
}

const base = '/inventory/opnames'

export const opnameService = {
  list: (params: ListParams = {}): Promise<Page<OpnameListItem>> =>
    api.page<OpnameListItem>(base, { params: clean(params) }),
  start: (body: StartOpnameInput): Promise<OpnameDetail> => api.post<OpnameDetail>(base, body),
  get: (id: number): Promise<OpnameDetail> => api.get<OpnameDetail>(`${base}/${id}`),
  saveCounts: async (id: number, items: OpnameCountInput[]): Promise<OpnameDetail> => {
    const res = await http.put<ApiEnvelope<OpnameDetail>>(`${base}/${id}/counts`, { items })
    return res.data.data
  },
  scanSerial: (id: number, rackId: number, code: string): Promise<OpnameSerial> =>
    api.post<OpnameSerial>(`${base}/${id}/scans`, { rack_id: rackId, code }),
  submit: (id: number): Promise<OpnameDetail> => api.post<OpnameDetail>(`${base}/${id}/submit`),
  finalize: (id: number): Promise<OpnameDetail> => api.post<OpnameDetail>(`${base}/${id}/finalize`),
  cancel: (id: number): Promise<OpnameDetail> => api.post<OpnameDetail>(`${base}/${id}/cancel`),
}
