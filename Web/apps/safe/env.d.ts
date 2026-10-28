/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL: string
  readonly VITE_LENS_BASE_URL: string
  readonly VITE_ROOT_CAUSE_BASE_URL: string
  readonly VITE_AGENT_BASE_URL: string
  readonly VITE_AGENT_PATH: string
  // Build-time switch: 'false' reads the workspace lists from the user record instead of
  // GET /users/self/workspaces. Anything else (including unset) uses the route.
  readonly VITE_SELF_WORKSPACES?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
