/** Membuka PDF Blob di tab baru (atau mengunduh bila pop-up diblokir browser). */
export function openPdfBlob(blob: Blob, fallbackFilename: string): void {
  const url = URL.createObjectURL(blob)
  const opened = window.open(url, '_blank', 'noopener')
  if (!opened) {
    downloadBlob(blob, fallbackFilename)
  }
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
}

/** Mengunduh file Blob (mis. Excel .xlsx atau PDF) dengan nama berkas tertentu. */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
}
