/**
 * Umpan balik suara untuk scanner (plan Bagian 9.3): satu `AudioContext` untuk seluruh aplikasi.
 * Browser menahan audio sampai ada interaksi pengguna, jadi konteks dibuat malas dan `resume()`
 * dipanggil pada gestur pertama (klik/ketuk/tombol). Tanpa dukungan audio, semua fungsi diam saja —
 * suara hanya pelengkap, tidak pernah satu-satunya penanda hasil scan.
 */

export type Tone = 'success' | 'error' | 'duplicate'

let ctx: AudioContext | null = null
let muted = false

function context(): AudioContext | null {
  if (ctx) return ctx
  const Ctor =
    typeof window === 'undefined'
      ? undefined
      : (window.AudioContext ??
        (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext)
  if (!Ctor) return null
  try {
    ctx = new Ctor()
  } catch {
    ctx = null
  }
  return ctx
}

/** Panggil dari handler gestur pengguna (klik/keydown) agar konteks boleh berbunyi. */
export function unlockAudio(): void {
  const c = context()
  if (c && c.state === 'suspended') void c.resume().catch(() => {})
}

export function setMuted(value: boolean): void {
  muted = value
}
export function isMuted(): boolean {
  return muted
}

interface Note {
  freq: number
  ms: number
}

const PATTERNS: Record<Tone, Note[]> = {
  success: [{ freq: 880, ms: 90 }],
  duplicate: [
    { freq: 440, ms: 80 },
    { freq: 440, ms: 80 },
  ],
  error: [{ freq: 220, ms: 260 }],
}

/** Bunyikan nada singkat. Aman dipanggil kapan saja (tidak melempar). */
export function beep(tone: Tone): void {
  if (muted) return
  const c = context()
  if (!c) return
  if (c.state === 'suspended') void c.resume().catch(() => {})

  let t = c.currentTime
  for (const note of PATTERNS[tone]) {
    const osc = c.createOscillator()
    const gain = c.createGain()
    osc.type = tone === 'error' ? 'sawtooth' : 'sine'
    osc.frequency.value = note.freq
    // Naik-turun cepat agar tidak ada bunyi "klik" di awal/akhir nada.
    gain.gain.setValueAtTime(0.0001, t)
    gain.gain.exponentialRampToValueAtTime(0.2, t + 0.01)
    gain.gain.exponentialRampToValueAtTime(0.0001, t + note.ms / 1000)
    osc.connect(gain).connect(c.destination)
    osc.start(t)
    osc.stop(t + note.ms / 1000 + 0.02)
    t += note.ms / 1000 + 0.06
  }
}

/** Hanya untuk test: buang singleton. */
export function resetAudioForTest(): void {
  ctx = null
  muted = false
}
