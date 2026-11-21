import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { api, http, idempotencyHeaders, setAuthBridge, type AuthBridge } from '@/lib/axios'
import { ApiError } from '@/lib/apiError'
import { fail, fakeResponse, ok } from '@/test/helpers'

const originalAdapter = http.defaults.adapter

/** Snapshot saat request dikirim (objek config yang sama dimutasi saat request diulang). */
interface Call {
  url: string | undefined
  auth: string | undefined
  idempotencyKey: string | undefined
}

let calls: Call[]

const authOf = (c: InternalAxiosRequestConfig) =>
  c.headers.get('Authorization') as string | undefined

function useServer(
  handler: (config: InternalAxiosRequestConfig) => ReturnType<typeof fakeResponse>,
) {
  http.defaults.adapter = async (config) => {
    calls.push({
      url: config.url,
      auth: authOf(config),
      idempotencyKey: config.headers.get('Idempotency-Key') as string | undefined,
    })
    return handler(config)
  }
}

function bridge(overrides: Partial<AuthBridge> = {}) {
  const state = { token: 'old-token' }
  const b = {
    state,
    getToken: vi.fn<AuthBridge['getToken']>(() => state.token),
    refresh: vi.fn<AuthBridge['refresh']>(async () => {
      state.token = 'new-token'
      return 'new-token'
    }),
    onAuthFailure: vi.fn<AuthBridge['onAuthFailure']>(),
    ...overrides,
  }
  setAuthBridge(b)
  return b
}
beforeEach(() => {
  calls = []
})

afterEach(() => {
  setAuthBridge(null)
  http.defaults.adapter = originalAdapter
})

describe('Authorization', () => {
  it('menempelkan access token dari memori', async () => {
    bridge()
    useServer((c) => fakeResponse(c, 200, ok({ hello: 'dunia' })))

    const data = await api.get<{ hello: string }>('/ping')

    expect(data).toEqual({ hello: 'dunia' })
    expect(calls[0]?.auth).toBe('Bearer old-token')
  })

  it('tanpa token tidak mengirim header Authorization', async () => {
    bridge({ getToken: () => null })
    useServer((c) => fakeResponse(c, 200, ok(null)))

    await api.get('/ping')

    expect(calls[0]?.auth).toBeUndefined()
  })
})

