export type InvoiceType = 'RENTAL' | 'CONSUMABLE' | 'SERVICE' | 'OTHER'
export type InvoiceStatus = 'DRAFT' | 'ISSUED' | 'PARTIALLY_PAID' | 'PAID' | 'VOID'
export type PaymentMethod = 'TRANSFER' | 'CASH' | 'GIRO' | 'OTHER'

export interface InvoiceItem {
  id: number
  description: string
  product_id: number | null
  product_serial_id: number | null
  customer_site_id: number | null
  rental_placement_id: number | null
  period_start: string | null
  period_end: string | null
  days_billed: number | null
  days_in_period: number | null
  qty: string
  unit_price: string
  line_total: string
}

export interface InvoicePayment {
  id: number
  invoice_id: number
  payment_date: string
  amount_received: string
  pph23_amount: string
  total_applied: string
  pph23_slip_number: string | null
  method: PaymentMethod
  reference_number: string | null
  proof_path: string | null
  notes: string | null
  recorded_by_name: string
  created_at: string
}

export interface Invoice {
  id: number
  invoice_number: string
  invoice_type: InvoiceType
  customer_id: number
  customer_code: string
  customer_name: string
  customer_npwp: string | null
  contract_id: number | null
  contract_number: string | null
  so_id: number | null
  so_number: string | null
  billing_period_start: string | null
  billing_period_end: string | null
  invoice_date: string
  due_date: string
  status: InvoiceStatus
  is_ppn: boolean
  subtotal: string
  discount_amount: string
  dpp: string
  dpp_other_value: string
  tax_rate: string
  tax_amount: string
  grand_total: string
  paid_amount: string
  remaining_amount: string
  faktur_pajak_number: string | null
  notes: string | null
  generated_by_job: boolean
  issued_at: string | null
  voided_at: string | null
  void_reason: string | null
  created_at: string
  updated_at: string
}

export interface InvoiceDetail extends Invoice {
  items: InvoiceItem[]
  payments: InvoicePayment[]
}

export interface BillingContractResult {
  contract_id: number
  contract_number: string
  customer_name: string
  period_start: string
  period_end: string
  status: string
  invoice_id: number | null
  invoice_number: string | null
  subtotal: string
  dpp: string
  dpp_other_value: string
  tax_rate: string
  tax_amount: string
  grand_total: string
  error?: string
}

export interface BillingRunResult {
  run_date: string
  preview: boolean
  contracts_checked: number
  invoices_created: number
  skipped: number
  errors: number
  results: BillingContractResult[]
}

export interface RecordPaymentPayload {
  payment_date?: string | null
  amount_received: string
  pph23_amount?: string | null
  pph23_slip_number?: string | null
  method: PaymentMethod
  reference_number?: string | null
  notes?: string | null
}
