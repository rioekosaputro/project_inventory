const rupiah = new Intl.NumberFormat('id-ID', {
  style: 'currency',
  currency: 'IDR',
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})

/**
 * Tampilkan uang dari API (string desimal) sebagai Rupiah. Hanya untuk tampilan: angka final
 * dan seluruh perhitungan pajak tetap dari backend. Kosong/null → tanda "—".
 */
export function formatRupiah(value: string | null | undefined): string {
  if (value === null || value === undefined || value === '') return '—'
  // Intl.NumberFormat menerima string desimal tanpa lewat float untuk nilai sebesar NUMERIC(15,2).
  return rupiah.format(value as unknown as number)
}

const WIB = 'Asia/Jakarta'

/** Tanggal hari ini menurut WIB sebagai "YYYY-MM-DD" (tanggal bisnis, invarian #5). */
export function todayWIB(now: Date = new Date()): string {
  return new Intl.DateTimeFormat('en-CA', {
    timeZone: WIB,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(now)
}

const dateFmt = new Intl.DateTimeFormat('id-ID', {
  timeZone: WIB,
  day: '2-digit',
  month: 'short',
  year: 'numeric',
})
const dateTimeFmt = new Intl.DateTimeFormat('id-ID', {
  timeZone: WIB,
  day: '2-digit',
  month: 'short',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
})

/** "2026-11-21" atau stempel ISO → "21 Nov 2026" (WIB). Kosong → "—". */
export function formatDate(value: string | null | undefined): string {
  if (!value) return '—'
  // Tanggal murni ("YYYY-MM-DD") ditafsirkan sebagai tengah hari WIB agar tidak bergeser hari.
  const d = /^\d{4}-\d{2}-\d{2}$/.test(value)
    ? new Date(`${value}T12:00:00+07:00`)
    : new Date(value)
  return Number.isNaN(d.getTime()) ? '—' : dateFmt.format(d)
}

/** Stempel waktu ISO → "21 Nov 2026 14.05" (WIB). */
export function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—'
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? '—' : dateTimeFmt.format(d)
}
