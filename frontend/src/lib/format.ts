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
