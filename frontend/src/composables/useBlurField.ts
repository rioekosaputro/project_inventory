import { useField } from 'vee-validate'
import { announce } from '@/composables/useAnnouncer'
import { isProgrammaticFocus } from '@/lib/focus'

/**
 * Perilaku validasi field (UX forms):
 *  - tidak ada error saat mengetik di field yang masih "pristine";
 *  - validasi dijalankan saat blur pertama; field wajib yang kosong error saat blur;
 *  - begitu invalid, error hilang seketika saat fokus atau ketikan perbaikan pertama;
 *  - tombol submit memvalidasi semuanya (handleSubmit) dan memfokuskan field invalid pertama.
 *
 * Validasi saat blur memakai mode `silent` lalu menerapkan hasilnya HANYA ke field ini. Mode
 * bawaan vee-validate (`validated-only`) menerapkan hasil skema ke SEMUA field yang pernah
 * divalidasi, sehingga blur di Password bisa memunculkan kembali error Email yang baru saja
 * dihapus saat Email difokus.
 */
export function useBlurField(name: string, label: () => string) {
  const { value, errorMessage, validate, setErrors } = useField<string>(name, undefined, {
    validateOnValueUpdate: false,
  })

  // Menandai validasi yang sedang berjalan; fokus ulang oleh pengguna membatalkannya agar
  // hasil yang datang terlambat tidak muncul di field yang sedang diperbaiki.
  let blurSeq = 0

  async function onBlur(): Promise<void> {
    const seq = ++blurSeq
    const result = await validate({ mode: 'silent' })
    if (seq !== blurSeq) return

    setErrors(result.valid ? [] : result.errors)
    const first = result.errors[0]
    if (!result.valid && first) announce(`${label()}: ${first}`)
  }

  function clearError(): void {
    if (errorMessage.value) setErrors([])
  }

  // Fokus yang dipindahkan program setelah submit gagal tidak boleh menghapus error-nya.
  function onFocus(): void {
    if (isProgrammaticFocus()) return
    blurSeq++
    clearError()
  }

  function onInput(): void {
    blurSeq++
    clearError()
  }

  return { value, error: errorMessage, onBlur, onFocus, onInput }
}
