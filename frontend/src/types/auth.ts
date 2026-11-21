// Bentuk respons API. Ditulis tangan untuk sementara; setelah backend/api/openapi.yaml ada,
// diganti dengan tipe hasil generate (src/types/api.generated.ts).

export const ROLES = [
  'ADMIN',
  'MANAGEMENT',
  'BUSINESS_ADMIN',
  'STAFF_WAREHOUSE',
  'HEAD_WAREHOUSE',
  'DRIVER',
  'TECHNICIAN',
] as const

export type Role = (typeof ROLES)[number]

/** Kunci permission, mis. `lpb.approve` (daftar resmi: backend internal/domain/permission.go). */
export type Permission = string

export interface UserDto {
  id: number
  name: string
  email: string
  phone: string | null
  role: Role
  warehouse_id: number | null
  must_change_password: boolean
}

/** GET /auth/me */
export interface ProfileDto {
  user: UserDto
  permissions: Permission[]
  default_home: string
}

/** POST /auth/login | /auth/refresh | /auth/change-password */
export interface SessionDto extends ProfileDto {
  access_token: string
  token_type: 'Bearer'
  /** detik */
  expires_in: number
}
