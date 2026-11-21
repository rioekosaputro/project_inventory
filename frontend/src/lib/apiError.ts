import { isAxiosError } from 'axios'
import type { ApiErrorBody, FieldError } from '@/types/api'

/** Kode untuk kegagalan yang tidak berasal dari backend. */
export const CLIENT_CODES = {
  network: 'NETWORK_ERROR',
  timeout: 'TIMEOUT',
  unknown: 'UNKNOWN_ERROR',
} as const

/**
 * Error API yang sudah dinormalisasi. Pesan selalu Bahasa Indonesia, tidak menyalahkan
 * pengguna, dan memberi arah pemulihan. `status` 0 = tidak ada respons dari server.
 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly fieldErrors: FieldError[] = [],
    readonly requestId?: string,
    readonly retryAfterSeconds?: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }

  get isNetworkError(): boolean {
    return this.status === 0
  }
}

function genericMessage(status: number): string {
  if (status === 502 || status === 503 || status === 504) {
    return 'Server sedang tidak dapat dijangkau. Coba lagi sebentar lagi.'
  }
  if (status >= 500)
    return 'Terjadi gangguan di server. Coba lagi, atau hubungi administrator jika berulang.'
  if (status === 401) return 'Sesi Anda berakhir. Silakan masuk kembali.'
  if (status === 403) return 'Anda tidak memiliki akses untuk aksi ini.'
  if (status === 404) return 'Data yang dicari tidak ditemukan.'
  return 'Permintaan tidak dapat diproses. Periksa data Anda lalu coba lagi.'
}

function isErrorBody(data: unknown): data is ApiErrorBody {
  return (
    typeof data === 'object' &&
    data !== null &&
    (data as { success?: unknown }).success === false &&
    typeof (data as { message?: unknown }).message === 'string'
  )
}

/** Mengubah error apa pun (AxiosError, ApiError, lainnya) menjadi ApiError. */
export function toApiError(err: unknown): ApiError {
  if (err instanceof ApiError) return err

  if (isAxiosError(err)) {
    if (!err.response) {
      if (err.code === 'ECONNABORTED' || err.code === 'ETIMEDOUT') {
        return new ApiError(0, CLIENT_CODES.timeout, 'Server terlalu lama merespons. Coba lagi.')
      }
      return new ApiError(
        0,
        CLIENT_CODES.network,
        'Tidak dapat terhubung ke server. Periksa koneksi internet Anda lalu coba lagi.',
      )
    }

    const { status, data, headers } = err.response
    const retryHeader = Number(headers?.['retry-after'])
    const retryAfter = Number.isFinite(retryHeader) && retryHeader > 0 ? retryHeader : undefined

    if (isErrorBody(data)) {
      return new ApiError(
        status,
        data.code,
        data.message,
        data.errors ?? [],
        data.request_id,
        retryAfter,
      )
    }
    return new ApiError(status, `HTTP_${status}`, genericMessage(status), [], undefined, retryAfter)
  }

  return new ApiError(
    0,
    CLIENT_CODES.unknown,
    'Terjadi kesalahan tak terduga. Muat ulang halaman lalu coba lagi.',
  )
}
