import { fileURLToPath, URL } from 'node:url'
import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import UnoCSS from 'unocss/vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, './', '')

  const API_TARGET = env.PROXY_API_TARGET || 'http://localhost:8088'
  const MCP_TARGET = env.PROXY_MCP_TARGET || API_TARGET
  const DEV_DOMAIN = env.PROXY_DEV_DOMAIN || 'localhost'

  return {
    base: '/hyperloom/',
    envDir: './',
    plugins: [vue(), UnoCSS()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      port: 3001,
      host: true,
      hmr: true,
      allowedHosts: true,
      proxy: {
        '/claw-api': {
          target: API_TARGET,
          changeOrigin: true,
          secure: false,
          timeout: 1800000,
          cookieDomainRewrite: { '*': DEV_DOMAIN },
          configure(proxy) {
            proxy.on('proxyReq', (proxyReq) => {
              proxyReq.setHeader('Connection', 'keep-alive')
            })
          },
        },
        '/tools': {
          target: API_TARGET,
          changeOrigin: true,
          secure: false,
          cookieDomainRewrite: { '*': DEV_DOMAIN },
          rewrite: (path: string) =>
            path.replace(/^\/tools\/api\/v1/, '/api/v1'),
        },
        '/mcp/tracelens': {
          target: MCP_TARGET,
          changeOrigin: true,
          secure: false,
          timeout: 60000,
          cookieDomainRewrite: { '*': DEV_DOMAIN },
          rewrite: (path: string) =>
            path.replace(
              /^\/mcp\/tracelens/,
              '/control-plane/control-plane-prod/trace-lens-agent-bwrmr/mcp',
            ),
        },
        '/mcp/geak': {
          target: MCP_TARGET,
          changeOrigin: true,
          secure: false,
          timeout: 60000,
          cookieDomainRewrite: { '*': DEV_DOMAIN },
          rewrite: (path: string) =>
            path.replace(
              /^\/mcp\/geak/,
              '/control-plane/control-plane-dev/geak-agent-wvsbv/mcp/sse',
            ),
        },
        '/api': {
          target: API_TARGET,
          changeOrigin: true,
          secure: false,
          rewrite: (p: string) => (p.startsWith('/api/v1') ? p : p.replace(/^\/api/, '/api/v1')),
          cookieDomainRewrite: { '*': DEV_DOMAIN },
          cookiePathRewrite: { '*': '/' },
        },
      },
    },
  }
})
