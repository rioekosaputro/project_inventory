import { vi } from 'vitest'
import type { authService as realAuthService } from '@/services/authService'

type Service = typeof realAuthService

/**
 * Pengganti authService untuk test (bertipe mengikuti aslinya):
 *   vi.mock('@/services/authService', () => import('@/test/mockAuthService'))
 */
export const authService = {
  login: vi.fn<Service['login']>(),
  refresh: vi.fn<Service['refresh']>(),
  logout: vi.fn<Service['logout']>(),
  me: vi.fn<Service['me']>(),
  changePassword: vi.fn<Service['changePassword']>(),
}
