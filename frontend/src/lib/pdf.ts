/** Membuka PDF Blob di tab baru (atau mengunduh bila pop-up diblokir browser). */
export function openPdfBlob(blob: Blob, fallbackFilename: string): void {
  const url = URL.createObjectURL(blob)
  const opened = window.open(url, '_blank', 'noopener')
  if (!opened) {
    const a = document.createElement('a')
    a.href = url
    a.download = fallbackFilename
    a.click()
  }
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
}
