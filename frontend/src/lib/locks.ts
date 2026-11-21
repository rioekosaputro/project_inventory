const REFRESH_LOCK = 'erp-auth-refresh'

/**
 * Menyerialkan refresh token ANTAR TAB. Refresh token dirotasi dan pemakaian ulang token lama
 * dianggap pencurian (seluruh sesi dicabut). Tanpa kunci, dua tab yang me-refresh bersamaan
 * dengan cookie yang sama akan memicu itu. Dengan kunci, tab kedua menunggu lalu memakai
 * cookie baru yang sudah ditulis tab pertama.
 */
export function withRefreshLock<T>(task: () => Promise<T>): Promise<T> {
  if (typeof navigator !== 'undefined' && navigator.locks?.request) {
    return navigator.locks.request(REFRESH_LOCK, task)
  }
  return task()
}
