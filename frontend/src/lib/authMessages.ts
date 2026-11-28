import type { ApiError } from '@/lib/apiError'

/** Pesan untuk 429 (rate limit): kapan boleh mencoba lagi. */
export function rateLimitMessage(err: ApiError): string {
  const minutes = Math.max(1, Math.ceil((err.retryAfterSeconds ?? 900) / 60))
  return `Terlalu banyak percobaan. Coba lagi dalam ${minutes} menit.`
}

/** Penjelasan mengapa pengguna dikembalikan ke halaman masuk (query `?reason=`). */
export function sessionEndMessage(reason: unknown): string {
  if (reason === 'idle')
    return 'Anda dikeluarkan otomatis karena tidak ada aktivitas. Silakan masuk lagi.'
  if (reason === 'expired') return 'Sesi Anda sudah berakhir. Silakan masuk lagi.'
  return ''
}

/** Pesan di atas form login; tidak membedakan "email tidak ada" dari "password salah". */
export function loginErrorMessage(err: ApiError): string {
  switch (err.code) {
    case 'INVALID_CREDENTIALS':
      return 'Email atau password salah. Periksa kembali, atau hubungi administrator untuk mereset password.'
    case 'TOO_MANY_REQUESTS':
      return rateLimitMessage(err)
    default:
      return err.message
  }
}
