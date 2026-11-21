/** Envelope sukses: {success, message, data, meta} (plan Bagian 7.4). */
export interface ApiEnvelope<T> {
  success: true
  message: string
  data: T
  meta?: PageMeta
}

export interface PageMeta {
  page: number
  limit: number
  total: number
  total_pages: number
}

export interface FieldError {
  field: string
  message: string
}

/** Envelope gagal: {success:false, message, code, errors, request_id}. */
export interface ApiErrorBody {
  success: false
  message: string
  code: string
  errors?: FieldError[]
  request_id?: string
}
