import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AuditLogView from './AuditLogView.vue'
import JobsView from './JobsView.vue'
import { adminService, type AuditLogRow, type JobRun } from '@/services/adminService'

vi.mock('@/services/adminService', () => ({
  adminService: {
    reconcile: vi.fn<() => Promise<unknown>>(),
    auditLogs: vi.fn<() => Promise<unknown>>(),
    jobs: vi.fn<() => Promise<unknown>>(),
  },
}))

const sampleAuditRows: AuditLogRow[] = [
  {
    id: 101,
    user_id: 1,
    user_name: 'Admin Utama',
    user_email: 'admin@example.com',
    action: 'LPB_APPROVE',
    entity_type: 'lpb',
    entity_id: 42,
    before_data: { status: 'SUBMITTED' },
    after_data: { status: 'APPROVED' },
    ip_address: '127.0.0.1',
    request_id: 'req-abc-123',
    created_at: '2026-11-28T03:00:00Z',
  },
]

const sampleJobs: JobRun[] = [
  {
    id: 11,
    job_name: 'STOCK_RECONCILE',
    run_key: '2026-11-28',
    status: 'SUCCESS',
    summary: { healthy: true, violation_count: 0 },
    started_at: '2026-11-28T00:00:00Z',
    finished_at: '2026-11-28T00:00:01Z',
  },
  {
    id: 12,
    job_name: 'BILLING_MONTHLY',
    run_key: '2026-11-28',
    status: 'FAILED',
    error: 'timeout koneksi DB',
    started_at: '2026-11-28T00:05:00Z',
    finished_at: '2026-11-28T00:05:05Z',
  },
]

describe('AdminViews (T4.3 AuditLogView & JobsView)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('AuditLogView merender daftar log audit dan membuka detail JSON before/after', async () => {
    vi.mocked(adminService.auditLogs).mockResolvedValue({
      items: sampleAuditRows,
      meta: { page: 1, limit: 25, total: 1, total_pages: 1 },
    })

    const wrapper = mount(AuditLogView)
    await flushPromises()

    expect(wrapper.text()).toContain('LPB_APPROVE')
    expect(wrapper.text()).toContain('Admin Utama')
    expect(wrapper.text()).toContain('lpb #42')

    const toggleBtn = wrapper.findAll('button').find((b) => b.text().includes('Lihat JSON'))
    expect(toggleBtn).toBeDefined()
    await toggleBtn!.trigger('click')

    expect(wrapper.text()).toContain('SUBMITTED')
    expect(wrapper.text()).toContain('APPROVED')
  })

  it('JobsView merender KPI status, daftar job runs, dan membuka detail summary/error', async () => {
    vi.mocked(adminService.jobs).mockResolvedValue(sampleJobs)

    const wrapper = mount(JobsView)
    await flushPromises()

    expect(wrapper.text()).toContain('STOCK_RECONCILE')
    expect(wrapper.text()).toContain('BILLING_MONTHLY')
    expect(wrapper.text()).toContain('SUCCESS')
    expect(wrapper.text()).toContain('FAILED')

    const detailButtons = wrapper.findAll('button').filter((b) => b.text().includes('Lihat Detail'))
    expect(detailButtons.length).toBe(2)
    await detailButtons[1]!.trigger('click')

    expect(wrapper.text()).toContain('timeout koneksi DB')
  })
})
