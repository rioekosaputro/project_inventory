/**
 * Waktu aktivitas terakhir pengguna, dibagi antar tab. HANYA cap waktu — token tidak pernah
 * disimpan di sini.
 */
const KEY = 'erp.last-activity'

let memory = 0

export function markActivity(at: number = Date.now()): void {
  memory = at
  try {
    window.localStorage.setItem(KEY, String(at))
  } catch {
    // localStorage tidak tersedia: cukup nilai di memori tab ini.
  }
}

/** 0 bila belum pernah tercatat. */
export function lastActivity(): number {
  try {
    const stored = Number(window.localStorage.getItem(KEY))
    if (Number.isFinite(stored) && stored > 0) return stored
  } catch {
    // abaikan
  }
  return memory
}

export function clearActivity(): void {
  memory = 0
  try {
    window.localStorage.removeItem(KEY)
  } catch {
    // abaikan
  }
}
