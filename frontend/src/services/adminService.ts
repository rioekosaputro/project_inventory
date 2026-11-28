import { api } from '@/lib/axios'

export interface ReconcileViolation {
  code: string
  check: string
  detail: string
}

export interface ReconcileReport {
  checked_at: string
  healthy: boolean
  total_checks: number
  violation_count: number
  violations: ReconcileViolation[]
}

export const adminService = {
  reconcile(): Promise<ReconcileReport> {
    return api.get<ReconcileReport>('/admin/reconcile')
  },
}
