import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { Page } from '@/services/masterService'
import { useAuthStore } from '@/stores/auth'
import type { Permission } from '@/types/auth'
import type { Invoice, InvoiceDetail } from '@/types/invoice'
import InvoiceDetailView from '@/views/invoices/InvoiceDetailView.vue'
import InvoiceListView from '@/views/invoices/InvoiceListView.vue'

vi.mock('primevue/usetoast', () => ({
  useToast: () => ({ add: vi.fn<() => void>() }),
}))

const invSvc = vi.hoisted(() => ({
  list: vi.fn<() => Promise<Page<Invoice>>>(),
  get: vi.fn<(id: number) => Promise<InvoiceDetail>>(),
  generate: vi.fn<() => Promise<unknown>>(),
  issue: vi.fn<() => Promise<InvoiceDetail>>(),
  void: vi.fn<() => Promise<InvoiceDetail>>(),
  recordPayment: vi.fn<() => Promise<InvoiceDetail>>(),
  fetchPdf: vi.fn<() => Promise<Blob>>(),
  exportExcel: vi.fn<() => Promise<Blob>>(),
}))

vi.mock('@/services/invoiceService', () => ({ invoiceService: invSvc }))

const sampleInvoice: Invoice = {
  id: 41,
  invoice_number: 'INV/GLOBAL/2026/12/0001',
  invoice_type: 'RENTAL',
  customer_id: 1,
  customer_code: 'CUST-001',
  customer_name: 'PT Maju Bersama',
  customer_npwp: '01.234.567.8-504.000',
  contract_id: 10,
  contract_number: 'KTR/GLOBAL/2026/01/0001',
  so_id: null,
  so_number: null,
  billing_period_start: '2026-11-01',
  billing_period_end: '2026-11-30',
  invoice_date: '2026-12-01',
  due_date: '2026-12-31',
  status: 'ISSUED',
  is_ppn: true,
  subtotal: '5000000.00',
  discount_amount: '0.00',
  dpp: '5000000.00',
  dpp_other_value: '4583333.00',
  tax_rate: '12.00',
  tax_amount: '550000.00',
  grand_total: '5550000.00',
  paid_amount: '0.00',
  remaining_amount: '5550000.00',
  faktur_pajak_number: '040.000-26.00000001',
  notes: null,
  generated_by_job: false,
  issued_at: '2026-12-01T08:00:00Z',
  voided_at: null,
  void_reason: null,
  created_at: '2026-12-01T07:00:00Z',
  updated_at: '2026-12-01T08:00:00Z',
}

const sampleDetail: InvoiceDetail = {
  ...sampleInvoice,
  items: [
    {
      id: 1,
      description:
        'Sewa HP LaserJet Pro (SN: SN-001) — Kantor Pusat · 2026-11-01 s/d 2026-11-30 (30/30 hari)',
      product_id: 5,
      product_serial_id: 10,
      customer_site_id: 2,
      rental_placement_id: 3,
      period_start: '2026-11-01',
      period_end: '2026-11-30',
      days_billed: 30,
      days_in_period: 30,
      qty: '1',
      unit_price: '5000000.00',
      line_total: '5000000.00',
    },
  ],
  payments: [],
}

function setupAuth(permissions: Permission[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = {
    id: 1,
    name: 'Admin Bisnis',
    email: 'biz@contoh.id',
    phone: null,
    role: 'BUSINESS_ADMIN',
    warehouse_id: null,
    must_change_password: false,
  }
  auth.permissions = permissions
  return pinia
}

function makeTestRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/invoices', component: { template: '<div />' } },
      { path: '/invoices/:id', component: { template: '<div />' } },
      { path: '/pelanggan/:id', component: { template: '<div />' } },
      { path: '/kontrak/:id', component: { template: '<div />' } },
    ],
  })
}

describe('InvoiceListView & InvoiceDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    invSvc.list.mockResolvedValue({
      items: [sampleInvoice],
      meta: { page: 1, limit: 20, total: 1, total_pages: 1 },
    })
    invSvc.get.mockResolvedValue(sampleDetail)
  })

  it('menampilkan daftar tagihan beserta tombol Generate Tagihan Bulanan dan Export Excel', async () => {
    const pinia = setupAuth(['invoice.read', 'invoice.create'])
    const router = makeTestRouter()
    await router.push('/invoices')
    await router.isReady()

    const w = mount(InvoiceListView, { global: { plugins: [pinia, router] } })
    await flushPromises()

    expect(w.text()).toContain('INV/GLOBAL/2026/12/0001')
    expect(w.text()).toContain('PT Maju Bersama')
    expect(w.text()).toContain('Generate Tagihan Bulanan')
    expect(w.text()).toContain('Export Excel')
  })

  it('menampilkan detail invoice ISSUED dengan DPP Nilai Lain, PPN 12%, dan tombol Catat Pembayaran', async () => {
    const pinia = setupAuth(['invoice.read', 'invoice.approve', 'invoice.pay'])
    const router = makeTestRouter()
    await router.push('/invoices/41')
    await router.isReady()

    const w = mount(InvoiceDetailView, { global: { plugins: [pinia, router] } })
    await flushPromises()

    expect(w.text()).toContain('INV/GLOBAL/2026/12/0001')
    expect(w.text()).toContain('DPP Nilai Lain (11/12)')
    expect(w.text()).toContain('040.000-26.00000001')
    expect(w.text()).toContain('Catat Pembayaran')
    expect(w.text()).toContain('Cetak PDF')
  })
})
