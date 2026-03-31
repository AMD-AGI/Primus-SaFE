/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_CLAW_BASE_URL: string
  readonly VITE_MCP_TRACELENS_URL: string
  readonly VITE_MCP_GEAK_URL: string
  readonly VITE_MCP_GEAK_API_KEY: string
  readonly VITE_API_BASE_URL: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<object, object, unknown>
  export default component
}
