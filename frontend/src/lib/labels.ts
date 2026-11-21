import type { Role } from '@/types/auth'

export const ROLE_LABELS: Record<Role, string> = {
  ADMIN: 'Administrator',
  MANAGEMENT: 'Manajemen',
  BUSINESS_ADMIN: 'Admin Bisnis',
  STAFF_WAREHOUSE: 'Staf Gudang',
  HEAD_WAREHOUSE: 'Kepala Gudang',
  DRIVER: 'Driver',
  TECHNICIAN: 'Teknisi',
}

export function roleLabel(role: Role | undefined): string {
  return role ? ROLE_LABELS[role] : ''
}
