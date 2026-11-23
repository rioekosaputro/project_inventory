import { describe, expect, it, vi } from 'vitest'
import { listAll } from '@/lib/listAll'
import type { ListParams, Page } from '@/services/masterService'

type ListFn = (params: ListParams) => Promise<Page<number>>

const page = (items: number[], p: number, totalPages: number) => ({
  items,
  meta: { page: p, limit: 100, total: items.length, total_pages: totalPages },
})

describe('listAll', () => {
  it('satu halaman: satu permintaan dengan limit 100', async () => {
    const list = vi.fn<ListFn>().mockResolvedValue(page([1, 2], 1, 1))
    const out = await listAll(list, { sort: 'name' })

    expect(list).toHaveBeenCalledTimes(1)
    expect(list).toHaveBeenCalledWith({ sort: 'name', limit: 100, page: 1 })
    expect(out.items).toEqual([1, 2])
  })

  it('beberapa halaman digabung berurutan', async () => {
    const list = vi
      .fn<ListFn>()
      .mockResolvedValueOnce(page([1], 1, 3))
      .mockResolvedValueOnce(page([2], 2, 3))
      .mockResolvedValueOnce(page([3], 3, 3))
    const out = await listAll(list, {})
    expect(out.items).toEqual([1, 2, 3])
    expect(out.meta.limit).toBe(3)
  })

  it('berhenti di batas halaman maksimum', async () => {
    const list = vi
      .fn<ListFn>()
      .mockImplementation((p) => Promise.resolve(page([Number(p.page)], Number(p.page), 50)))
    const out = await listAll(list, {}, 2)
    expect(list).toHaveBeenCalledTimes(2)
    expect(out.items).toEqual([1, 2])
  })
})
