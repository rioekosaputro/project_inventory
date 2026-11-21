import type { Router } from 'vue-router'
import { setAuthBridge } from '@/lib/axios'
import type { useAuthStore } from '@/stores/auth'

/**
 * Menyambungkan klien HTTP ke store auth & router:
 *  - token Authorization diambil dari memori store,
 *  - 401 → refresh (satu kali, antre) → ulangi request,
 *  - refresh gagal → bersihkan sesi dan arahkan ke /login (menyimpan halaman tujuan).
 */
export function installAuthBridge(router: Router, auth: ReturnType<typeof useAuthStore>): void {
  setAuthBridge({
    getToken: () => auth.accessToken,
    refresh: () => auth.refreshSession(),
    onAuthFailure: () => {
      auth.clear()
      const current = router.currentRoute.value
      if (current.meta.public) return
      void router.replace({
        name: 'login',
        query: current.fullPath === '/' ? undefined : { redirect: current.fullPath },
      })
    },
  })
}
