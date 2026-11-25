import { api } from '@/lib/axios'
import type { ListParams, Page } from '@/services/masterService'
import type {
  CreatedLPBFromDO,
  DeliveryOrder,
  DeliveryOrderDetail,
  ReserveDOItemPayload,
} from '@/types/deliveryOrder'

function clean(params: ListParams): ListParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ''),
  ) as ListParams
}

export const deliveryOrderService = {
  list(params: ListParams = {}): Promise<Page<DeliveryOrder>> {
    return api.page<DeliveryOrder>('/delivery-orders', { params: clean(params) })
  },

  get(id: number): Promise<DeliveryOrderDetail> {
    return api.get<DeliveryOrderDetail>(`/delivery-orders/${id}`)
  },

  reserve(id: number, items: ReserveDOItemPayload[]): Promise<DeliveryOrderDetail> {
    return api.post<DeliveryOrderDetail>(`/delivery-orders/${id}/reserve`, { items })
  },

  release(id: number): Promise<DeliveryOrderDetail> {
    return api.post<DeliveryOrderDetail>(`/delivery-orders/${id}/release`)
  },

  cancel(id: number): Promise<DeliveryOrderDetail> {
    return api.post<DeliveryOrderDetail>(`/delivery-orders/${id}/cancel`)
  },

  createLPB(
    id: number,
    payload: { rack_id?: number | null; notes?: string | null } = {},
  ): Promise<CreatedLPBFromDO> {
    return api.post<CreatedLPBFromDO>(`/delivery-orders/${id}/lpb`, payload)
  },
}
