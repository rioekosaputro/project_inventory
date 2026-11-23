import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { type InternalAxiosRequestConfig } from 'axios'
import { http } from '@/lib/axios'
import { lpbService } from '@/services/lpbService'
import { fakeResponse, ok } from '@/test/helpers'

const originalAdapter = http.defaults.adapter
let seen: InternalAxiosRequestConfig[]

beforeEach(() => {
  seen = []
})
afterEach(() => {
  http.defaults.adapter = originalAdapter
})

function serve(body: unknown, status = 200) {
  http.defaults.adapter = async (config) => {
    seen.push(config)
    return fakeResponse(config, status, body)
  }
}

describe('lpbService', () => {
  it('list/approvals: jalur benar dan parameter kosong dibuang', async () => {
    serve({ ...ok([]), meta: { page: 1, limit: 20, total: 0, total_pages: 0 } })
    await lpbService.list({ page: 1, status: 'DRAFT', q: '', mine: undefined })
    await lpbService.approvals({ sub_type: '' })

    expect(seen[0]!.url).toBe('/lpb')
    expect(seen[0]!.params).toEqual({ page: 1, status: 'DRAFT' })
    expect(seen[1]!.url).toBe('/lpb/approvals')
    expect(seen[1]!.params).toEqual({})
  })

  it('approve mengirim header Idempotency-Key dari pemanggil (kunci sama bila diulang)', async () => {
    serve(ok({ id: 3 }))
    await lpbService.approve(3, 'kunci-tetap-123')
    await lpbService.approve(3, 'kunci-tetap-123')

    expect(seen[0]!.url).toBe('/lpb/3/approve')
    expect(seen[0]!.method).toBe('post')
    expect(seen[0]!.headers.get('Idempotency-Key')).toBe('kunci-tetap-123')
    expect(seen[1]!.headers.get('Idempotency-Key')).toBe('kunci-tetap-123')
  })

  it('aksi lain: jalur dan metode yang benar', async () => {
    serve(ok({}))
    await lpbService.submit(1)
    await lpbService.cancel(1)
    await lpbService.revise(1)
    await lpbService.reject(1, 'Qty salah')
    await lpbService.addSerial(1, 5, { serial_number: 'SN-1', condition: 'GOOD' })
    await lpbService.removeSerial(1, 5, 9)

    expect(seen.map((c) => `${c.method} ${c.url}`)).toEqual([
      'post /lpb/1/submit',
      'post /lpb/1/cancel',
      'post /lpb/1/revise',
      'post /lpb/1/reject',
      'post /lpb/1/items/5/serials',
      'delete /lpb/1/items/5/serials/9',
    ])
    expect(JSON.parse(seen[3]!.data)).toEqual({ reason: 'Qty salah' })
    expect(JSON.parse(seen[4]!.data)).toEqual({ serial_number: 'SN-1', condition: 'GOOD' })
  })

  it('update: PATCH dengan null mengosongkan field', async () => {
    serve(ok({ id: 1 }))
    await lpbService.update(4, { notes: null })
    expect(seen[0]!.method).toBe('patch')
    expect(JSON.parse(seen[0]!.data)).toEqual({ notes: null })
  })
})
