import './assets/main.css'
import 'primeicons/primeicons.css'

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'

import App from './App.vue'
import { createAppRouter } from './router'
import { installAuthBridge } from '@/lib/authBridge'
import { useAuthStore } from '@/stores/auth'
import { primeVueOptions } from '@/theme/primevue'

const app = createApp(App)
const pinia = createPinia()
const router = createAppRouter()

app.use(pinia)
app.use(router)
app.use(PrimeVue, primeVueOptions)
app.use(ToastService)

installAuthBridge(router, useAuthStore(pinia))

app.mount('#app')

if (import.meta.env.PROD && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    void navigator.serviceWorker.register('/sw.js').catch(() => {})
  })
}
