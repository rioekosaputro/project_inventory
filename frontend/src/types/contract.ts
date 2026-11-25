export type ContractStatus = 'DRAFT' | 'ACTIVE' | 'EXPIRED' | 'TERMINATED'
export type BillingMode = 'PRORATA' | 'FULL_MONTH'

export interface RentalContractItem {
  id: number
  contract_id: number
  product_id: number
  product_sku: string
  product_name: string
  customer_site_id: number | null
  site_name: string | null
  qty: number
  monthly_rate: string
  line_monthly: string
  notes: string | null
}

export interface RentalContract {
  id: number
  contract_number: string
  customer_id: number
  customer_code: string
  customer_name: string
  start_date: string
  end_date: string
  duration_months: number
  billing_day_of_month: number
  billing_mode: BillingMode
  billing_in_advance: boolean
  is_ppn: boolean
  payment_term_days: number
  deposit_amount: string
  status: ContractStatus
  terminated_at: string | null
  termination_reason: string | null
  renewed_from_id: number | null
  renewed_from_number: string | null
  signed_document_path: string | null
  notes: string | null
  created_by: number
  created_by_name: string
  activated_by: number | null
  activated_by_name: string | null
  activated_at: string | null
  item_count: number
  total_qty: number
  estimated_monthly: string
  active_placement_qty: number
  created_at: string
  updated_at: string
}

export interface RentalContractDetail extends RentalContract {
  items: RentalContractItem[]
}

export interface ContractItemPayload {
  product_id: number
  customer_site_id?: number | null
  qty: number
  monthly_rate?: string | null
  notes?: string | null
}

export interface CreateContractPayload {
  customer_id: number
  start_date: string
  end_date?: string
  duration_months: number
  billing_day_of_month?: number
  billing_mode?: BillingMode
  billing_in_advance?: boolean
  is_ppn?: boolean
  payment_term_days?: number
  deposit_amount?: string
  notes?: string | null
  items: ContractItemPayload[]
}

export interface UpdateContractPayload {
  start_date?: string
  end_date?: string
  duration_months?: number
  billing_day_of_month?: number
  billing_mode?: BillingMode
  billing_in_advance?: boolean
  is_ppn?: boolean
  payment_term_days?: number
  deposit_amount?: string
  notes?: string | null
  items?: ContractItemPayload[]
}

export interface TerminateContractPayload {
  terminated_at?: string | null
  termination_reason: string
}

export interface RenewContractPayload {
  start_date?: string | null
  end_date?: string | null
  duration_months?: number | null
  notes?: string | null
}
