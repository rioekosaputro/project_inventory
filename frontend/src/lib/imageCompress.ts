const DEFAULT_MAX_BYTES = 300 * 1024 // < 300 KB sesuai blueprint & batas body serverless Vercel
const DEFAULT_MAX_DIMENSION = 1600

/** Menghasilkan UUID v4 untuk idempotensi aksi driver (`client_event_id`). */
export function generateEventId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

/**
 * Mengompres foto dari kamera HP ke JPEG < 300 KB di browser sebelum diunggah.
 * Jika berkas sudah di bawah batas dan berformat gambar, atau browser tidak mendukung canvas,
 * mengembalikan berkas aslinya dengan aman.
 */
export async function compressImageToJpeg(
  file: File,
  maxBytes = DEFAULT_MAX_BYTES,
  maxDimension = DEFAULT_MAX_DIMENSION,
): Promise<File> {
  if (file.size <= maxBytes && file.type === 'image/jpeg') {
    return file
  }

  const bitmapUrl = URL.createObjectURL(file)
  try {
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const el = new Image()
      el.onload = () => resolve(el)
      el.onerror = () => reject(new Error('Gagal membaca file gambar.'))
      el.src = bitmapUrl
    })

    let width = img.naturalWidth || img.width
    let height = img.naturalHeight || img.height
    if (width > maxDimension || height > maxDimension) {
      const ratio = Math.min(maxDimension / width, maxDimension / height)
      width = Math.max(1, Math.round(width * ratio))
      height = Math.max(1, Math.round(height * ratio))
    }

    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const ctx = canvas.getContext('2d')
    if (!ctx) return file

    ctx.fillStyle = '#ffffff'
    ctx.fillRect(0, 0, width, height)
    ctx.drawImage(img, 0, 0, width, height)

    let quality = 0.82
    let blob: Blob | null = null
    for (let attempt = 0; attempt < 6; attempt++) {
      blob = await new Promise<Blob | null>((resolve) =>
        canvas.toBlob((b) => resolve(b), 'image/jpeg', quality),
      )
      if (!blob || blob.size <= maxBytes || quality <= 0.35) {
        break
      }
      quality = Math.max(0.35, quality - 0.12)
    }

    if (!blob) return file
    const baseName = file.name.replace(/\.[^.]+$/, '') || 'pod-photo'
    return new File([blob], `${baseName}.jpg`, { type: 'image/jpeg' })
  } finally {
    URL.revokeObjectURL(bitmapUrl)
  }
}
