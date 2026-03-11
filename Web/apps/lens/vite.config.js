import { fileURLToPath, URL } from 'node:url'

import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'
import UnoCSS from 'unocss/vite'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  // Load all env vars (including non-VITE_ prefixed) for proxy config
  const env = loadEnv(mode, './', '')

  const BACKEND_TARGET = env.PROXY_BACKEND_TARGET || 'http://localhost:8088'
  const DEV_DOMAIN = env.PROXY_DEV_DOMAIN || 'localhost'

  return {
    base: '/lens/', // Set base path to /lens/
    server: {
      port: 3000,
      host: true, // Listen on all addresses
      hmr: true, // Explicitly enable HMR
      allowedHosts: [DEV_DOMAIN],
      watch: {
        usePolling: true, // Use polling mode to fix file system watch issues
        interval: 1000, // Polling interval
      },
      proxy: {
        // Lens Agent Chat API proxy
        '/lens/v1/agent': {
          target: env.PROXY_AGENT_TARGET || 'http://localhost:8003',  // lens-agent-chat service port
          changeOrigin: true,
          secure: false,
          rewrite: (path) => path.replace(/^\/lens\/v1\/agent/, '/api/v1')
        },
        // Lens API proxy (general rule, matches all other /v1 requests)
        '/lens/v1': {
          target: BACKEND_TARGET,
          changeOrigin: true,
          secure: false, // Ignore SSL certificate verification
          cookieDomainRewrite: {
            '*': DEV_DOMAIN,
          },
        },
        // Safe API proxy
        '/api/v1': {
          target: BACKEND_TARGET,
          changeOrigin: true,
          secure: false,
          cookieDomainRewrite: {
            '*': DEV_DOMAIN,
          },
        }
      }
    },
    preview: {
      host: true,
      allowedHosts: [DEV_DOMAIN],
    },
    plugins: [
      vue(),
      vueDevTools(),
      UnoCSS(),
    ],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url))
      },
    },
  }
})
