import { fileURLToPath } from 'node:url'
import { mergeConfig, defineConfig, configDefaults } from 'vitest/config'
import viteConfig from './vite.config.ts'

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: 'jsdom',
      exclude: [...configDefaults.exclude, 'e2e/**'],
      root: fileURLToPath(new URL('./', import.meta.url)),
      setupFiles: ['./src/test/setup.ts'],
      css: false,
      // Tes pertama tiap berkas memuat banyak modul (PrimeVue, seluruh view). Di runner CI 2 vCPU itu dapat
      // melampaui 5 detik bawaan, padahal tesnya benar — jadi batas dilonggarkan (tes yang benar-benar
      // menggantung tetap gagal, hanya setelah 20 detik).
      testTimeout: 20_000,
      hookTimeout: 30_000,
    },
  }),
)
