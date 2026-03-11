import axios from 'axios'
import type { AxiosResponse } from 'axios'
import {toSnakeCase, toCamelCase} from '@/utils/index'
// import { v4 as uuidv4 } from 'uuid'

const SUCCESS_CODE = 2000

// Main API request instance (for Lens API)
const request = axios.create({
  baseURL: `${import.meta.env.BASE_URL}v1`,  // Uses Vite base config to auto-adapt paths
  timeout: 30000,  // increased timeout to 30s for large data requests
  withCredentials: true,
})

request.interceptors.request.use(config => {
//   config.headers['X-Trace-Id'] = uuidv4()  //todo addTrace
  if (config.params) {
    config.params = toSnakeCase(config.params)
  }
  if (config.data) {
    config.data = toSnakeCase(config.data)
  }

  return config
})

request.interceptors.response.use(
  (response: AxiosResponse) => {
    const { data } = response
    // Support two response formats
    // 1. Meta-wrapped format: { meta: { code: 200 }, data: {...} }
    if (data.meta?.code === SUCCESS_CODE) {
      return toCamelCase(data.data)
    } else if (data.meta?.code) {
      // Has meta but code is not 2000
      const errorMessage = data.meta?.message || `API Error`
      const error = new Error(errorMessage)
      // Attach error code to error object for later handling
      ;(error as any).code = data.meta.code
      ;(error as any).fullMessage = `${errorMessage} (code: ${data.meta.code})`
      return Promise.reject(error)
    }
    // 2. Direct data format (e.g. weekly-reports)
    else {
      return toCamelCase(data)
    }
  },
  async error => {
    // Handle 401 Unauthorized
    if (error?.response?.status === 401) {
      const currentPath = window.location.pathname
      const currentSearch = window.location.search
      
      // Avoid handling 401 during login process:
      // 1. If on login-related pages, skip 401 handling
      // 2. If during SSO callback (with code parameter), skip 401 handling
      const isAuthPage = currentPath.includes('/login') || 
                        currentPath.includes('/sso') || 
                        currentPath.includes('/sso-bridge')
      const hasAuthCode = currentSearch.includes('code=')
      
      // Check if login is in progress
      const { useUserStore } = await import('@/stores/user')
      const userStore = useUserStore()
      const isLoggingIn = sessionStorage.getItem('is_logging_in') === 'true'
      
      // If on auth page or login is in progress, skip 401 handling
      if (isAuthPage || hasAuthCode || isLoggingIn) {
        return Promise.reject(error)
      }
      
      // Only clear session and redirect when re-login is truly needed
      try {
        // @ts-ignore - logout method exists, type definition issue
        await userStore.logout?.()
      } catch {}
      
      // Calculate relative path (strip base path)
      const basePath = import.meta.env.BASE_URL || '/'
      let redirect = currentPath
      if (redirect.startsWith(basePath)) {
        redirect = redirect.slice(basePath.length - 1) // keep leading /
      }
      // Preserve query parameters
      if (currentSearch && !hasAuthCode) {
        redirect = redirect + currentSearch
      }
      window.location.href = `${basePath}login?redirect=${encodeURIComponent(redirect)}`
      
      return Promise.reject(error)
    }
    
    return Promise.reject(error.message || 'Network Error')
  }
)

export default request
