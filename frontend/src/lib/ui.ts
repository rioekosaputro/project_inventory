// Kelas Tailwind bersama untuk tombol (satu sumber gaya; target sentuh minimal 44px).

const base =
  'inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-4 text-sm font-medium whitespace-nowrap disabled:cursor-not-allowed disabled:opacity-60'

export const btnPrimary = `${base} bg-indigo-700 text-white hover:bg-indigo-800`
export const btnSecondary = `${base} border border-slate-300 bg-white text-slate-800 hover:bg-slate-50`
export const btnDanger = `${base} border border-red-300 bg-white text-red-800 hover:bg-red-50`
/** Tombol aksi di dalam baris tabel (tetap 44px agar mudah disentuh). */
export const btnRow = `${base} px-3 text-slate-800 hover:bg-slate-100`

/** Tautan di dalam kalimat/subjudul: boleh membungkus (nama pelanggan panjang di HP). */
export const textLink =
  'inline-block py-1.5 font-medium wrap-break-word text-indigo-800 hover:underline'

/** Tautan pada sel utama baris (kode/SKU): area sentuh 44px, tampil sebagai tautan. */
export const rowLink =
  'inline-flex min-h-11 items-center font-medium whitespace-nowrap text-indigo-800 hover:underline'
