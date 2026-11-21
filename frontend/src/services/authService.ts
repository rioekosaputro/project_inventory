import { api } from '@/lib/axios'
import type { ProfileDto, SessionDto } from '@/types/auth'

/** Wrapper API autentikasi (plan Bagian 8.1). View/store tidak memanggil axios langsung. */
export const authService = {
  login: (email: string, password: string) =>
    api.post<SessionDto>('/auth/login', { email, password }),

  /** Menukar cookie refresh (HttpOnly) → access token baru + data user. */
  refresh: () => api.post<SessionDto>('/auth/refresh'),

  logout: () => api.post<null>('/auth/logout'),

  me: () => api.get<ProfileDto>('/auth/me'),

  changePassword: (currentPassword: string, newPassword: string) =>
    api.post<SessionDto>('/auth/change-password', {
      current_password: currentPassword,
      new_password: newPassword,
    }),
}
