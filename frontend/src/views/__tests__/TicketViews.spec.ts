import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import TicketListView from '@/views/tickets/TicketListView.vue'
import type { MaintenanceTicket } from '@/services/ticketService'
import type { Page } from '@/services/masterService'

const mockTicketService = vi.hoisted(() => ({
  list: vi.fn<() => Promise<Page<MaintenanceTicket>>>(),
}))

vi.mock('@/services/ticketService', () => ({
  ticketService: mockTicketService,
}))

describe('TicketListView (T3.8)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
  })

  it('menampilkan papan Kanban tiket berdasarkan status', async () => {
    mockTicketService.list.mockResolvedValue({
      items: [
        {
          id: 1,
          ticket_number: 'TKT/202611/0001',
          customer_id: 1,
          customer_name: 'PT Sukses',
          customer_site_id: 1,
          site_name: 'Pusat',
          site_address: 'Jl. A',
          site_city: 'Semarang',
          product_serial_id: 10,
          asset_tag: 'AST-0010',
          serial_number: 'SN-10',
          product_name: 'Printer Laser',
          reported_by_name: 'Budi',
          reported_via: 'PHONE',
          category: 'PAPER_JAM',
          priority: 'HIGH',
          status: 'OPEN',
          title: 'Paper jam berulang',
          description: null,
          assigned_to: null,
          assigned_to_name: null,
          sla_due_at: '2026-11-28T18:00:00Z',
          sla_breached: false,
          resolution_notes: null,
          resolved_at: null,
          closed_at: null,
          swap_so_id: null,
          swap_so_number: null,
          created_by: 1,
          created_by_name: 'Admin',
          created_at: '2026-11-28T10:00:00Z',
          updated_at: '2026-11-28T10:00:00Z',
        },
      ],
      meta: { page: 1, limit: 100, total: 1, total_pages: 1 },
    })

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/tickets', component: TicketListView },
        { path: '/tickets/:id', component: { template: '<div />' } },
      ],
    })
    await router.push('/tickets')
    await router.isReady()

    const w = mount(TicketListView, {
      global: { plugins: [createPinia(), router, PrimeVue, ToastService] },
    })
    await flushPromises()

    expect(w.text()).toContain('TKT/202611/0001')
    expect(w.text()).toContain('Paper jam berulang')
    expect(w.text()).toContain('Terbuka')
  })
})