describe('antrean refresh pada 401', () => {
  it('me-refresh sekali lalu mengulang request dengan token baru', async () => {
    const b = bridge()
    useServer((c) =>
      authOf(c) === 'Bearer new-token'
        ? fakeResponse(c, 200, ok({ id: 7 }))
        : fakeResponse(c, 401, fail('INVALID_TOKEN', 'Sesi berakhir')),
    )

    const data = await api.get<{ id: number }>('/users/7')

    expect(data).toEqual({ id: 7 })
    expect(b.refresh).toHaveBeenCalledTimes(1)
    expect(calls.map((c) => c.auth)).toEqual(['Bearer old-token', 'Bearer new-token'])
    expect(b.onAuthFailure).not.toHaveBeenCalled()
  })

  it('banyak request 401 bersamaan hanya memicu SATU refresh (refresh token dirotasi!)', async () => {
    const b = bridge()
    useServer((c) =>
      authOf(c) === 'Bearer new-token'
        ? fakeResponse(c, 200, ok({ path: c.url }))
        : fakeResponse(c, 401, fail('INVALID_TOKEN', 'Sesi berakhir')),
    )

    const results = await Promise.all([api.get('/a'), api.get('/b'), api.get('/c')])

    expect(b.refresh).toHaveBeenCalledTimes(1)
    expect(results).toHaveLength(3)
    expect(calls.filter((c) => c.auth === 'Bearer old-token')).toHaveLength(3)
    expect(calls.filter((c) => c.auth === 'Bearer new-token')).toHaveLength(3)
  })

  it('refresh berikutnya (setelah yang pertama selesai) berjalan normal', async () => {
    const b = bridge()
    useServer((c) =>
      authOf(c) === 'Bearer new-token'
        ? fakeResponse(c, 200, ok(null))
        : fakeResponse(c, 401, fail('INVALID_TOKEN', 'x')),
    )
    await api.get('/a')
    expect(b.refresh).toHaveBeenCalledTimes(1)

    b.state.token = 'old-token' // token kedaluwarsa lagi
    await api.get('/b')
    expect(b.refresh).toHaveBeenCalledTimes(2)
  })

  it('refresh gagal → onAuthFailure dipanggil dan pemanggil menerima ApiError 401', async () => {
    const b = bridge({
      refresh: vi.fn<AuthBridge['refresh']>(async () => Promise.reject(new Error('cookie habis'))),
    })
    useServer((c) => fakeResponse(c, 401, fail('INVALID_TOKEN', 'Sesi berakhir')))

    const [r1, r2] = await Promise.allSettled([api.get('/a'), api.get('/b')])

    expect(b.refresh).toHaveBeenCalledTimes(1)
    expect(b.onAuthFailure).toHaveBeenCalled()
    for (const r of [r1, r2]) {
      expect(r.status).toBe('rejected')
      const reason = (r as PromiseRejectedResult).reason as ApiError
      expect(reason).toBeInstanceOf(ApiError)
      expect(reason.status).toBe(401)
    }
  })

  it('tidak berputar: 401 lagi setelah refresh tidak memicu refresh kedua', async () => {
    const b = bridge()
    useServer((c) => fakeResponse(c, 401, fail('UNAUTHORIZED', 'Autentikasi diperlukan')))

    await expect(api.get('/a')).rejects.toBeInstanceOf(ApiError)

    expect(b.refresh).toHaveBeenCalledTimes(1)
    expect(calls).toHaveLength(2)
  })

  it('Idempotency-Key yang sama ikut terkirim saat request diulang', async () => {
    bridge()
    useServer((c) =>
      authOf(c) === 'Bearer new-token'
        ? fakeResponse(c, 200, ok(null))
        : fakeResponse(c, 401, fail('X', 'x')),
    )

    await api.post('/lpb/1/approve', undefined, { headers: idempotencyHeaders('key-123') })

    expect(calls.map((c) => c.idempotencyKey)).toEqual(['key-123', 'key-123'])
  })

  it('401 dari login/refresh/logout TIDAK memicu refresh (kredensial salah ≠ sesi habis)', async () => {
    const b = bridge()
    useServer((c) => fakeResponse(c, 401, fail('INVALID_CREDENTIALS', 'Email atau password salah')))

    for (const path of ['/auth/login', '/auth/refresh', '/auth/logout']) {
      await expect(api.post(path)).rejects.toMatchObject({ code: 'INVALID_CREDENTIALS' })
    }

    expect(b.refresh).not.toHaveBeenCalled()
    expect(b.onAuthFailure).not.toHaveBeenCalled()
  })

  it('error non-401 (403/500) tidak memicu refresh', async () => {
    const b = bridge()
    useServer((c) => fakeResponse(c, 403, fail('FORBIDDEN', 'Tidak punya akses')))

    await expect(api.get('/users')).rejects.toMatchObject({ status: 403, code: 'FORBIDDEN' })
    expect(b.refresh).not.toHaveBeenCalled()
  })

  it('kegagalan request ulang (mis. 403) dilaporkan apa adanya, bukan dianggap sesi habis', async () => {
    const b = bridge()
    useServer((c) =>
      authOf(c) === 'Bearer new-token'
        ? fakeResponse(c, 403, fail('FORBIDDEN', 'Tidak punya akses'))
        : fakeResponse(c, 401, fail('INVALID_TOKEN', 'x')),
    )

    await expect(api.get('/users')).rejects.toMatchObject({ status: 403 })
    expect(b.onAuthFailure).not.toHaveBeenCalled()
  })
})

describe('penanganan error', () => {
  it('semua error menjadi ApiError berbahasa Indonesia', async () => {
    bridge()
    useServer(() => Promise.reject(new AxiosError('Network Error', 'ERR_NETWORK')))

    await expect(api.get('/x')).rejects.toMatchObject({ code: 'NETWORK_ERROR', status: 0 })
  })

  it('tanpa bridge, 401 langsung ditolak sebagai ApiError', async () => {
    useServer((c) => fakeResponse(c, 401, fail('UNAUTHORIZED', 'Autentikasi diperlukan')))
    await expect(api.get('/x')).rejects.toBeInstanceOf(ApiError)
  })
})

describe('idempotencyHeaders', () => {
  it('membuat UUID unik per pemanggilan bila key tidak diberikan', () => {
    const a = idempotencyHeaders()['Idempotency-Key']
    const b = idempotencyHeaders()['Idempotency-Key']
    expect(a).toMatch(/^[0-9a-f-]{36}$/)
    expect(a).not.toBe(b)
  })
})
