import { api } from '@/lib/axios'
import type { ListParams, Page } from '@/services/masterService'
import type {
  CreateSOPayload,
  DeliveryOrderSummary,
  IssueDOPayload,
  SalesOrder,
  SalesOrderDetail,
  UpdateSOPayload,
} from '@/types/salesOrder'

function clean(params: ListParams): ListParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ''),
  ) as ListParams
}

export const salesOrderService = {
  list(params: ListParams = {}): Promise<Page<SalesOrder>> {
    return api.page<SalesOrder>('/sales-orders', { params: clean(params) })
  },

  get(id: number): Promise<SalesOrderDetail> {
    return api.get<SalesOrderDetail>(`/sales-orders/${id}`)
  },

  create(payload: CreateSOPayload): Promise<SalesOrderDetail> {
    return api.post<SalesOrderDetail>('/sales-orders', payload)
  },

  update(id: number, payload: UpdateSOPayload): Promise<SalesOrderDetail> {
    return api.patch<SalesOrderDetail>(`/sales-orders/${id}`, payload)
  },

  confirm(id: number): Promise<SalesOrderDetail> {
    return api.post<SalesOrderDetail>(`/sales-orders/${id}/confirm`)
  },

  cancel(id: number, cancelReason: string): Promise<SalesOrderDetail> {
    return api.post<SalesOrderDetail>(`/sales-orders/${id}/cancel`, {
      cancel_reason: cancelReason,
    })
  },

  issueDeliveryOrders(id: number, payload: IssueDOPayload): Promise<DeliveryOrderSummary[]> {
    return api.post<DeliveryOrderSummary[]>(`/sales-orders/${id}/delivery-orders`, payload)
  },
}
