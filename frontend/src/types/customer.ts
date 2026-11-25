export interface Customer {
  id: number
  customer_code: string
  company_name: string
  pic_name: string
  phone: string
  email: string | null
  is_pkp: boolean
  npwp: string | null
  tax_address: string | null
  billing_address: string
  billing_postal_code: string
  billing_email: string | null
  payment_term_days: number
  is_active: boolean
  site_count: number
  active_asset_count: number
  created_at: string
  updated_at: string
}

export interface CustomerSite {
  id: number
  customer_id: number
  site_name: string
  pic_name: string
  pic_phone: string
  address_line: string
  subdistrict: string | null
  district: string | null
  city: string
  province: string
  postal_code: string
  latitude: number | null
  longitude: number | null
  is_primary: boolean
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface CustomerDetail extends Customer {
  sites: CustomerSite[]
}

export interface CustomerAsset {
  serial_id: number
  serial_number: string
  asset_tag: string
  product_id: number
  product_sku: string
  product_name: string
  condition: string
  site_id: number | null
  site_name: string | null
  site_city: string | null
  contract_id: number | null
  contract_number: string | null
  updated_at: string
}

export interface CreateCustomerPayload {
  customer_code: string
  company_name: string
  pic_name: string
  phone: string
  email?: string | null
  is_pkp?: boolean
  npwp?: string | null
  tax_address?: string | null
  billing_address: string
  billing_postal_code: string
  billing_email?: string | null
  payment_term_days?: number
}

export interface UpdateCustomerPayload {
  company_name?: string
  pic_name?: string
  phone?: string
  email?: string | null
  is_pkp?: boolean
  npwp?: string | null
  tax_address?: string | null
  billing_address?: string
  billing_postal_code?: string
  billing_email?: string | null
  payment_term_days?: number
  is_active?: boolean
}

export interface CreateCustomerSitePayload {
  site_name: string
  pic_name: string
  pic_phone: string
  address_line: string
  subdistrict?: string | null
  district?: string | null
  city: string
  province: string
  postal_code: string
  latitude?: number | null
  longitude?: number | null
  is_primary?: boolean
}

export interface UpdateCustomerSitePayload {
  site_name?: string
  pic_name?: string
  pic_phone?: string
  address_line?: string
  subdistrict?: string | null
  district?: string | null
  city?: string
  province?: string
  postal_code?: string
  latitude?: number | null
  longitude?: number | null
  is_primary?: boolean
  is_active?: boolean
}
