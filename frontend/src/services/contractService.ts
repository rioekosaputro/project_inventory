import { api } from '@/lib/axios'
import type { ListParams, Page } from '@/services/masterService'
import type {
  CreateContractPayload,
  RenewContractPayload,
  RentalContract,
  RentalContractDetail,
  TerminateContractPayload,
  UpdateContractPayload,
} from '@/types/contract'

function clean(params: ListParams): ListParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ''),
  ) as ListParams
}

export const contractService = {
  list(params: ListParams = {}): Promise<Page<RentalContract>> {
    return api.page<RentalContract>('/contracts', { params: clean(params) })
  },

  get(id: number): Promise<RentalContractDetail> {
    return api.get<RentalContractDetail>(`/contracts/${id}`)
  },

  create(payload: CreateContractPayload): Promise<RentalContractDetail> {
    return api.post<RentalContractDetail>('/contracts', payload)
  },

  update(id: number, payload: UpdateContractPayload): Promise<RentalContractDetail> {
    return api.patch<RentalContractDetail>(`/contracts/${id}`, payload)
  },

  activate(id: number): Promise<RentalContractDetail> {
    return api.post<RentalContractDetail>(`/contracts/${id}/activate`)
  },

  terminate(id: number, payload: TerminateContractPayload): Promise<RentalContractDetail> {
    return api.post<RentalContractDetail>(`/contracts/${id}/terminate`, payload)
  },

  renew(id: number, payload: RenewContractPayload = {}): Promise<RentalContractDetail> {
    return api.post<RentalContractDetail>(`/contracts/${id}/renew`, payload)
  },
}
