import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { isPdfBlob, openPdfBlob } from '../pdf'

const html2canvasMock = vi.hoisted(() =>
  vi.fn<() => Promise<{ width: number; height: number; toDataURL: () => string }>>(),
)
const addImageMock = vi.hoisted(() => vi.fn<() => void>())
const addPageMock = vi.hoisted(() => vi.fn<() => void>())
const outputMock = vi.hoisted(() => vi.fn<() => Blob>())

vi.mock('html2canvas', () => ({
  default: html2canvasMock,
}))

vi.mock('jspdf', () => ({
  jsPDF: class MockJsPDF {
    addImage = addImageMock
    addPage = addPageMock
    output = outputMock
  },
}))

describe('pdf helper', () => {
  const createObjectURL = vi.fn<(blob: Blob) => string>(() => 'blob:http://localhost/mock-doc')
  const revokeObjectURL = vi.fn<() => void>()

  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL })
    createObjectURL.mockClear()
    revokeObjectURL.mockClear()
    html2canvasMock.mockReset()
    addImageMock.mockReset()
    addPageMock.mockReset()
    outputMock.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('isPdfBlob mengenali header biner %PDF- dan menolak HTML fallback', async () => {
    expect(await isPdfBlob(new Blob(['%PDF-1.4\n...'], { type: 'application/pdf' }))).toBe(true)
    expect(await isPdfBlob(new Blob(['<!DOCTYPE html><html></html>'], { type: 'text/html' }))).toBe(
      false,
    )
    expect(await isPdfBlob(new Blob(['<html>fake pdf</html>'], { type: 'application/pdf' }))).toBe(
      false,
    )
  })

  it('langsung mengunduh Blob %PDF- asli tanpa membuka popup preview atau memuat konverter HTML', async () => {
    const openSpy = vi.spyOn(window, 'open')
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    const pdfBlob = new Blob(['%PDF-1.4\ncontent'], { type: 'application/pdf' })
    await openPdfBlob(pdfBlob, 'INV-001.pdf')

    expect(openSpy).not.toHaveBeenCalled()
    expect(html2canvasMock).not.toHaveBeenCalled()
    expect(createObjectURL).toHaveBeenCalledWith(pdfBlob)
    expect(clickSpy).toHaveBeenCalledTimes(1)
  })

  it('mengonversi fallback HTML Vercel menjadi Blob PDF biner asli sebelum diunduh', async () => {
    const generatedPdf = new Blob(['%PDF-1.3\ngenerated'], { type: 'application/pdf' })
    html2canvasMock.mockResolvedValue({
      width: 1588,
      height: 1200,
      toDataURL: () => 'data:image/png;base64,AAAA',
    })
    outputMock.mockReturnValue(generatedPdf)

    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const origAppend = document.body.appendChild.bind(document.body)
    vi.spyOn(document.body, 'appendChild').mockImplementation((node: Node) => {
      const appended = origAppend(node)
      if (node instanceof HTMLIFrameElement) {
        // jsdom tidak memicu event load secara otomatis pada iframe.srcdoc
        queueMicrotask(() => {
          Object.defineProperty(node, 'contentDocument', {
            configurable: true,
            value: new DOMParser().parseFromString(node.srcdoc, 'text/html'),
          })
          node.dispatchEvent(new Event('load'))
        })
      }
      return appended
    })

    const htmlFallback = new Blob(
      [
        '<!DOCTYPE html><html><body><div class="erp-print-bar">Bar</div><script>window.print()</script><h1>INVOICE</h1></body></html>',
      ],
      { type: 'text/html; charset=utf-8' },
    )

    await openPdfBlob(htmlFallback, 'INV-001.pdf')

    expect(html2canvasMock).toHaveBeenCalledTimes(1)
    expect(addImageMock).toHaveBeenCalledTimes(1)
    expect(createObjectURL).toHaveBeenCalledWith(generatedPdf)
    expect(clickSpy).toHaveBeenCalledTimes(1)
    expect(document.body.querySelector('iframe')).toBeNull()
  })
})
