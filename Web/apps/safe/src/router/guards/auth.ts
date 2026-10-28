import type { Router } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { ssoLoginRaw } from '@/services/login'
import { getSafeRedirect } from '@/utils'
import { resolveOwnBridge } from '@/utils/ssoBridge'

const PUBLIC_NAME = new Set(['Login', 'LoginAdmin', 'Register', 'SSOError'])
const ENTRY_KEY = 'sso.redirect'
const OAUTH_STATE_KEY = 'oauth_state'
const SSO_ATTEMPTS_KEY = 'sso.attempts' // persisted in localStorage

function isPublicRoute(to: any) {
  // Check by route name first
  if (to.name && PUBLIC_NAME.has(to.name as string)) return true

  // Fallback: check by path, normalizing trailing slashes
  const cleanPath = (to.path || '/').replace(/\/+$/, '') || '/'
  return (
    cleanPath === '/login' ||
    cleanPath === '/login-admin' ||
    cleanPath === '/register' ||
    cleanPath === '/sso-error'
  )
}

/** Extracts the raw bridge URL carried in the OAuth state; '' when there is none. */
function getBridgeCandidateFromState(state?: string) {
  if (!state) return ''

  const parts = state.split(':')
  if (parts.length < 3) return ''

  try {
    return decodeURIComponent(parts.slice(2).join(':'))
  } catch {
    return ''
  }
}

export default function setupAuthGuard(router: Router) {
  router.beforeEach(async (to, _from, next) => {
    const user = useUserStore()

    // A) If any route carries ?code, handle SSO login first
    const code = to.query.code as string | undefined
    if (code) {
      const state = to.query.state as string | undefined

      // Check if this is an SSO request from an app served under this console's origin
      // (Lens). State format: {app}:{csrfToken}:{encodeURIComponent(bridgeUrl)}
      const bridgeCandidate = getBridgeCandidateFromState(state)
      if (bridgeCandidate && state) {
        const bridge = resolveOwnBridge(bridgeCandidate, window.location.origin)
        if (!bridge) {
          // Never hand the code to a target outside this console's origin.
          history.replaceState(null, '', to.path)
          return next({
            path: '/sso-error',
            query: {
              error: 'invalid_bridge',
              error_description: 'The sign-in request targets an application that is not allowed.',
            },
          })
        }
        bridge.searchParams.set('code', code)
        bridge.searchParams.set('state', state)
        bridge.hash = ''
        window.location.href = bridge.toString()
        return
      }

      const saved = sessionStorage.getItem(OAUTH_STATE_KEY)
      if (saved && state && saved !== state) {
        history.replaceState(null, '', to.path)
        return next({
          path: '/sso-error',
          query: {
            error: 'state_mismatch',
            error_description: 'The authentication state does not match. Please try again.',
          },
        })
      }

      // ========== SSO login: use raw request to get the full response ==========
      const resp = await ssoLoginRaw(code, state)

      const loginOk = resp.status >= 200 && resp.status < 300 && resp.data?.id

      if (loginOk) {
        // ✅ Login success — set state via user store
        try {
          user.$patch({ userId: resp.data.id })
          await user.fetchUser(true)
          user.$patch({ session: 'authenticated' })

          // Reset cluster & workspace stores
          const { useClusterStore } = await import('@/stores/cluster')
          const { useWorkspaceStore } = await import('@/stores/workspace')
          useClusterStore().$reset()
          useWorkspaceStore().$reset()

          sessionStorage.removeItem(OAUTH_STATE_KEY)
          localStorage.removeItem(SSO_ATTEMPTS_KEY)
          sessionStorage.removeItem('sso.blocked')

          const target = getSafeRedirect(sessionStorage.getItem(ENTRY_KEY))
          sessionStorage.removeItem(ENTRY_KEY)
          history.replaceState(null, '', target)

          // First-login onboarding: admin → QuickStart, regular user → UserQuickStart
          if (user.shouldAutoShowQuickStart) {
            return next({ path: '/quickstart', query: { next: target } })
          }
          if (user.shouldAutoShowUserQuickStart) {
            return next({ path: '/userquickstart', query: { next: target } })
          }
          return next(target)
        } catch (fetchErr: any) {
          console.warn('[SSO Auth] Login OK but fetchUser failed:', fetchErr?.message)
        }
      }

      // ❌ Login failed
      sessionStorage.removeItem(ENTRY_KEY)
      localStorage.removeItem(SSO_ATTEMPTS_KEY)
      sessionStorage.removeItem('sso.blocked')
      history.replaceState(null, '', to.path)
      const errMsg = resp.data?.errorMessage || resp.data?.message || 'SSO authentication failed.'
      console.error('[SSO Auth] Login failed:', errMsg)
      return next({
        path: '/sso-error',
        query: {
          error: 'authentication_failed',
          error_description: errMsg,
        },
      })
    }

    // B) Error callback — the identity provider returned an error parameter
    if (to.query.error && to.path !== '/sso-error') {
      localStorage.removeItem(SSO_ATTEMPTS_KEY)
      sessionStorage.removeItem('sso.blocked')
      history.replaceState(null, '', to.path)
      return next({
        path: '/sso-error',
        query: {
          error: to.query.error,
          error_description: to.query.error_description || to.query.error,
        },
      })
    }

    // C) Session probe
    if (user.session === 'unknown') {
      try {
        await user.ensureSessionOnce()
      } catch {}
    }

    // D) Already logged in — skip login-related pages
    if (user.isLogin && user.userId) {
      if (PUBLIC_NAME.has(to.name as string)) {
        return next(getSafeRedirect(to.query.redirect))
      }
      return next()
    }

    // E) Not logged in: non-public routes → redirect to /login (SSO entry point)
    if (!isPublicRoute(to)) {
      const redirect = to.fullPath || '/'
      return next({ path: '/login', query: { redirect } })
    }

    // Public routes pass through (including /login-admin)
    return next()
  })
}
