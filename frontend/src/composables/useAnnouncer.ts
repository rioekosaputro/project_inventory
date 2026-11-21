import { ref } from 'vue'

/**
 * Satu live region (polite) untuk seluruh halaman, dirender LiveRegion.vue di App.vue.
 * Error validasi diumumkan di sini (bukan tiap pesan error menjadi live region sendiri, yang
 * tidak andal saat elemen hanya "ditampilkan"). Jeda 500 ms agar tidak menimpa pembacaan
 * label field yang baru menerima fokus.
 */
export const announcement = ref('')

const ANNOUNCE_DELAY_MS = 500
let timer: ReturnType<typeof setTimeout> | undefined

export function announce(message: string, delayMs: number = ANNOUNCE_DELAY_MS): void {
  clearTimeout(timer)
  announcement.value = ''
  timer = setTimeout(() => {
    announcement.value = message
  }, delayMs)
}

export function clearAnnouncement(): void {
  clearTimeout(timer)
  announcement.value = ''
}
