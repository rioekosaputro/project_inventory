import { ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import { toApiError } from '@/lib/apiError'

interface Toggleable {
  id: number
  is_active: boolean
}

export interface ActiveToggleOptions<T extends Toggleable> {
  update: (id: number, body: { is_active: boolean }) => Promise<unknown>
  /** Nama entitas yang disebut pada pesan, mis. "gudang 'WH-A'". */
  describe: (row: T) => string
  /** Dipanggil setelah berhasil (muat ulang daftar). */
  onDone: () => void | Promise<void>
}

/**
 * Aktifkan/nonaktifkan data master dari baris tabel. Menonaktifkan perlu konfirmasi yang
 * menyebut entitasnya (dialog); mengaktifkan kembali langsung (mudah dibatalkan).
 */
export function useActiveToggle<T extends Toggleable>(opts: ActiveToggleOptions<T>) {
  const toast = useToast()
  const pending = ref<T | null>(null)
  const busy = ref(false)

  async function apply(row: T, active: boolean): Promise<void> {
    busy.value = true
    try {
      await opts.update(row.id, { is_active: active })
      toast.add({
        severity: 'success',
        summary: `${capitalize(opts.describe(row))} ${active ? 'diaktifkan' : 'dinonaktifkan'}`,
        life: 4000,
      })
      await opts.onDone()
    } catch (e) {
      // Pesan server menjelaskan penyebab (mis. rak masih menyimpan stok) dan jalan keluarnya.
      toast.add({
        severity: 'error',
        summary: 'Perubahan gagal',
        detail: toApiError(e).message,
        life: 8000,
      })
    } finally {
      busy.value = false
      pending.value = null
    }
  }

  function request(row: T): void {
    if (row.is_active) pending.value = row
    else void apply(row, true)
  }

  return {
    pending,
    busy,
    request,
    confirm: () => (pending.value ? apply(pending.value, false) : Promise.resolve()),
    cancel: () => {
      pending.value = null
    },
  }
}

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}
