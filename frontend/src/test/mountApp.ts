import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { createMemoryHistory, type RouteRecordRaw, type Router } from 'vue-router'
import App from '@/App.vue'
import { createAppRouter } from '@/router'
import { useAuthStore } from '@/stores/auth'

/**
 * Menunggu microtask DAN timer singkat. vee-validate memproses validasi dengan jeda timer,
 * jadi `flushPromises()` saja belum cukup untuk melihat error di DOM.
 */
export async function settle(ms = 30): Promise<void> {
  await flushPromises()
  await new Promise((resolve) => setTimeout(resolve, ms))
  await flushPromises()
}

/**
 * Memuat semua komponen rute yang lazy (`() => import(...)`) lebih dulu. Tanpa ini, navigasi pertama ke
 * tiap halaman menunggu impor dinamis yang waktunya bergantung beban mesin — sumber tes yang lulus di
 * laptop tetapi gagal di CI yang lebih lambat. Setelah dimuat, navigasi cukup menunggu microtask.
 */
export async function preloadRouteComponents(router: Router): Promise<void> {
  const loaders: (() => Promise<unknown>)[] = []
  for (const record of router.getRoutes()) {
    for (const c of Object.values(record.components ?? {})) {
      // Pembeda yang sama dengan vue-router: fungsi tanpa penanda komponen = pemuat lazy.
      if (
        typeof c === 'function' &&
        !('displayName' in c) &&
        !('props' in c) &&
        !('__vccOpts' in c)
      ) {
        loaders.push(c as () => Promise<unknown>)
      }
    }
  }
  await Promise.all(loaders.map((load) => load()))
}

/**
 * Me-mount aplikasi penuh (App + router + Pinia + PrimeVue) pada path tertentu.
 * Panggil setelah authService di-mock (sesi/anonim ditentukan oleh hasil `refresh`).
 */
export async function mountApp(path: string, extraRoutes: RouteRecordRaw[] = []) {
  const pinia = createPinia()
  const router = createAppRouter(createMemoryHistory())
  for (const r of extraRoutes) router.addRoute(r)
  await preloadRouteComponents(router)

  const wrapper = mount(App, {
    global: { plugins: [pinia, router, [PrimeVue, { theme: 'none' }], ToastService] },
    attachTo: document.body,
  })
  await router.push(path)
  await router.isReady()
  await settle()

  return { wrapper, router, auth: useAuthStore(pinia) }
}

export const stubRoute = (path: string, name: string, permission?: string): RouteRecordRaw => ({
  path,
  name,
  component: { render: () => null },
  meta: { layout: 'app', ...(permission ? { permission } : {}) },
})
