import type { Router } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserStore } from '@/stores/user'
import { ssoLoginRaw } from '@/services/login'
import { addUserToGroup, getUserNTID } from '@/services/mcp'

const SSO_GROUP = 'dl.primus-safe-users'
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

export default function setupAuthGuard(router: Router) {
  router.beforeEach(async (to, _from, next) => {
    const user = useUserStore()

    // A) If any route carries ?code, handle SSO login first
    const code = to.query.code as string | undefined
    if (code) {
      const state = to.query.state as string | undefined

      // Check if this is an SSO request from the Lens app
      if (state && state.startsWith('lens:')) {
        // Parse the Lens callback URL from the state
        const parts = state.split(':')
        if (parts.length >= 3) {
          const lensRedirect = decodeURIComponent(parts.slice(2).join(':'))
          // Forward code and state to Lens
          window.location.href = `${lensRedirect}?code=${code}&state=${encodeURIComponent(state)}`
          return
        }
      }

      // Check if this is an SSO request from the HyperLoom app
      if (state && state.startsWith('hyperloom:')) {
        const parts = state.split(':')
        if (parts.length >= 3) {
          const hlRedirect = decodeURIComponent(parts.slice(2).join(':'))
          window.location.href = `${hlRedirect}?code=${code}&state=${encodeURIComponent(state)}`
          return
        }
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

          const target = sessionStorage.getItem(ENTRY_KEY) || '/'
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

      // ❌ Login failed — try auto-joining the user group
      const userEmail =
        resp.data?.email || resp.data?.name || resp.data?.user?.email || resp.data?.user?.name

      if (userEmail) {
        try {
          await addUserToGroup(userEmail, SSO_GROUP)
          ElMessage.success('You have been added to the user group. Retrying SSO login...')

          // Clear state and re-trigger SSO login
          sessionStorage.removeItem(OAUTH_STATE_KEY)
          localStorage.removeItem(SSO_ATTEMPTS_KEY)
          sessionStorage.removeItem('sso.blocked')
          history.replaceState(null, '', to.path)

          // Preserve original redirect target
          const redirect = sessionStorage.getItem(ENTRY_KEY) || to.fullPath || '/'
          sessionStorage.setItem(ENTRY_KEY, redirect)

          return next({ path: '/login', query: { redirect } })
        } catch (addErr: any) {
          console.warn('[SSO Auth] Failed to auto-add user to group:', addErr?.message)
        }
      }

      // Fallback: redirect to SSO error page
      sessionStorage.removeItem(ENTRY_KEY)
      localStorage.removeItem(SSO_ATTEMPTS_KEY)
      sessionStorage.removeItem('sso.blocked')
      history.replaceState(null, '', to.path)
      const errMsg = resp.data?.errorMessage || resp.data?.message || 'SSO authentication failed.'
      console.error('[SSO Auth] Login failed, no auto-join possible:', errMsg)
      return next({
        path: '/sso-error',
        query: {
          error: 'authentication_failed',
          error_description: errMsg,
        },
      })
    }

    // B) Error callback — Okta returned an error parameter
    if (to.query.error && to.path !== '/sso-error') {
      localStorage.removeItem(SSO_ATTEMPTS_KEY)
      sessionStorage.removeItem('sso.blocked')
      history.replaceState(null, '', to.path)
      // access_denied: user is not in the group — try auto-joining
      if (to.query.error === 'access_denied') {
        try {
          const result = await ElMessageBox.prompt(
            'You don\'t have access yet. Enter your NTID or AMD email and we\'ll add you to the user group.\n\nNote: It may take up to 2 hours for Okta to sync after joining.',
            'Request Access',
            {
              confirmButtonText: 'Join',
              cancelButtonText: 'Cancel',
              inputPlaceholder: 'NTID or email',
              inputValidator: (v) => (v && v.trim().length > 0) || 'Please enter your NTID or email',
            },
          )
          const identifier = (result as any).value as string | undefined

          if (identifier) {
            const userIdentifier = identifier.trim()

            ElMessage.info('Verifying your identity...')
            const ntidResult = await getUserNTID(userIdentifier)
            if (typeof ntidResult === 'string' && ntidResult.startsWith('Error')) {
              ElMessage.error('User not found in Active Directory. Please check your NTID or email.')
              // Don't redirect; let the catch block below handle the sso-error page
              throw new Error(ntidResult)
            }
            ElMessage.info('Adding you to the user group...')
            await addUserToGroup(userIdentifier, SSO_GROUP)
            // Redirect to a friendly waiting page
            return next({
              path: '/sso-error',
              query: {
                error: 'joined',
                error_description: userIdentifier,
              },
            })
          }
        } catch (err: any) {
          if (err !== 'cancel' && err !== 'close') {
            console.error('[SSO Auth] Failed to auto-add user:', err)
            ElMessage.error(`Failed to add user to group: ${err?.message || err}`)
          }
        }
      }

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
        return next((to.query.redirect as string) || '/')
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
