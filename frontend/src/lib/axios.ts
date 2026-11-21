import axios, {
  isAxiosError,
  type AxiosRequestConfig,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import { toApiError } from '@/lib/apiError'
import type { ApiEnvelope } from '@/types/api'

/**
 * Jembatan ke store auth. Disuntik saat startup (main.ts) agar modul ini tidak mengimpor store
 * (menghindari dependensi melingkar) dan mudah diuji.
 */
export interface AuthBridge {
  /** Access token saat ini (di memori), atau null. */
  getToken(): string | null
  /** Menukar refresh token (cookie) → access token baru. Menolak (reject) bila gagal. */
  refresh(): Promise<string>
  /** Dipanggil bila sesi tidak bisa dipulihkan: bersihkan state & arahkan ke halaman login. */
  onAuthFailure(): void
}

let bridge: AuthBridge | null = null

export function setAuthBridge(next: AuthBridge | null): void {
  bridge = next
}

export const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

export const http = axios.create({
  baseURL: API_BASE_URL,
  withCredentials: true,
  timeout: 30_000,
  headers: { Accept: 'application/json' },
})

type RetriableConfig = InternalAxiosRequestConfig & { _retried?: boolean }

/** Endpoint auth yang 401-nya bermakna sendiri (kredensial salah / sesi habis) — jangan di-refresh. */
const NO_REFRESH_PATHS = ['/auth/login', '/auth/refresh', '/auth/logout']

function shouldAttemptRefresh(config: RetriableConfig | undefined): config is RetriableConfig {
  if (!config || config._retried) return false
  const url = config.url ?? ''
  return !NO_REFRESH_PATHS.some((p) => url.endsWith(p))
}

// Satu refresh untuk semua request yang 401 bersamaan (antrean refresh, plan 9.3).
let inflightRefresh: Promise<string> | null = null

function refreshOnce(): Promise<string> {
  if (!bridge) return Promise.reject(new Error('auth bridge belum dipasang'))
  inflightRefresh ??= bridge.refresh().finally(() => {
    inflightRefresh = null
  })
  return inflightRefresh
}

http.interceptors.request.use((config) => {
  const token = bridge?.getToken()
  if (token && !config.headers.has('Authorization')) {
    config.headers.set('Authorization', `Bearer ${token}`)
  }
  return config
})

http.interceptors.response.use(
  (response) => response,
  async (error: unknown) => {
    if (isAxiosError(error) && error.response?.status === 401 && bridge) {
      const original = error.config as RetriableConfig | undefined
      if (shouldAttemptRefresh(original)) {
        original._retried = true

        let token: string
        try {
          token = await refreshOnce()
        } catch {
          // Refresh gagal = sesi tidak bisa dipulihkan. Pemanggil tetap menerima 401 aslinya.
          bridge.onAuthFailure()
          throw toApiError(error)
        }

        original.headers.set('Authorization', `Bearer ${token}`)
        // Idempotency-Key (bila ada) ikut terbawa → aman diulang. Kegagalan pada pengulangan
        // sudah berupa ApiError (interceptor yang sama) dan tidak memicu refresh lagi (_retried).
        return http(original)
      }
    }
    throw toApiError(error)
  },
)

/** Header Idempotency-Key untuk aksi kritis (approve LPB, POD, pembayaran). */
export function idempotencyHeaders(key: string = crypto.randomUUID()): Record<string, string> {
  return { 'Idempotency-Key': key }
}

async function unwrap<T>(request: Promise<AxiosResponse<ApiEnvelope<T>>>): Promise<T> {
  return (await request).data.data
}

/** Akses API bertipe: mengembalikan `data` dari envelope; error selalu ApiError. */
export const api = {
  get: <T>(url: string, config?: AxiosRequestConfig) => unwrap<T>(http.get(url, config)),
  post: <T>(url: string, body?: unknown, config?: AxiosRequestConfig) =>
    unwrap<T>(http.post(url, body, config)),
  patch: <T>(url: string, body?: unknown, config?: AxiosRequestConfig) =>
    unwrap<T>(http.patch(url, body, config)),
  delete: <T>(url: string, config?: AxiosRequestConfig) => unwrap<T>(http.delete(url, config)),
}
