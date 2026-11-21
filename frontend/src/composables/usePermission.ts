import { useAuthStore } from '@/stores/auth'

/**
 * Cek permission untuk menyembunyikan tombol/menu. Hanya kenyamanan UI —
 * otorisasi sebenarnya tetap di backend.
 */
export function usePermission() {
  const auth = useAuthStore()
  return { can: auth.can, canAny: auth.canAny }
}
