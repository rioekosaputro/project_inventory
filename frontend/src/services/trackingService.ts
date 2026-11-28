import { api } from '@/lib/axios'

export type TrackingStatus = 'PICKED_UP' | 'IN_TRANSIT' | 'ARRIVED' | 'DELIVERED' | 'FAILED'

export interface DriverDeliveryTask {
  lpb_id: number
  lpb_number: string
  type: 'INBOUND' | 'OUTBOUND'
  sub_type: string
  status: string
  delivery_status: string | null
  doc_date: string
  warehouse_id: number
  warehouse_code: string
  warehouse_name: string
  customer_id: number | null
  customer_name: string | null
  customer_site_id: number | null
  site_name: string | null
  site_address: string | null
  site_city: string | null
  site_pic_name: string | null
  site_pic_phone: string | null
  site_latitude: number | null
  site_longitude: number | null
  item_count: number
  total_qty: number
  notes: string | null
}

export interface ShipmentTrackingLog {
  id: number
  lpb_id: number
  driver_id: number
  driver_name: string
  status: TrackingStatus
  latitude: number
  longitude: number
  accuracy_m: number | null
  recorded_at: string
  client_time: string | null
  client_event_id: string | null
  receiver_name: string | null
  photo_path: string | null
  failure_reason: string | null
  notes: string | null
}

export interface CheckpointPayload {
  status: 'PICKED_UP' | 'IN_TRANSIT' | 'ARRIVED'
  latitude: number
  longitude: number
  accuracy_m?: number
  client_time?: string
  client_event_id?: string
  notes?: string
}

export interface CheckpointResponse {
  checkpoint: ShipmentTrackingLog
  idempotent: boolean
}

export interface DriverDeliverResponse {
  checkpoint: ShipmentTrackingLog
  idempotent: boolean
  distance_meters: number | null
  distance_warning: string | null
}

export interface DriverFailPayload {
  reason: string
  latitude: number
  longitude: number
  accuracy_m?: number
  client_time?: string
  client_event_id?: string
  notes?: string
}

export interface DriverPickupPayload {
  latitude: number
  longitude: number
  accuracy_m?: number
  client_time?: string
  client_event_id?: string
  notes?: string
}

export interface TrackingRoute {
  lpb_id: number
  lpb_number: string
  delivery_status: string | null
  receiver_name: string | null
  delivered_at: string | null
  pod_photo_url: string | null
  logs: ShipmentTrackingLog[]
}

export const trackingService = {
  myDeliveries(): Promise<DriverDeliveryTask[]> {
    return api.get<DriverDeliveryTask[]>('/tracking/my-deliveries')
  },

  checkpoint(lpbId: number, payload: CheckpointPayload): Promise<CheckpointResponse> {
    return api.post<CheckpointResponse>(`/tracking/lpb/${lpbId}/checkpoint`, payload)
  },

  deliver(lpbId: number, formData: FormData): Promise<DriverDeliverResponse> {
    return api.post<DriverDeliverResponse>(`/tracking/lpb/${lpbId}/deliver`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },

  fail(lpbId: number, payload: DriverFailPayload): Promise<CheckpointResponse> {
    return api.post<CheckpointResponse>(`/tracking/lpb/${lpbId}/fail`, payload)
  },

  pickup(lpbId: number, payload: DriverPickupPayload): Promise<CheckpointResponse> {
    return api.post<CheckpointResponse>(`/tracking/lpb/${lpbId}/pickup`, payload)
  },

  route(lpbId: number): Promise<TrackingRoute> {
    return api.get<TrackingRoute>(`/tracking/lpb/${lpbId}/route`)
  },
}
