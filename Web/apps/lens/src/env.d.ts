  declare module '*.vue' {
    import type { DefineComponent } from 'vue'
    const component: DefineComponent<{}, {}, any>
    export default component
  }

  declare module '*.png' {
    const value: string
    export default value
  }

  declare module '*.jpg' {
    const value: string
    export default value
  }

  declare module '*.jpeg' {
    const value: string
    export default value
  }

  declare module '*.gif' {
    const value: string
    export default value
  }

  declare module '*.svg' {
    const value: string
    export default value
  }

  interface ImportMetaEnv {
    readonly VITE_API_BASE_URL: string
    readonly BASE_URL: string
    readonly PROD: boolean
    readonly DEV: boolean
    readonly MODE: string
    readonly VITE_SSO_OKTA_DOMAIN: string
    readonly VITE_SSO_CLIENT_ID: string
    readonly VITE_SSO_AUTH_ENDPOINT: string
    readonly VITE_SSO_TOKEN_ENDPOINT: string
  }
  
  interface ImportMeta {
    readonly env: ImportMetaEnv
  }