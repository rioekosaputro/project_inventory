import type { Router } from 'vue-router'
import { toApiError } from '@/lib/apiError'
import { lastActivity, markActivity } from '@/lib/sessionActivity'
import type { useAuthStore } from '@/stores/auth'

export type SessionEndReason = 'idle' | 'expired'

const CHECK_EVERY_MS = 15_000
const WRITE_THROTTLE_MS = 5_000
/** Selama pengguna aktif, refresh token dirotasi paling lama setiap sekian ms. */
const MAX_KEEPALIVE_MS = 5 * 60_000

const ACTIVITY_EVENTS = ['pointerdown', 'pointermove', 'keydown', 'wheel', 'scroll', 'touchstart']

/**
 * Batas sesi di sisi klien (aturan sebenarnya ditegakkan backend):
 *  - tidak ada aktivitas selama batas idle → keluar otomatis,
 *  - batas maksimum sejak login tercapai → keluar,
 *  - selama aktif (mis. mengisi form panjang tanpa request) sesi di server diperpanjang.
 * Mengembalikan fungsi untuk melepas semua listener.
 */
export function installSessionTimeout(
  router: Router,
  auth: ReturnType<typeof useAuthStore>,
): () => void {
  let lastWrite = 0
  let ending = false

  async function end(reason: SessionEndReason): Promise<void> {
    if (ending) return
    ending = true
    const current = router.currentRoute.value
    try {
      await auth.logout()
      if (!current.meta.public) {
        await router.replace({
          name: 'login',
          query: { reason, ...(current.fullPath === '/' ? {} : { redirect: current.fullPath }) },
        })
      }
    } finally {
      ending = false
    }
  }

  /** true bila sesi sudah (atau sedang) diakhiri. */
  function check(): boolean {
    if (!auth.isAuthenticated) return false
    const reason = auth.sessionEndReason(Date.now())
    if (!reason) return ending
    void end(reason)
    return true
  }

  function keepAlive(now: number): void {
    if (!auth.idleTimeoutMs) return
    const every = Math.min(MAX_KEEPALIVE_MS, auth.idleTimeoutMs / 3)
    if (now - auth.lastRefreshAt < every) return
    auth.refreshSession().catch((e: unknown) => {
      // Gangguan jaringan tidak mengeluarkan pengguna; hanya penolakan dari server.
      if (toApiError(e).status === 401) void end('expired')
    })
  }

  function onActivity(): void {
    // Cek DULU: gerakan pertama setelah perangkat tidur lama tidak boleh menghidupkan sesi lagi.
    if (!auth.isAuthenticated || check()) return
    const now = Date.now()
    if (now - lastWrite >= WRITE_THROTTLE_MS) {
      lastWrite = now
      markActivity(now)
    } else if (!lastActivity()) {
      markActivity(now)
    }
    keepAlive(now)
  }

  const onVisible = (): void => {
    if (document.visibilityState === 'visible') check()
  }

  const options = { capture: true, passive: true }
  for (const name of ACTIVITY_EVENTS) window.addEventListener(name, onActivity, options)
  document.addEventListener('visibilitychange', onVisible)
  const timer = window.setInterval(check, CHECK_EVERY_MS)

  return () => {
    for (const name of ACTIVITY_EVENTS) window.removeEventListener(name, onActivity, options)
    document.removeEventListener('visibilitychange', onVisible)
    window.clearInterval(timer)
  }
}
