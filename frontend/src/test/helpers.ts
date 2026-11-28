import { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import type { Role, SessionDto, UserDto } from '@/types/auth'

export function makeUser(over: Partial<UserDto> = {}): UserDto {
  return {
    id: 1,
    name: 'Budi Santoso',
    email: 'budi@contoh.id',
    phone: null,
    role: 'STAFF_WAREHOUSE',
    warehouse_id: null,
    must_change_password: false,
    ...over,
  }
}

export function makeSession(
  over: {
    role?: Role
    permissions?: string[]
    default_home?: string
    user?: Partial<UserDto>
    token?: string
  } = {},
): SessionDto {
  return {
    access_token: over.token ?? 'access-token-1',
    token_type: 'Bearer',
    expires_in: 900,
    session_expires_in: 12 * 3600,
    idle_timeout_seconds: 30 * 60,
    user: makeUser({ ...(over.role ? { role: over.role } : {}), ...over.user }),
    permissions: over.permissions ?? ['lpb.read'],
    default_home: over.default_home ?? '/dashboard',
  }
}

/** Respons adapter axios palsu: 2xx → resolve, selain itu reject AxiosError (seperti axios asli). */
export function fakeResponse(
  config: InternalAxiosRequestConfig,
  status: number,
  data: unknown,
  headers: Record<string, string> = {},
): Promise<AxiosResponse> {
  const response: AxiosResponse = {
    data,
    status,
    statusText: String(status),
    headers,
    config,
    request: {},
  }
  if (status >= 200 && status < 300) return Promise.resolve(response)
  return Promise.reject(new AxiosError(`HTTP ${status}`, 'ERR_BAD_REQUEST', config, {}, response))
}

export const ok = <T>(data: T) => ({ success: true, message: 'Berhasil', data })

export const fail = (
  code: string,
  message: string,
  errors?: { field: string; message: string }[],
) => ({
  success: false,
  message,
  code,
  ...(errors ? { errors } : {}),
  request_id: 'req-test-0001',
})
