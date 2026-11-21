import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { authService } from '@/services/authService'
import { withRefreshLock } from '@/lib/locks'
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

  let bootstrapPromise: Promise<void> | null = null

  const permissionSet = computed(() => new Set(permissions.value))
  const isAuthenticated = computed(() => accessToken.value !== null && user.value !== null)
  const mustChangePassword = computed(() => user.value?.must_change_password === true)

  function can(permission: Permission): boolean {
    return permissionSet.value.has(permission)
  }

  function canAny(...list: Permission[]): boolean {
    return list.some((p) => permissionSet.value.has(p))
  }

  function applySession(session: SessionDto): void {
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
  }

  async function login(email: string, password: string): Promise<void> {
    applySession(await authService.login(email, password))
  }

  /**
   * Memulihkan sesi saat aplikasi dimuat (cookie refresh → access token). Dipanggil guard router
   * dan hanya berjalan SEKALI; gagal (tidak ada cookie / kedaluwarsa) berarti anonim, bukan error.
   */
  function bootstrap(): Promise<void> {
    bootstrapPromise ??= withRefreshLock(() => authService.refresh())
      .then(applySession)
      .catch(clear)
      .finally(() => {
        ready.value = true
      })
    return bootstrapPromise
  }

  /** Dipakai antrean refresh axios saat access token kedaluwarsa. Menolak bila gagal. */
  async function refreshSession(): Promise<string> {
    const session = await withRefreshLock(() => authService.refresh())
    applySession(session)
    return session.access_token
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
  }

  return {
    accessToken,
    user,
    permissions,
    defaultHome,
    ready,
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
