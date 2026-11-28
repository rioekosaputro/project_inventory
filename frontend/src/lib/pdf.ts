const A4_WIDTH_MM = 210
const A4_HEIGHT_MM = 297
const MARGIN_MM = 10
const CONTENT_WIDTH_MM = A4_WIDTH_MM - MARGIN_MM * 2
const CONTENT_HEIGHT_MM = A4_HEIGHT_MM - MARGIN_MM * 2
const RENDER_WIDTH_PX = 794

/** Memeriksa apakah Blob benar-benar berkas PDF biner (header `%PDF-`). */
export async function isPdfBlob(blob: Blob): Promise<boolean> {
  if (blob.size < 5) return false
  const header = await blob.slice(0, 5).text()
  return header === '%PDF-'
}

/** Menghapus toolbar pratinjau cetak & script auto-print dari HTML fallback backend. */
function sanitizePrintHtml(rawHtml: string): string {
  const parser = new DOMParser()
  const doc = parser.parseFromString(rawHtml, 'text/html')
  for (const el of doc.querySelectorAll('script, .erp-print-bar')) {
    el.remove()
  }
  return `<!DOCTYPE html>${doc.documentElement.outerHTML}`
}

async function waitForImages(doc: Document): Promise<void> {
  const imgs = Array.from(doc.images)
  await Promise.all(
    imgs.map((img) => {
      if (img.complete) return Promise.resolve()
      return new Promise<void>((resolve) => {
        img.addEventListener('load', () => resolve(), { once: true })
        img.addEventListener('error', () => resolve(), { once: true })
      })
    }),
  )
}

/**
 * Mengonversi HTML siap-cetak (fallback backend di Vercel serverless tanpa Gotenberg)
 * menjadi Blob PDF biner asli (`application/pdf`, header `%PDF-`) di sisi klien.
 * Dirender dalam iframe terisolasi agar CSS Tailwind v4 (`oklch()`) tidak bocor ke template.
 */
export async function htmlBlobToPdfBlob(htmlBlob: Blob): Promise<Blob> {
  const cleanHtml = sanitizePrintHtml(await htmlBlob.text())
  const [{ default: html2canvas }, { jsPDF }] = await Promise.all([
    import('html2canvas'),
    import('jspdf'),
  ])

  const iframe = document.createElement('iframe')
  iframe.style.position = 'fixed'
  iframe.style.left = '-10000px'
  iframe.style.top = '0'
  iframe.style.width = `${RENDER_WIDTH_PX}px`
  iframe.style.height = '1123px'
  iframe.style.border = '0'
  iframe.style.opacity = '0'
  iframe.style.pointerEvents = 'none'
  document.body.appendChild(iframe)

  try {
    await new Promise<void>((resolve, reject) => {
      iframe.addEventListener('load', () => resolve(), { once: true })
      iframe.addEventListener('error', () => reject(new Error('Gagal memuat dokumen PDF.')), {
        once: true,
      })
      iframe.srcdoc = cleanHtml
    })

    const iframeDoc = iframe.contentDocument
    if (!iframeDoc?.body) {
      throw new Error('Dokumen pratinjau PDF kosong.')
    }

    await waitForImages(iframeDoc)

    // Sesuaikan tinggi iframe dengan konten penuh agar tidak terpotong saat capture.
    const fullHeight = Math.max(
      iframeDoc.body.scrollHeight,
      iframeDoc.documentElement.scrollHeight,
      1123,
    )
    iframe.style.height = `${fullHeight}px`

    const canvas = await html2canvas(iframeDoc.body, {
      scale: 2,
      useCORS: true,
      backgroundColor: '#ffffff',
      windowWidth: RENDER_WIDTH_PX,
      windowHeight: fullHeight,
    })

    const pdf = new jsPDF({
      orientation: 'portrait',
      unit: 'mm',
      format: 'a4',
      compress: true,
    })

    const pxPerMm = canvas.width / CONTENT_WIDTH_MM
    const pageHeightPx = Math.floor(CONTENT_HEIGHT_MM * pxPerMm)

    if (canvas.height <= pageHeightPx) {
      const renderedHeightMm = canvas.height / pxPerMm
      pdf.addImage(
        canvas.toDataURL('image/png'),
        'PNG',
        MARGIN_MM,
        MARGIN_MM,
        CONTENT_WIDTH_MM,
        renderedHeightMm,
      )
    } else {
      let offsetY = 0
      let pageIdx = 0
      while (offsetY < canvas.height) {
        const sliceHeight = Math.min(pageHeightPx, canvas.height - offsetY)
        const sliceCanvas = document.createElement('canvas')
        sliceCanvas.width = canvas.width
        sliceCanvas.height = sliceHeight
        const ctx = sliceCanvas.getContext('2d')
        if (ctx) {
          ctx.fillStyle = '#ffffff'
          ctx.fillRect(0, 0, sliceCanvas.width, sliceCanvas.height)
          ctx.drawImage(
            canvas,
            0,
            offsetY,
            canvas.width,
            sliceHeight,
            0,
            0,
            canvas.width,
            sliceHeight,
          )
        }
        if (pageIdx > 0) {
          pdf.addPage()
        }
        pdf.addImage(
          sliceCanvas.toDataURL('image/png'),
          'PNG',
          MARGIN_MM,
          MARGIN_MM,
          CONTENT_WIDTH_MM,
          sliceHeight / pxPerMm,
        )
        offsetY += sliceHeight
        pageIdx += 1
      }
    }

    return pdf.output('blob')
  } finally {
    iframe.remove()
  }
}

/**
 * Mengunduh dokumen sebagai berkas `.pdf` yang valid.
 * - Jika backend (Gotenberg / Chrome lokal) mengembalikan biner `%PDF-`, langsung diunduh.
 * - Jika backend (Vercel serverless) mengembalikan fallback `text/html`, dikonversi dulu
 *   menjadi berkas PDF biner asli di browser agar tidak pernah menghasilkan `.pdf` korup.
 */
export async function openPdfBlob(blob: Blob, fallbackFilename: string): Promise<void> {
  const filename = fallbackFilename.toLowerCase().endsWith('.pdf')
    ? fallbackFilename
    : `${fallbackFilename}.pdf`
  const pdfBlob = (await isPdfBlob(blob)) ? blob : await htmlBlobToPdfBlob(blob)
  downloadBlob(pdfBlob, filename)
}

/** Mengunduh file Blob (mis. Excel .xlsx atau PDF) dengan nama berkas tertentu. */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
}
