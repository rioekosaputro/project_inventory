import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as audio from '@/lib/audio'
import { ApiError } from '@/lib/apiError'
import { useScanQueue } from '@/composables/useScanQueue'
import type { AddSerialResult } from '@/types/lpb'

vi.mock('@/lib/audio', () => ({ beep: vi.fn<(tone: string) => void>() }))

const result = (sn: string, scanned = 1, qty = 5): AddSerialResult => ({
  id: scanned,
  product_serial_id: null,
  serial_number: sn,
  asset_tag: null,
  condition: 'GOOD',
  scanned_by: 1,
  scanned_at: '2026-11-21T10:00:00+07:00',
  scanned_count: scanned,
  qty,
})

/** Janji yang diselesaikan dari luar, untuk mengendalikan urutan balasan server. */
function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const tick = () => new Promise((r) => setTimeout(r, 0))

function setup(over: { known?: string[]; capacity?: number } = {}) {
  const known = new Set(over.known ?? [])
  const calls: { itemId: number; code: string; meta: string }[] = []
  const pending: ReturnType<typeof deferred<AddSerialResult>>[] = []
  const saved: AddSerialResult[] = []
  const queue = useScanQueue<string>({
    send: (itemId, code, meta) => {
      calls.push({ itemId, code, meta })
      const d = deferred<AddSerialResult>()
      pending.push(d)
      return d.promise
    },
    known: () => known,
    capacity: () => over.capacity ?? 10,
    onSaved: (_i, r) => {
      saved.push(r)
      known.add(r.serial_number)
    },
  })
  return { queue, calls, pending, saved }
}

beforeEach(() => vi.mocked(audio.beep).mockClear())

describe('useScanQueue', () => {
  it('menormalkan kode (pangkas + huruf besar), membawa meta, dan mengabaikan kode kosong', () => {
    const { queue, calls } = setup()
    expect(queue.submit('   ', 1, 'GOOD')).toBeNull()
    queue.submit('  sn-1 ', 1, 'MINOR_DEFECT')
    expect(calls).toEqual([{ itemId: 1, code: 'SN-1', meta: 'MINOR_DEFECT' }])
  })

  it('scan dobel dari SN yang sudah tercatat dikenali LOKAL tanpa memanggil API', () => {
    const { queue, calls } = setup({ known: ['SN-1'] })
    const entry = queue.submit('sn-1', 1, 'GOOD')

    expect(entry?.status).toBe('duplicate')
    expect(entry?.message).toContain('SN-1')
    expect(calls).toHaveLength(0)
    expect(audio.beep).toHaveBeenCalledWith('duplicate')
  })

  it('kode yang sama dua kali sebelum jawaban server tiba: hanya satu permintaan', () => {
    const { queue, calls } = setup()
    queue.submit('SN-9', 1, 'GOOD')
    const second = queue.submit('sn-9', 1, 'GOOD')

    expect(second?.status).toBe('duplicate')
    expect(calls).toHaveLength(1)
  })

  it('mode batch: scan berikutnya tidak menunggu yang sebelumnya; maksimal 3 berjalan bersamaan', async () => {
    const { queue, calls, pending } = setup()
    for (const sn of ['A', 'B', 'C', 'D', 'E']) queue.submit(sn, 1, 'GOOD')

    expect(calls.map((c) => c.code)).toEqual(['A', 'B', 'C']) // D dan E menunggu giliran
    expect(queue.pending.value).toBe(5)

    pending[0]!.resolve(result('A', 1))
    await tick()
    expect(calls.map((c) => c.code)).toEqual(['A', 'B', 'C', 'D']) // satu slot bebas → D berangkat

    pending[1]!.resolve(result('B', 2))
    pending[2]!.resolve(result('C', 3))
    await tick()
    expect(calls.map((c) => c.code)).toEqual(['A', 'B', 'C', 'D', 'E'])
  })

  it('hasil sukses: onSaved dipanggil, entri ok, bunyi sukses; scan ulang kode itu menjadi dobel', async () => {
    const { queue, pending, saved } = setup()
    queue.submit('SN-1', 1, 'GOOD')
    pending[0]!.resolve(result('SN-1', 1, 5))
    await tick()

    expect(saved).toHaveLength(1)
    expect(queue.entries.value[0]!.status).toBe('ok')
    expect(queue.entries.value[0]!.message).toContain('1/5')
    expect(audio.beep).toHaveBeenCalledWith('success')
    expect(queue.submit('SN-1', 1, 'GOOD')?.status).toBe('duplicate')
  })

  it('galat server: pesannya (menyebut kode) tampil, bunyi gagal, kode boleh dicoba lagi', async () => {
    const { queue, pending, calls } = setup()
    queue.submit('SN-X', 1, 'GOOD')
    pending[0]!.reject(new ApiError(409, 'SERIAL_EXISTS', 'Serial number sudah terdaftar'))
    await tick()

    const e = queue.entries.value[0]!
    expect(e.status).toBe('error')
    expect(e.message).toBe('SN-X: Serial number sudah terdaftar')
    expect(audio.beep).toHaveBeenCalledWith('error')
    expect(queue.pending.value).toBe(0)

    queue.submit('SN-X', 1, 'GOOD') // gagal bukan berarti tercatat → boleh diulang
    expect(calls).toHaveLength(2)
  })

  it('galat validasi 422: alasan per field ditampilkan, bukan kalimat umum', async () => {
    const { queue, pending } = setup()
    queue.submit('SN-Z', 1, 'GOOD')
    pending[0]!.reject(
      new ApiError(422, 'VALIDATION_ERROR', 'Data LPB tidak valid', [
        {
          field: 'serial_number',
          message: 'unit dengan kode ini tidak ditemukan untuk produk pada item ini',
        },
      ]),
    )
    await tick()

    expect(queue.entries.value[0]!.message).toBe(
      'SN-Z: Unit dengan kode ini tidak ditemukan untuk produk pada item ini',
    )
  })

  it('kapasitas item habis (termasuk yang masih dikirim): ditolak lokal tanpa API', () => {
    const { queue, calls } = setup({ capacity: 2 })
    queue.submit('A', 1, 'GOOD')
    queue.submit('B', 1, 'GOOD')
    const third = queue.submit('C', 1, 'GOOD')

    expect(third?.status).toBe('error')
    expect(third?.message).toContain('sudah lengkap')
    expect(calls.map((c) => c.code)).toEqual(['A', 'B'])
  })

  it('kegagalan membebaskan kapasitas item untuk scan berikutnya', async () => {
    const { queue, pending, calls } = setup({ capacity: 1 })
    queue.submit('A', 1, 'GOOD')
    pending[0]!.reject(new Error('putus'))
    await tick()

    queue.submit('B', 1, 'GOOD')
    expect(calls.map((c) => c.code)).toEqual(['A', 'B'])
  })

  it('clear() hanya membuang entri yang sudah selesai, bukan yang masih berjalan', async () => {
    const { queue, pending } = setup({ known: ['D1'] })
    queue.submit('D1', 1, 'GOOD') // dobel (selesai)
    queue.submit('LIVE', 1, 'GOOD') // masih berjalan
    queue.clear()

    expect(queue.entries.value.map((e) => e.code)).toEqual(['LIVE'])
    pending[0]!.resolve(result('LIVE'))
    await tick()
  })
})
