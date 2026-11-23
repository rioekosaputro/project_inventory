import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { beep, isMuted, resetAudioForTest, setMuted, unlockAudio } from '@/lib/audio'

interface FakeOsc {
  type: string
  frequency: { value: number }
  connect: (n: unknown) => unknown
  start: () => void
  stop: () => void
}
interface FakeCtx {
  state: string
  resume: ReturnType<typeof vi.fn>
  oscillators: FakeOsc[]
}

/** AudioContext palsu: mencatat instansi yang dibuat dan oscillator yang dibunyikan. */
function installFakeAudio(state: 'running' | 'suspended' = 'suspended') {
  const instances: FakeCtx[] = []
  class FakeAudioContext {
    state = state
    currentTime = 0
    destination = {}
    resume = vi.fn<() => Promise<void>>(() => Promise.resolve())
    oscillators: FakeOsc[] = []
    constructor() {
      instances.push(this)
    }
    createOscillator(): FakeOsc {
      const o: FakeOsc = {
        type: '',
        frequency: { value: 0 },
        connect: (n) => n,
        start: () => {},
        stop: () => {},
      }
      this.oscillators.push(o)
      return o
    }
    createGain() {
      return {
        gain: { setValueAtTime: () => {}, exponentialRampToValueAtTime: () => {} },
        connect: (n: unknown) => n,
      }
    }
  }
  window.AudioContext = FakeAudioContext as unknown as typeof AudioContext
  return instances
}

beforeEach(() => resetAudioForTest())
afterEach(() => {
  ;(window as unknown as { AudioContext?: unknown }).AudioContext = undefined
  resetAudioForTest()
})

describe('audio scanner', () => {
  it('AudioContext hanya dibuat sekali (singleton) walau dibunyikan berkali-kali', () => {
    const instances = installFakeAudio()
    beep('success')
    beep('error')
    unlockAudio()
    expect(instances).toHaveLength(1)
  })

  it('konteks yang masih suspended di-resume pada gestur pertama', () => {
    const instances = installFakeAudio('suspended')
    unlockAudio()
    expect(instances[0]!.resume).toHaveBeenCalled()
  })

  it('tiap nada punya jumlah bunyi sendiri: sukses 1, dobel 2, gagal 1', () => {
    const instances = installFakeAudio('running')
    beep('success')
    expect(instances[0]!.oscillators).toHaveLength(1)
    beep('duplicate')
    expect(instances[0]!.oscillators).toHaveLength(3)
    beep('error')
    expect(instances[0]!.oscillators).toHaveLength(4)
  })

  it('mode senyap tidak membunyikan apa pun (konteks pun tidak dibuat)', () => {
    const instances = installFakeAudio('running')
    setMuted(true)
    expect(isMuted()).toBe(true)
    beep('success')
    expect(instances).toHaveLength(0)
  })

  it('tanpa dukungan audio semua fungsi diam dan tidak melempar', () => {
    ;(window as unknown as { AudioContext?: unknown }).AudioContext = undefined
    expect(() => {
      unlockAudio()
      beep('success')
    }).not.toThrow()
  })
})
