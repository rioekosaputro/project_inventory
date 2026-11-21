import Aura from '@primeuix/themes/aura'
import { definePreset } from '@primeuix/themes'
import type { PrimeVueConfiguration } from 'primevue/config'

/**
 * Aura dengan warna primer indigo. Warna aksi (tombol) memakai indigo-700 agar teks putih di
 * atasnya memenuhi kontras WCAG AA (≥ 4.5:1); default Aura (indigo-500) hanya ≈ 4.4:1.
 */
export const AppPreset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '{indigo.50}',
      100: '{indigo.100}',
      200: '{indigo.200}',
      300: '{indigo.300}',
      400: '{indigo.400}',
      500: '{indigo.500}',
      600: '{indigo.600}',
      700: '{indigo.700}',
      800: '{indigo.800}',
      900: '{indigo.900}',
      950: '{indigo.950}',
    },
    colorScheme: {
      light: {
        primary: {
          color: '{indigo.700}',
          contrastColor: '#ffffff',
          hoverColor: '{indigo.800}',
          activeColor: '{indigo.900}',
        },
      },
    },
  },
})

export const primeVueOptions: PrimeVueConfiguration = {
  theme: {
    preset: AppPreset,
    options: {
      // Mode gelap belum didukung: selector yang tidak pernah dipasang mencegah Aura mengikuti
      // preferensi sistem sementara layout Tailwind masih terang.
      darkModeSelector: '.app-dark',
      // Urutan layer: PrimeVue di antara base dan utilities Tailwind (lihat assets/main.css).
      cssLayer: { name: 'primevue', order: 'theme, base, primevue' },
    },
  },
}
