import { api, http } from '@/lib/axios'
import type { ListParams, Page } from '@/services/masterService'
import type {
  LedgerRow,
  SerialDetail,
  SerialItem,
  StockBalance,
  StockCard,
} from '@/types/inventory'

function clean(params: ListParams): ListParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ''),
  ) as ListParams
}

const XLSX_MIME = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'

export const inventoryService = {
  balances: (params: ListParams = {}): Promise<Page<StockBalance>> =>
    api.page<StockBalance>('/inventory/balances', { params: clean(params) }),
  serials: (params: ListParams = {}): Promise<Page<SerialItem>> =>
    api.page<SerialItem>('/inventory/serials', { params: clean(params) }),
  serialHistory: (id: number): Promise<SerialDetail> =>
    api.get<SerialDetail>(`/inventory/serials/${id}/history`),
  ledgers: (params: ListParams = {}): Promise<Page<LedgerRow>> =>
    api.page<LedgerRow>('/inventory/ledgers', { params: clean(params) }),
  exportLedgersXlsx: async (params: ListParams = {}): Promise<Blob> => {
    const res = await http.get<Blob>('/inventory/ledgers/export', {
      params: clean(params),
      responseType: 'blob',
      headers: { Accept: XLSX_MIME },
    })
    return res.data
  },
  stockCard: (params: ListParams): Promise<StockCard> =>
    api.get<StockCard>('/inventory/stock-card', { params: clean(params) }),
  exportStockCardXlsx: async (params: ListParams): Promise<Blob> => {
    const res = await http.get<Blob>('/inventory/stock-card', {
      params: { ...clean(params), format: 'xlsx' },
      responseType: 'blob',
      headers: { Accept: XLSX_MIME },
    })
    return res.data
  },
  fetchSerialLabelsPdf: async (serialIds: number[]): Promise<Blob> => {
    const res = await http.post<Blob>(
      '/inventory/serials/labels/pdf',
      { serial_ids: serialIds },
      {
        params: { fallback: 'html' },
        responseType: 'blob',
        headers: { Accept: 'application/pdf, text/html' },
      },
    )
    return res.data
  },
}
