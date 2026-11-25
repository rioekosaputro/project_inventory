import { api } from '@/lib/axios'
import type { ListParams, Page } from '@/services/masterService'
import type {
  CreateCustomerPayload,
  CreateCustomerSitePayload,
  Customer,
  CustomerAsset,
  CustomerDetail,
  CustomerSite,
  UpdateCustomerPayload,
  UpdateCustomerSitePayload,
} from '@/types/customer'

function clean(params: ListParams): ListParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ''),
  ) as ListParams
}

export const customerService = {
  list(params: ListParams = {}): Promise<Page<Customer>> {
    return api.page<Customer>('/customers', { params: clean(params) })
  },

  get(id: number): Promise<CustomerDetail> {
    return api.get<CustomerDetail>(`/customers/${id}`)
  },

  create(payload: CreateCustomerPayload): Promise<Customer> {
    return api.post<Customer>('/customers', payload)
  },

  update(id: number, payload: UpdateCustomerPayload): Promise<Customer> {
    return api.patch<Customer>(`/customers/${id}`, payload)
  },

  listSites(customerId: number, isActive?: boolean): Promise<CustomerSite[]> {
    const params = isActive === undefined ? undefined : { is_active: isActive }
    return api.get<CustomerSite[]>(`/customers/${customerId}/sites`, { params })
  },

  createSite(customerId: number, payload: CreateCustomerSitePayload): Promise<CustomerSite> {
    return api.post<CustomerSite>(`/customers/${customerId}/sites`, payload)
  },

  updateSite(siteId: number, payload: UpdateCustomerSitePayload): Promise<CustomerSite> {
    return api.patch<CustomerSite>(`/customer-sites/${siteId}`, payload)
  },

  listAssets(customerId: number, siteId?: number): Promise<CustomerAsset[]> {
    const params = siteId === undefined ? undefined : { site_id: siteId }
    return api.get<CustomerAsset[]>(`/customers/${customerId}/assets`, { params })
  },
}
