import { computed, ref } from 'vue'
import { beep } from '@/lib/audio'
import { toApiError } from '@/lib/apiError'
import { normalizeCode } from '@/lib/lpb'
import type { AddSerialResult } from '@/types/lpb'

export type ScanStatus = 'queued' | 'sending' | 'ok' | 'error' | 'duplicate'

export interface ScanEntry {
  key: number
  code: string
  itemId: number
  status: ScanStatus
  /** Pesan untuk pengguna (selalu terisi saat selesai). */
  message: string
}

export interface ScanQueueOptions<M = undefined> {
  /** Kirim satu scan ke server (tanpa mengulang; gagal = reject). `meta` = data yang dibawa saat scan diterima. */
  send: (itemId: number, code: string, meta: M) => Promise<AddSerialResult>
  /** Kode (SN/asset tag, sudah dinormalisasi) yang sudah tercatat di dokumen. */
  known: () => ReadonlySet<string>
  /** Sisa slot scan item ini; ≤ 0 = item sudah lengkap. Hanya penjaga cepat — server tetap memeriksa. */
  capacity: (itemId: number) => number
  /** Dipanggil setelah scan tersimpan di server. */
  onSaved: (itemId: number, result: AddSerialResult) => void
  /** Banyak permintaan berjalan bersamaan (plan Bagian 9.3: maks 3). */
  concurrency?: number
  /** Berapa hasil terakhir yang disimpan untuk ditampilkan. */
  history?: number
}

/**
 * Antrean scan untuk scanner POS (plan Bagian 9.3):
 *  - kode dinormalisasi (pangkas + huruf besar), scan dobel dikenali SECARA LOKAL tanpa memanggil API
 *    (sudah tercatat, atau sedang diantre/dikirim);
 *  - scan berikutnya tidak menunggu validasi sebelumnya (mode batch), dengan paling banyak
 *    `concurrency` (3) permintaan berjalan bersamaan;
 *  - setiap hasil memberi umpan balik suara dan pesan yang menyebut kodenya.
 */
export function useScanQueue<M = undefined>(opts: ScanQueueOptions<M>) {
  const limit = opts.concurrency ?? 3
  const keep = opts.history ?? 8

  const entries = ref<ScanEntry[]>([])
  let seq = 0
  let running = 0
  /** Kode yang sedang diantre/dikirim, agar scan kembar tertangkap sebelum jawaban server datang. */
  const inflight = new Set<string>()
  /** Jumlah scan yang sedang diantre/dikirim per item (dihitung terhadap sisa kapasitas item). */
  const inflightByItem = new Map<number, number>()
  const waiting: { entry: ScanEntry; meta: M }[] = []

  const pending = computed(
    () => entries.value.filter((e) => e.status === 'queued' || e.status === 'sending').length,
  )

  function remember(entry: ScanEntry): ScanEntry {
    entries.value = [entry, ...entries.value].slice(0, keep)
    return entry
  }

  function finish(entry: ScanEntry, status: ScanStatus, message: string): void {
    entries.value = entries.value.map((e) => (e.key === entry.key ? { ...e, status, message } : e))
  }

  async function run(entry: ScanEntry, meta: M): Promise<void> {
    running++
    finish(entry, 'sending', 'Memeriksa…')
    try {
      const result = await opts.send(entry.itemId, entry.code, meta)
      opts.onSaved(entry.itemId, result)
      finish(entry, 'ok', `${entry.code} ditambahkan (${result.scanned_count}/${result.qty})`)
      beep('success')
    } catch (e) {
      finish(entry, 'error', `${entry.code}: ${toApiError(e).message}`)
      beep('error')
    } finally {
      inflight.delete(entry.code)
      inflightByItem.set(entry.itemId, (inflightByItem.get(entry.itemId) ?? 1) - 1)
      running--
      pump()
    }
  }

  function pump(): void {
    while (running < limit && waiting.length > 0) {
      const next = waiting.shift()!
      void run(next.entry, next.meta)
    }
  }

  /** Terima satu kode hasil scan/ketikan. Mengembalikan entri hasil (atau null bila kosong). */
  function submit(raw: string, itemId: number, meta: M): ScanEntry | null {
    const code = normalizeCode(raw)
    if (code === '') return null

    const base = { key: ++seq, code, itemId }
    if (opts.known().has(code) || inflight.has(code)) {
      beep('duplicate')
      return remember({ ...base, status: 'duplicate', message: `${code} sudah di-scan` })
    }
    if (opts.capacity(itemId) - (inflightByItem.get(itemId) ?? 0) <= 0) {
      beep('error')
      return remember({
        ...base,
        status: 'error',
        message: `${code}: jumlah scan untuk item ini sudah lengkap`,
      })
    }

    inflight.add(code)
    inflightByItem.set(itemId, (inflightByItem.get(itemId) ?? 0) + 1)
    const entry = remember({ ...base, status: 'queued', message: 'Menunggu giliran…' })
    waiting.push({ entry, meta })
    pump()
    return entry
  }

  function clear(): void {
    entries.value = entries.value.filter((e) => e.status === 'queued' || e.status === 'sending')
  }

  return { entries, pending, submit, clear }
}
