import { onMounted, ref } from 'vue'
import type { Page } from '@/services/masterService'

export interface Option {
  value: string
  label: string
}

/**
 * Pilihan untuk select/filter yang berasal dari daftar master lain (gudang, kategori).
 * Mengambil satu halaman terbesar (100); gagal memuat = pilihan kosong (form tetap dapat dipakai
 * untuk field lain, dan server tetap memvalidasi).
 */
export function useOptions<T>(fetch: () => Promise<Page<T>>, map: (item: T) => Option) {
  const options = ref<Option[]>([])
  const loading = ref(false)

  async function load(): Promise<void> {
    loading.value = true
    try {
      options.value = (await fetch()).items.map(map)
    } catch {
      options.value = []
    } finally {
      loading.value = false
    }
  }
  onMounted(load)

  return { options, loading, reload: load }
}
