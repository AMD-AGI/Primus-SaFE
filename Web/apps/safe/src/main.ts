import { createApp } from 'vue'
import { createPinia } from 'pinia'
import piniaPersist from 'pinia-plugin-persistedstate'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import '@/assets/main.scss'
import './assets/main.css'
import 'uno.css'

import App from './App.vue'
import router from './router'
import vRoute from '@/directives/vRoute'
import { useUserStore } from './stores/user'

// ---------------------------------------------------------------------------
// Vite preload error handler — catches chunk preload failures globally
// (complements the router.onError handler in router/index.ts)
// ---------------------------------------------------------------------------
window.addEventListener('vite:preloadError', (event) => {
  console.warn('[Vite] Preload error detected, reloading page…', event)
  // Prevent the default error behaviour and force a full reload
  event.preventDefault()
  window.location.reload()
})

const app = createApp(App)

// Global default loading text
app.config.globalProperties.$loadingText = `Primus SaFE\n(Stability and Fault Endurance)`
declare module 'vue' {
  interface ComponentCustomProperties {
    $loadingText: string
  }
}

const pinia = createPinia()
pinia.use(piniaPersist)

app.use(pinia)
app.use(router)
app.use(ElementPlus, { zIndex: 3000 })
app.directive('route', vRoute)

const user = useUserStore(pinia)
user
  .ensureSessionOnce()
  .finally(() => router.isReady())
  .then(() => app.mount('#app'))
