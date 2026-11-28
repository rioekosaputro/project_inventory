import { api } from '@/lib/axios'
import type { ListParams, Page } from '@/services/masterService'

export type TicketStatus =
  'OPEN' | 'ASSIGNED' | 'IN_PROGRESS' | 'WAITING_PART' | 'RESOLVED' | 'CLOSED' | 'CANCELLED'

export type TicketPriority = 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL'

export type TicketCategory =
  'PAPER_JAM' | 'PRINT_QUALITY' | 'HARDWARE' | 'SOFTWARE' | 'NETWORK' | 'PREVENTIVE' | 'OTHER'

export interface MaintenanceTicket {
  id: number
  ticket_number: string
  customer_id: number
  customer_name: string
  customer_site_id: number
  site_name: string
  site_address: string
  site_city: string
  product_serial_id: number
  asset_tag: string
  serial_number: string
  product_name: string
  reported_by_name: string | null
  reported_via: string
  category: TicketCategory
  priority: TicketPriority
  status: TicketStatus
  title: string
  description: string | null
  assigned_to: number | null
  assigned_to_name: string | null
  sla_due_at: string | null
  sla_breached: boolean
  resolution_notes: string | null
  resolved_at: string | null
  closed_at: string | null
  swap_so_id: number | null
  swap_so_number: string | null
  created_by: number
  created_by_name: string
  created_at: string
  updated_at: string
}

export interface TicketActivity {
  id: number
  ticket_id: number
  user_id: number
  user_name: string
  activity_type:
    'COMMENT' | 'STATUS_CHANGE' | 'ASSIGN' | 'VISIT_CHECKIN' | 'VISIT_CHECKOUT' | 'PHOTO'
  message: string | null
  from_status: string | null
  to_status: string | null
  latitude: number | null
  longitude: number | null
  photo_path: string | null
  photo_url: string | null
  created_at: string
}

export interface TicketSparepart {
  id: number
  ticket_id: number
  product_id: number
  sku: string
  product_name: string
  qty: number
  status: string
  lpb_id: number | null
  lpb_number: string | null
  is_billable: boolean
  requested_by: number
  requested_by_name: string
  requested_at: string
  notes: string | null
}

export interface TicketDetail extends MaintenanceTicket {
  activities: TicketActivity[]
  spareparts: TicketSparepart[]
}

export interface CreateTicketPayload {
  customer_id: number
  customer_site_id: number
  product_serial_id: number
  reported_by_name?: string
  reported_via?: string
  category: TicketCategory
  priority: TicketPriority
  title: string
  description?: string
  assigned_to?: number
}

function clean(params: ListParams): ListParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ''),
  ) as ListParams
}

export const ticketService = {
  list(params: ListParams = {}): Promise<Page<MaintenanceTicket>> {
    return api.page<MaintenanceTicket>('/tickets', { params: clean(params) })
  },

  get(id: number): Promise<TicketDetail> {
    return api.get<TicketDetail>(`/tickets/${id}`)
  },

  create(payload: CreateTicketPayload): Promise<TicketDetail> {
    return api.post<TicketDetail>('/tickets', payload)
  },

  assign(id: number, assignedTo: number, notes?: string): Promise<TicketDetail> {
    return api.post<TicketDetail>(`/tickets/${id}/assign`, { assigned_to: assignedTo, notes })
  },

  changeStatus(
    id: number,
    payload: { status: TicketStatus; resolution_notes?: string; notes?: string },
  ): Promise<TicketDetail> {
    return api.post<TicketDetail>(`/tickets/${id}/status`, payload)
  },

  addActivity(
    id: number,
    payload:
      | FormData
      | {
          activity_type: 'COMMENT' | 'VISIT_CHECKIN' | 'VISIT_CHECKOUT' | 'PHOTO'
          message?: string
          latitude?: number
          longitude?: number
        },
  ): Promise<TicketActivity> {
    if (payload instanceof FormData) {
      return api.post<TicketActivity>(`/tickets/${id}/activities`, payload, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
    }
    return api.post<TicketActivity>(`/tickets/${id}/activities`, payload)
  },

  requestSparepart(
    ticketId: number,
    payload: { product_id: number; qty: number; is_billable: boolean; notes?: string },
  ): Promise<TicketDetail> {
    return api.post<TicketDetail>(`/tickets/${ticketId}/spareparts`, payload)
  },

  createSparepartLpb(
    sparepartId: number,
    payload: { warehouse_id: number; rack_id: number; notes?: string },
  ): Promise<{ lpb_id: number; lpb_number: string; status: string }> {
    return api.post<{ lpb_id: number; lpb_number: string; status: string }>(
      `/ticket-spareparts/${sparepartId}/lpb`,
      payload,
    )
  },

  escalateSwap(
    ticketId: number,
    payload: { replacement_product_id?: number; notes?: string } = {},
  ): Promise<TicketDetail> {
    return api.post<TicketDetail>(`/tickets/${ticketId}/escalate-swap`, payload)
  },
}
