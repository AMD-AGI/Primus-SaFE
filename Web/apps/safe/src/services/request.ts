import { useClusterStore } from '@/stores/cluster'
import axios from 'axios'
import type { AxiosResponse, AxiosRequestConfig, AxiosInstance } from 'axios'
import { ElMessage } from 'element-plus'

declare module 'axios' {
  export interface AxiosRequestConfig {
    rawResponse?: boolean
    /** When true the global interceptor will NOT show ElMessage and will reject with the raw error object */
    skipErrorHandler?: boolean
  }
}

/** ---- Flag to prevent concurrent 401 handling ---- **/
let isHandling401 = false

/** ---- Attach interceptors to an axios instance ---- **/
function attachInterceptors(instance: AxiosInstance) {
  instance.interceptors.request.use((config) => config)

  instance.interceptors.response.use(
    (response: AxiosResponse) => {
      if (response.config.rawResponse === true) return response

      const rawData = response.data
      const hasBizErr =
        (rawData && typeof rawData === 'object' && 'errorCode' in rawData) ||
        (rawData && typeof rawData === 'object' && 'code' in rawData && rawData.code !== 0)

      if (hasBizErr) {
        const code = rawData.errorCode ?? rawData.code
        const msg = rawData.errorMessage ?? rawData.message ?? 'API Error'
        if (response.config.skipErrorHandler) {
          const bizErr = new Error(msg) as any
          bizErr.errorCode = code
          bizErr.errorMessage = msg
          return Promise.reject(bizErr)
        }
        ElMessage({ type: 'error', message: `${code ?? ''} ${msg}`.trim() })
        return Promise.reject(msg)
      }
      return rawData
    },
    async (error) => {
      if (error?.config?.skipErrorHandler) {
        return Promise.reject(error)
      }

      const status = error?.response?.status as number | undefined

      if (status === 401) {
        // Prevent duplicate 401 handling from concurrent requests (race condition)
        if (isHandling401) {
          return Promise.reject(error)
        }

        const pathname = location.pathname

        if (pathname === '/login' || pathname === '/login-admin' || pathname === '/register') {
          ElMessage({
            type: 'error',
            message: error?.response?.data?.errorMessage || 'Unauthorized',
          })
          return Promise.reject(error)
        }

        isHandling401 = true

        try {
          localStorage.removeItem('user')
        } catch {}

        try {
          const { useClusterStore } = await import('@/stores/cluster')
          const { useWorkspaceStore } = await import('@/stores/workspace')
          const { useUserStore } = await import('@/stores/user')
          useClusterStore().$reset()
          useWorkspaceStore().$reset()
          // Clear user in-memory state so router guards stop granting access
          const userStore = useUserStore()
          userStore.$patch({
            session: 'anonymous',
            userId: '',
            profile: null,
            _profileFetched: false,
            _initPromise: null,
          })
        } catch {}

        const from = encodeURIComponent(pathname + location.search)
        location.href = `/login?redirect=${from}`

        return Promise.reject(error)
      }

      const pathname = location.pathname
      const isLoginPage =
        pathname === '/login' || pathname === '/login-admin' || pathname === '/register'

      let errorMsg = 'Request failed.'

      if (error?.response?.data?.errorMessage) {
        errorMsg = `${error.response.data.errorCode ?? error.response.data.code ?? ''}: ${
          error.response.data.errorMessage || error.response.data.message || ''
        }`.trim()
      } else if (error?.response?.status >= 500) {
        // 500 on login pages may indicate an auth state issue
        errorMsg = isLoginPage
          ? 'Request failed. Please try refreshing the page.'
          : 'Server error. Please try again later.'
      }

      ElMessage({
        type: 'error',
        message: errorMsg,
      })
      return Promise.reject(error.message || 'Request failed.')
    },
  )
}

export const request = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api',
  timeout: 10000,
  withCredentials: true,
})
attachInterceptors(request)

function buildQuery(params?: Record<string, any>) {
  const usp = new URLSearchParams()
  if (!params) return ''
  for (const [k, v] of Object.entries(params)) {
    if (v == null) continue
    if (Array.isArray(v)) v.forEach((i) => usp.append(k, String(i)))
    else if (v instanceof Date) usp.append(k, v.toISOString())
    else usp.append(k, String(v))
  }
  return usp.toString()
}

const lensRequest = axios.create({
  baseURL: import.meta.env.VITE_LENS_BASE_URL || '/lens/v1',
  timeout: 10000,
  withCredentials: true,
})

// Manually serialize params to the URL and clear config.params to bypass any case-transformers
lensRequest.interceptors.request.use((config) => {
  const qs = buildQuery(config.params as any)
  if (qs) {
    const url = config.url || ''
    config.url = url + (url.includes('?') ? '&' : '?') + qs
  }
  config.params = undefined
  return config
})

attachInterceptors(lensRequest)

// Root Cause Analysis Request
const rootCauseRequest = axios.create({
  baseURL: import.meta.env.VITE_ROOT_CAUSE_BASE_URL || '/root-cause-skills',
  timeout: 60000, // Root cause analysis may need longer timeout
  withCredentials: true,
})
attachInterceptors(rootCauseRequest)

export function postForm<TData = any>(
  url: string,
  data: Record<string, any>,
  config?: AxiosRequestConfig,
): Promise<TData> {
  const params = new URLSearchParams()
  Object.entries(data).forEach(([k, v]) => params.append(k, v == null ? '' : String(v)))
  return request.post<any, TData>(url, params, {
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    ...config,
  })
}

// PrimusClaw service — shared backend for tools / plugins / resources.
// Response envelope is `{ code, data }` (success: 200 ≤ code < 300), which the
// global `request` interceptor does not unwrap — so we keep a dedicated
// instance with its own unwrap + 401 redirect logic. Auth rides on the session
// cookie via `withCredentials: true`, same as the main `request` instance.
const clawRequest = axios.create({
  baseURL: '/claw-api/v1',
  timeout: 10000,
  withCredentials: true,
})

clawRequest.interceptors.response.use(
  (response: AxiosResponse) => {
    if (response.config.rawResponse === true) return response

    const rawData = response.data
    if (rawData && typeof rawData === 'object' && 'code' in rawData && rawData.code >= 200 && rawData.code < 300) {
      return rawData.data
    }

    const msg = rawData?.message ?? rawData?.error ?? 'Claw API Error'
    ElMessage({ type: 'error', message: msg })
    return Promise.reject(msg)
  },
  async (error) => {
    const status = error?.response?.status as number | undefined
    if (status === 401) {
      location.href = `/login?redirect=${encodeURIComponent(location.pathname + location.search)}`
      return Promise.reject(error)
    }
    ElMessage({ type: 'error', message: error?.response?.data?.message || 'Claw request failed.' })
    return Promise.reject(error)
  },
)

export default request
export { lensRequest, rootCauseRequest, clawRequest }
