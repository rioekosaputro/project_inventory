import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { authService } from '@/services/authService'
import { withRefreshLock } from '@/lib/locks'
import { clearActivity, lastActivity, markActivity } from '@/lib/sessionActivity'
import type { Permission, SessionDto, UserDto } from '@/types/auth'

/** Rute tujuan bila peran belum punya halaman sendiri. */
export const FALLBACK_HOME = '/beranda'

/**
 * State autentikasi. Access token HANYA di memori (tidak pernah ke localStorage/sessionStorage);
 * refresh token ada di cookie HttpOnly yang tidak terbaca JavaScript.
 */
export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref<string | null>(null)
  const user = ref<UserDto | null>(null)
  const permissions = ref<Permission[]>([])
  const defaultHome = ref(FALLBACK_HOME)
  /** true setelah bootstrap() selesai (berhasil atau tidak). */
  const ready = ref(false)
  /** Batas sesi dari backend; 0 = tidak dikirim (tidak dibatasi di klien). */
  const idleTimeoutMs = ref(0)
  const sessionEndsAt = ref(0)
  const lastRefreshAt = ref(0)

  let bootstrapPromise: Promise<void> | null = null
  let refreshPromise: Promise<string> | null = null
  // Naik setiap sesi dibersihkan, agar balasan refresh yang terlambat tidak menghidupkannya lagi.
  let generation = 0

  const permissionSet = computed(() => new Set(permissions.value))
  const isAuthenticated = computed(() => accessToken.value !== null && user.value !== null)
  const mustChangePassword = computed(() => user.value?.must_change_password === true)

  function can(permission: Permission): boolean {
    return permissionSet.value.has(permission)
  }

  function canAny(...list: Permission[]): boolean {
    return list.some((p) => permissionSet.value.has(p))
  }

  /** Alasan sesi harus diakhiri sekarang, atau null bila masih berlaku. */
  function sessionEndReason(now: number): 'idle' | 'expired' | null {
    if (sessionEndsAt.value && now >= sessionEndsAt.value) return 'expired'
    const active = lastActivity()
    if (idleTimeoutMs.value && active && now - active >= idleTimeoutMs.value) return 'idle'
    return null
  }

  function applySession(session: SessionDto): void {
    const now = Date.now()
    idleTimeoutMs.value = (session.idle_timeout_seconds ?? 0) * 1000
    sessionEndsAt.value = session.session_expires_in ? now + session.session_expires_in * 1000 : 0
    lastRefreshAt.value = now
    accessToken.value = session.access_token
    user.value = session.user
    permissions.value = session.permissions
    defaultHome.value = session.default_home || FALLBACK_HOME
  }

  function clear(): void {
    accessToken.value = null
    user.value = null
    permissions.value = []
    defaultHome.value = FALLBACK_HOME
    idleTimeoutMs.value = 0
    sessionEndsAt.value = 0
    lastRefreshAt.value = 0
    generation++
    clearActivity()
  }

  async function login(email: string, password: string): Promise<void> {
    applySession(await authService.login(email, password))
    markActivity()
  }

  /**
   * Memulihkan sesi saat aplikasi dimuat (cookie refresh → access token). Dipanggil guard router
   * dan hanya berjalan SEKALI; gagal (tidak ada cookie / kedaluwarsa) berarti anonim, bukan error.
   */
  function bootstrap(): Promise<void> {
    bootstrapPromise ??= withRefreshLock(() => authService.refresh())
      .then(async (session) => {
        applySession(session)
        // Tab dibuka lagi setelah lama ditinggal: jangan pulihkan sesi yang sudah melewati batas.
        if (sessionEndReason(Date.now())) await logout()
        else markActivity()
      })
      .catch(clear)
      .finally(() => {
        ready.value = true
      })
    return bootstrapPromise
  }

  /** Dipakai antrean refresh axios saat access token kedaluwarsa. Menolak bila gagal. */
  function refreshSession(): Promise<string> {
    const started = generation
    refreshPromise ??= withRefreshLock(() => authService.refresh())
      .then((session) => {
        if (started !== generation) throw new Error('sesi sudah diakhiri')
        applySession(session)
        return session.access_token
      })
      .finally(() => {
        refreshPromise = null
      })
    return refreshPromise
  }

  async function logout(): Promise<void> {
    try {
      await authService.logout()
    } catch {
      // Tetap keluar secara lokal walau server tidak terjangkau.
    } finally {
      clear()
    }
  }

  /** Mengganti password; backend menerbitkan sesi baru (semua sesi lama dicabut). */
  async function changePassword(current: string, next: string): Promise<void> {
    applySession(await authService.changePassword(current, next))
    markActivity()
  }

  return {
    accessToken,
    user,
    permissions,
    defaultHome,
    ready,
    idleTimeoutMs,
    lastRefreshAt,
    sessionEndReason,
    isAuthenticated,
    mustChangePassword,
    can,
    canAny,
    login,
    bootstrap,
    refreshSession,
    logout,
    changePassword,
    clear,
  }
})
