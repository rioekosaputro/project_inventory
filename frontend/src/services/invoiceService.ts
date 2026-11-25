import { api, http } from '@/lib/axios'
import type { ListParams, Page } from '@/services/masterService'
import type {
  BillingRunResult,
  Invoice,
  InvoiceDetail,
  RecordPaymentPayload,
} from '@/types/invoice'

function clean(params: ListParams): ListParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ''),
  ) as ListParams
}

export const invoiceService = {
  list(params: ListParams = {}): Promise<Page<Invoice>> {
    return api.page<Invoice>('/invoices', { params: clean(params) })
  },

  get(id: number): Promise<InvoiceDetail> {
    return api.get<InvoiceDetail>(`/invoices/${id}`)
  },

  generate(
    payload: { run_date?: string | null; contract_id?: number | null } = {},
    preview = false,
  ): Promise<BillingRunResult> {
    return api.post<BillingRunResult>('/invoices/generate', payload, {
      params: preview ? { preview: 1 } : undefined,
    })
  },

  issue(id: number, fakturPajakNumber?: string | null): Promise<InvoiceDetail> {
    return api.post<InvoiceDetail>(`/invoices/${id}/issue`, {
      faktur_pajak_number: fakturPajakNumber || null,
    })
  },

  void(id: number, voidReason: string): Promise<InvoiceDetail> {
    return api.post<InvoiceDetail>(`/invoices/${id}/void`, { void_reason: voidReason })
  },

  recordPayment(
    id: number,
    payload: RecordPaymentPayload,
    idempotencyKey: string,
  ): Promise<InvoiceDetail> {
    return api.post<InvoiceDetail>(`/invoices/${id}/payments`, payload, {
      headers: { 'Idempotency-Key': idempotencyKey },
    })
  },

  async fetchPdf(id: number, download = false): Promise<Blob> {
    const res = await http.get<Blob>(`/invoices/${id}/pdf`, {
      params: download ? { download: 1 } : undefined,
      responseType: 'blob',
      headers: { Accept: 'application/pdf' },
    })
    return res.data
  },

  async exportExcel(params: ListParams = {}): Promise<Blob> {
    const res = await http.get<Blob>('/invoices/export', {
      params: clean(params),
      responseType: 'blob',
      headers: { Accept: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' },
    })
    return res.data
  },
}
