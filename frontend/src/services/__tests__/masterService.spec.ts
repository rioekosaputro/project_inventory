import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { type InternalAxiosRequestConfig } from 'axios'
import { http } from '@/lib/axios'
import { productService, warehouseService } from '@/services/masterService'
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

describe('masterService', () => {
  it('list: mengembalikan item dan meta, parameter kosong dibuang dari query', async () => {
    serve({ ...ok([{ id: 1 }]), meta: { page: 2, limit: 10, total: 11, total_pages: 2 } })

    const result = await warehouseService.list({
      page: 2,
      limit: 10,
      q: '',
      sort: 'code',
      is_active: undefined,
    })

    expect(result.items).toEqual([{ id: 1 }])
    expect(result.meta).toEqual({ page: 2, limit: 10, total: 11, total_pages: 2 })
    expect(seen[0]!.url).toBe('/warehouses')
    expect(seen[0]!.params).toEqual({ page: 2, limit: 10, sort: 'code' })
  })

  it('list tanpa meta dari server: meta diturunkan dari isi', async () => {
    serve(ok([{ id: 1 }, { id: 2 }]))
    const result = await productService.list()
    expect(result.meta).toEqual({ page: 1, limit: 2, total: 2, total_pages: 1 })
  })

  it('get/create/update memakai jalur REST yang benar dan membuka envelope', async () => {
    serve(ok({ id: 5, code: 'WH-1' }))
    expect(await warehouseService.get(5)).toEqual({ id: 5, code: 'WH-1' })
    expect(seen[0]).toMatchObject({ method: 'get', url: '/warehouses/5' })

    await warehouseService.create({
      code: 'WH-1',
      name: 'G',
      address: null,
      phone: null,
      latitude: null,
      longitude: null,
    })
    expect(seen[1]).toMatchObject({ method: 'post', url: '/warehouses' })

    await warehouseService.update(5, { is_active: false })
    expect(seen[2]).toMatchObject({ method: 'patch', url: '/warehouses/5' })
    expect(JSON.parse(String(seen[2]!.data))).toEqual({ is_active: false })
  })
})
