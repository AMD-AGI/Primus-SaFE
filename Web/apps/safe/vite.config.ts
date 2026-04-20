import { fileURLToPath, URL } from 'node:url'

import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'
import UnoCSS from 'unocss/vite'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  // Load all env vars (including non-VITE_ prefixed) for proxy config
  const env = loadEnv(mode, './', '')

  const API_TARGET = env.PROXY_API_TARGET || 'http://localhost:8088'
  const BACKEND_TARGET = env.PROXY_BACKEND_TARGET || API_TARGET
  const WS_TARGET = env.PROXY_WS_TARGET || API_TARGET.replace(/^http/, 'ws')
  const TOOLS_TARGET = env.PROXY_TOOLS_TARGET || API_TARGET
  const MCP_TARGET = env.PROXY_MCP_TARGET || API_TARGET
  const MCP_REWRITE_PREFIX = env.PROXY_MCP_REWRITE_PREFIX || '/mcp'
  const DEV_DOMAIN = env.PROXY_DEV_DOMAIN || 'localhost'

  return {
    // Set envDir to current directory so Vite reads .env files from apps/safe/
    envDir: './',
    plugins: [vue(), vueDevTools(), UnoCSS()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      host: '0.0.0.0',
      port: 5173,
      allowedHosts: [DEV_DOMAIN],
      proxy: {
        '/api': {
          target: API_TARGET,
          changeOrigin: true,
          secure: false,
          timeout: 1800000, // 30 minutes, for large file uploads
          rewrite: (p) => p.replace(/^\/api/, '/api/v1'),
          cookieDomainRewrite: { '*': DEV_DOMAIN },
          configure(proxy) {
            proxy.on('proxyRes', (proxyRes, req, res) => {
              const setCookie = proxyRes.headers['set-cookie']
              if (setCookie) {
                // Rewrite cookie Domain; strip Secure flag for local http dev
                proxyRes.headers['set-cookie'] = setCookie.map((c) =>
                  c
                    .replace(/;\s*Domain=[^;]+/i, `; Domain=${DEV_DOMAIN}`)
                    // If upstream sends SameSite=None; Secure, it gets dropped in http dev:
                    // Strip Secure (dev only) and change SameSite to Lax so the cookie is accepted
                    .replace(/;\s*Secure/gi, '')
                    .replace(/;\s*SameSite=None/gi, '; SameSite=Lax'),
                )
              }
            })
          },
        },
        '/lens': {
          target: BACKEND_TARGET,
          changeOrigin: true,
          secure: false,
          rewrite: (p) => (p.startsWith('/lens/v1') ? p : p.replace(/^\/lens(\/)?/, '/lens/v1$1')),
        },
        '/root-cause': {
          target: BACKEND_TARGET,
          changeOrigin: true,
          secure: false,
        },
        '/ws': {
          target: WS_TARGET,
          ws: true,
          changeOrigin: true,
        },
        '/agent/ops': {
          target: BACKEND_TARGET,
          ws: true,
          changeOrigin: true,
          secure: false,
          rewrite: (path) => path,
        },
        '/claw-api': {
          target: API_TARGET,
          changeOrigin: true,
          secure: false,
          timeout: 300000,
          cookieDomainRewrite: { '*': DEV_DOMAIN },
          configure(proxy) {
            proxy.on('proxyRes', (proxyRes) => {
              const setCookie = proxyRes.headers['set-cookie']
              if (setCookie) {
                proxyRes.headers['set-cookie'] = setCookie.map((c) =>
                  c
                    .replace(/;\s*Domain=[^;]+/i, `; Domain=${DEV_DOMAIN}`)
                    .replace(/;\s*Secure/gi, '')
                    .replace(/;\s*SameSite=None/gi, '; SameSite=Lax'),
                )
              }
            })
          },
        },
        '/tools-api': {
          target: TOOLS_TARGET,
          changeOrigin: true,
          secure: false,
          rewrite: (p) => p.replace(/^\/tools-api/, ''),
          cookieDomainRewrite: { '*': DEV_DOMAIN },
          configure(proxy) {
            proxy.on('proxyReq', (proxyReq, req) => {
              proxyReq.removeHeader('cookie')
              console.log('[tools-api proxy]', req.method, req.url, '→', TOOLS_TARGET)
              console.log('[tools-api proxy] Authorization:', proxyReq.getHeader('authorization'))
            })
            proxy.on('proxyRes', (proxyRes, req) => {
              console.log('[tools-api proxy] response', proxyRes.statusCode, req.url)
            })
            proxy.on('error', (err, req) => {
              console.error('[tools-api proxy] ERROR', req.url, err.message)
            })
          },
        },
        '/x-flannel': {
          target: API_TARGET,
          changeOrigin: true,
          secure: false,
          rewrite: (path) => path,
        },
        '/mcp-proxy': {
          target: MCP_TARGET,
          changeOrigin: true,
          secure: false,
          timeout: 30000,
          rewrite: (path) => path.replace(/^\/mcp-proxy/, MCP_REWRITE_PREFIX),
        },
      },
    },
  }
})
