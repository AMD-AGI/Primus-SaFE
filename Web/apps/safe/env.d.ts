/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL: string
  readonly VITE_LENS_BASE_URL: string
  readonly VITE_ROOT_CAUSE_BASE_URL: string
  readonly VITE_AGENT_BASE_URL: string
  readonly VITE_AGENT_PATH: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
