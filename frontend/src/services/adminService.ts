import { api } from '@/lib/axios'
import type { PageMeta } from '@/types/api'

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

export interface AuditLogRow {
  id: number
  user_id?: number
  user_name?: string
  user_email?: string
  action: string
  entity_type?: string
  entity_id?: number
  before_data?: unknown
  after_data?: unknown
  ip_address?: string
  user_agent?: string
  request_id?: string
  created_at: string
}

export interface AuditLogParams {
  q?: string
  action?: string
  entity_type?: string
  user_id?: number
  date_from?: string
  date_to?: string
  page?: number
  limit?: number
  sort?: string
}

export interface JobRun {
  id: number
  job_name: string
  run_key: string
  status: 'RUNNING' | 'SUCCESS' | 'FAILED' | string
  summary?: Record<string, unknown>
  error?: string
  started_at: string
  finished_at?: string
}

export const adminService = {
  reconcile(): Promise<ReconcileReport> {
    return api.get<ReconcileReport>('/admin/reconcile')
  },
  auditLogs(params?: AuditLogParams): Promise<{ items: AuditLogRow[]; meta: PageMeta }> {
    return api.page<AuditLogRow>('/admin/audit-logs', { params })
  },
  jobs(limit = 50): Promise<JobRun[]> {
    return api.get<JobRun[]>('/admin/jobs', { params: { limit } })
  },
}
