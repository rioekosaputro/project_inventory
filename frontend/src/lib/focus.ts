import { nextTick } from 'vue'

let programmaticFocus = false

/**
 * true selama fokus dipindahkan oleh program (bukan oleh pengguna). Dipakai field agar tidak
 * menghapus error yang justru baru ditampilkan: error baru hilang saat PENGGUNA fokus/mengetik.
 */
export function isProgrammaticFocus(): boolean {
  return programmaticFocus
}

/**
 * Setelah submit gagal, pindahkan fokus ke field invalid pertama (pembaca layar membacakan
 * error-nya) dan gulirkan ke area terlihat.
 */
export async function focusFirstInvalid(root: ParentNode | null | undefined): Promise<void> {
  await nextTick()
  const el = root?.querySelector<HTMLElement>('[aria-invalid="true"]')
  if (!el) return

  programmaticFocus = true
  try {
    el.focus() // event focus dikirim sinkron, jadi flag masih aktif saat handler field berjalan
  } finally {
    programmaticFocus = false
  }
  el.scrollIntoView?.({ block: 'center' })
}
