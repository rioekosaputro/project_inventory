import { z } from 'zod'

// Pembantu skema zod bersama untuk form master. Pesan Bahasa Indonesia, memberi arah perbaikan.

export const requiredText = (label: string, max: number) =>
  z.string().trim().min(1, `${label} wajib diisi`).max(max, `${label} maksimal ${max} karakter`)

export const optionalText = (label: string, max: number) =>
  z.string().trim().max(max, `${label} maksimal ${max} karakter`)

/** Angka uang sebagai teks desimal (maks 2 desimal, tanpa pemisah ribuan); kosong = tidak diisi. */
export const optionalMoney = z
  .string()
  .trim()
  .refine((v) => v === '' || /^\d{1,13}(\.\d{1,2})?$/.test(v), {
    message: 'Isi angka tanpa pemisah ribuan, maksimal 2 desimal (mis. 550000 atau 550000.50)',
  })

/** Bilangan bulat tidak negatif sebagai teks. */
export const wholeNumber = (label: string) =>
  z
    .string()
    .trim()
    .min(1, `${label} wajib diisi`)
    .regex(/^\d{1,9}$/, `${label} harus berupa bilangan bulat 0 atau lebih`)

/** Teks kosong → null (untuk kolom yang boleh dikosongkan). */
export function nullIfEmpty(value: string): string | null {
  const v = value.trim()
  return v === '' ? null : v
}

/** Koordinat opsional: kosong atau angka dalam rentang. */
export const optionalCoordinate = (label: string, min: number, max: number) =>
  z
    .string()
    .trim()
    .refine(
      (v) => {
        if (v === '') return true
        const n = Number(v)
        return Number.isFinite(n) && n >= min && n <= max
      },
      { message: `${label} harus berupa angka antara ${min} dan ${max}` },
    )
