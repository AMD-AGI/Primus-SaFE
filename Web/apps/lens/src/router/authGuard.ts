import type { Router } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/stores/user'

const PUBLIC_NAME = new Set(['Login', 'Register', 'SSOBridge'])
const ENTRY_KEY = 'sso.redirect'
const OAUTH_STATE_KEY = 'oauth_state'

function isPublicRoute(to: any) {
  // Check by name first
  if (to.name && PUBLIC_NAME.has(to.name as string)) return true

  // Fallback: check by path, handling trailing slashes
  const cleanPath = (to.path || '/').replace(/\/+$/, '') || '/'
  return cleanPath === '/login' || cleanPath === '/register' || cleanPath === '/sso-bridge'
}

export default function setupAuthGuard(router: Router) {
  router.beforeEach(async (to, _from, next) => {
    // If navigating from root and not actually intending to visit root, try to restore actual path
    if (_from.path === '/' && to.path === '/statistics/cluster' && !to.query.from_redirect) {
      const actualPath = window.location.pathname.replace('/lens', '') || '/'
      const actualSearch = window.location.search
      if (actualPath !== '/' && actualPath !== to.path) {
        return next({ path: actualPath, query: Object.fromEntries(new URLSearchParams(actualSearch)) })
      }
    }


    const user = useUserStore()

    // A) Any route with ?code: handle SSO login first
    const code = to.query.code as string | undefined
    if (code) {
      // Check if this code has already been processed (prevent duplicate submission to backend)
      const processedKey = `sso_processed_${code}`
      if (sessionStorage.getItem(processedKey)) {
        console.warn('[AuthGuard] Code already processed, cleaning up URL:', code)
        // Construct clean path without code/state
        const cleanPath = to.path === '/sso-bridge' ? '/' : to.path
        history.replaceState(null, '', cleanPath)
        return next({ path: cleanPath, replace: true })
      }

      sessionStorage.setItem(processedKey, 'true')

      const stateParam = to.query.state as string | undefined

      // Parse state, extract original state value
      let actualState = stateParam
      if (stateParam && stateParam.startsWith('lens:')) {
        const parts = stateParam.split(':')
        if (parts.length >= 2) {
          actualState = parts[1] // get original state
        }
      }

      // Verify state matches
      const saved = sessionStorage.getItem(OAUTH_STATE_KEY)
      if (saved && actualState !== saved) {
        console.error('State mismatch:', { saved, actualState, stateParam })
        ElMessage.error('State mismatch - possible security issue')
        history.replaceState(null, '', to.path)
        return next({ path: '/login', replace: true })
      }

      try {
        // Call login with SSO type - using extracted original state
        await user.login({
          code: code,
          state: actualState || '',
          type: 'sso'
        })

        // Ensure session is established
        await user.ensureSessionOnce()

        // Verify login truly succeeded (prevent case where backend returns 200 but cookie/token is empty)
        if (!user.isLogin || !user.userId) {
          console.error('[AuthGuard] Login API returned 200 but session not established')
          sessionStorage.removeItem(OAUTH_STATE_KEY)
          sessionStorage.removeItem(ENTRY_KEY)
          sessionStorage.removeItem('is_logging_in')
          history.replaceState(null, '', to.path)
          ElMessage.error('Login succeeded but session could not be established. Please try again.')
          return next({ path: '/login', replace: true })
        }

        // Clean up
        sessionStorage.removeItem(OAUTH_STATE_KEY)

        // Get redirect target — key fix: cannot use to.fullPath, otherwise code/state would be carried into target causing reprocessing
        let target: string | null = null
        if (to.path !== '/login' && to.path !== '/sso-bridge' && to.path !== '/') {
          // Keep only path, strip code/state and other SSO-related query params
          const cleanQuery = { ...to.query }
          delete cleanQuery.code
          delete cleanQuery.state
          const qs = new URLSearchParams(cleanQuery as Record<string, string>).toString()
          target = qs ? `${to.path}?${qs}` : to.path
        }
        if (!target) {
          target = sessionStorage.getItem(ENTRY_KEY) || '/'
        }
        sessionStorage.removeItem(ENTRY_KEY)

        sessionStorage.removeItem('is_logging_in')

        // Clean up code/state from browser address bar
        history.replaceState(null, '', target)
        return next({ path: target, replace: true })
      } catch (error: any) {
        console.error('SSO login failed:', error)

        // Clear login-in-progress flag
        sessionStorage.removeItem('is_logging_in')

        // Check for specific error types
        const errorMessage = error?.response?.data?.message || error?.message || 'SSO login failed'
        ElMessage.error(errorMessage)

        // Clean code and state params from URL
        history.replaceState(null, '', to.path)

        // Redirect to login page without any params to avoid loops
        return next({ path: '/login', replace: true })
      }
    }

    // B) Error redirect
    if (to.query.error) {
      ElMessage.error(
        decodeURIComponent((to.query.error_description as string) || (to.query.error as string))
      )
      history.replaceState(null, '', to.path)
      return next('/login')
    }

    // C) Session detection
    if (user.session === 'unknown') {
      try {
        await user.ensureSessionOnce()
      } catch (error) {
        console.error('[AuthGuard] Session check failed:', error)
        // Session check failed, continue as anonymous
      }
    }

    // D) Already logged in: skip login-related pages
    if (user.isLogin && user.userId) {
      if (PUBLIC_NAME.has(to.name as string)) {
        return next((to.query.redirect as string) || '/')
      }
      return next()
    }

    // E) Not logged in: non-public routes -> redirect to /login (auto SSO entry)
    if (!isPublicRoute(to)) {
      // Preserve full path and query parameters
      const redirect = to.fullPath !== '/' ? to.fullPath : '/'
      return next({ path: '/login', query: { redirect } })
    }

    // Allow public routes directly
    return next()
  })
}
