import { describe, expect, it } from 'vitest'
import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { ApiError, CLIENT_CODES, toApiError, userMessage } from '@/lib/apiError'
import { fail } from '@/test/helpers'

const config = { headers: {} } as InternalAxiosRequestConfig

function axiosFailure(status: number, data: unknown, headers: Record<string, string> = {}) {
  return new AxiosError(
    'x',
    'ERR_BAD_REQUEST',
    config,
    {},
    {
      status,
      statusText: '',
      data,
      headers,
      config,
    },
  )
}

describe('toApiError', () => {
  it('memakai envelope error backend (kode, pesan, detail field, request id)', () => {
    const err = toApiError(
      axiosFailure(
        422,
        fail('INVALID_DATA', 'Data user tidak valid', [{ field: 'role', message: 'tidak valid' }]),
      ),
    )

    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(422)
    expect(err.code).toBe('INVALID_DATA')
    expect(err.message).toBe('Data user tidak valid')
    expect(err.fieldErrors).toEqual([{ field: 'role', message: 'tidak valid' }])
    expect(err.requestId).toBe('req-test-0001')
  })

  it('membaca header Retry-After', () => {
    const err = toApiError(
      axiosFailure(429, fail('TOO_MANY_REQUESTS', 'Terlalu banyak'), { 'retry-after': '120' }),
    )
    expect(err.retryAfterSeconds).toBe(120)
  })

  it('respons non-JSON (mis. 502 dari proxy) → pesan Indonesia, bukan HTML mentah', () => {
    const err = toApiError(axiosFailure(502, '<html>Bad gateway</html>'))
    expect(err.code).toBe('HTTP_502')
    expect(err.message).toContain('tidak dapat dijangkau')
    expect(err.message).not.toContain('<html>')
  })

  it('tanpa respons → kesalahan jaringan dengan arahan pemulihan', () => {
    const err = toApiError(new AxiosError('Network Error', 'ERR_NETWORK'))
    expect(err.isNetworkError).toBe(true)
    expect(err.code).toBe(CLIENT_CODES.network)
    expect(err.message).toContain('Periksa koneksi')
  })

  it('timeout dibedakan dari jaringan putus', () => {
    expect(toApiError(new AxiosError('t', 'ECONNABORTED')).code).toBe(CLIENT_CODES.timeout)
  })

  it('ApiError dikembalikan apa adanya; error asing → UNKNOWN_ERROR', () => {
    const original = new ApiError(400, 'X', 'x')
    expect(toApiError(original)).toBe(original)
    expect(toApiError(new Error('boom')).code).toBe(CLIENT_CODES.unknown)
    expect(toApiError('string').code).toBe(CLIENT_CODES.unknown)
  })

  it('tidak membocorkan detail teknis ke pesan pengguna', () => {
    const err = toApiError(new Error('TypeError: cannot read properties of undefined'))
    expect(err.message).not.toContain('TypeError')
  })
})

describe('userMessage', () => {
  it('rincian per field menggantikan kalimat umum dan diawali huruf besar', () => {
    const e = new ApiError(422, 'VALIDATION_ERROR', 'Data LPB tidak valid', [
      {
        field: 'serial_number',
        message: 'unit dengan kode ini tidak ditemukan untuk produk pada item ini',
      },
      { field: 'condition', message: 'harus GOOD, MINOR_DEFECT, atau BROKEN' },
    ])
    expect(userMessage(e)).toBe(
      'Unit dengan kode ini tidak ditemukan untuk produk pada item ini; Harus GOOD, MINOR_DEFECT, atau BROKEN',
    )
  })

  it('tanpa rincian field memakai pesan server apa adanya', () => {
    expect(
      userMessage(new ApiError(409, 'INSUFFICIENT_STOCK', 'Stok di rak tidak mencukupi')),
    ).toBe('Stok di rak tidak mencukupi')
  })
})
