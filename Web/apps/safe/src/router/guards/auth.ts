import type { Router } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserStore } from '@/stores/user'
import { getEnvs, ssoLoginRaw } from '@/services/login'
import type { EnvsResp } from '@/services/login/type'
import { addUserToGroup, getUserNTID } from '@/services/mcp'
import { getSafeRedirect } from '@/utils'
import {
  leaveConsoleFor,
  outsideConsoleReturn,
  RETURN_REFUSED_MESSAGE,
  returnPrefixes,
} from '@/utils/returnPath'
import { resolveAllowedBridge } from '@/utils/ssoBridge'
import { pkceRequired, takeCallbackVerifier } from '@/utils/ssoSignIn'

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

/** Allow-listed bridges come from /envs; if it cannot be read only this origin is trusted. */
async function loadBridgeAllowlist(): Promise<string[]> {
  try {
    const envs = await getEnvs()
    return Array.isArray(envs?.ssoBridgeAllowlist) ? envs.ssoBridgeAllowlist : []
  } catch {
    return []
  }
}

/** /envs for the sign-in callback, or null if it cannot be read. */
async function loadEnvs(): Promise<EnvsResp | null> {
  try {
    return (await getEnvs()) ?? null
  } catch {
    return null
  }
}

/** How long /login waits for /envs (the allowed return prefixes) before using the defaults. */
export const ENVS_WAIT_MS = 3000

/** /envs within `ms`, or null when it is slower or cannot be read (no toast either way). */
async function envsWithin(ms: number): Promise<EnvsResp | null> {
  let timer: ReturnType<typeof setTimeout> | undefined
  const late = new Promise<null>((resolve) => {
    timer = setTimeout(() => resolve(null), ms)
  })
  try {
    return (await Promise.race([getEnvs(true).catch(() => null), late])) ?? null
  } finally {
    clearTimeout(timer)
  }
}

/**
 * Whether the server, not only localStorage, says this browser is signed in. The saved session
 * may be stale (the session cookie expired since); ensureSessionOnce asks the server (the probe
 * main.ts starts on load, or a new one after a 401 reset it) and clears the saved session when
 * the server refuses it.
 */
async function sessionConfirmed(user: ReturnType<typeof useUserStore>): Promise<boolean> {
  try {
    await user.ensureSessionOnce()
  } catch {
    return false
  }
  return !!(user.isLogin && user.userId)
}

export default function setupAuthGuard(router: Router) {
  router.beforeEach(async (to, _from, next) => {
    const user = useUserStore()

    // A) If any route carries ?code, handle SSO login first
    const code = to.query.code as string | undefined
    if (code) {
      const state = to.query.state as string | undefined

      // Check if this is an SSO request from another app.
      // State format: {app}:{csrfToken}:{encodeURIComponent(bridgeUrl)}
      const bridgeCandidate = getBridgeCandidateFromState(state)
      if (bridgeCandidate && state) {
        const bridge = resolveAllowedBridge(
          bridgeCandidate,
          await loadBridgeAllowlist(),
          window.location.origin,
        )
        if (!bridge) {
          // Never hand the code to a target that is not allow-listed.
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

      // With PKCE the code is used only with the verifier this tab kept for this state; whether
      // the sign-in uses PKCE (ssoPkce) is false if /envs cannot be read.
      const envs = await loadEnvs()
      const pkce = takeCallbackVerifier(state, pkceRequired(envs))
      if (!pkce.ok) {
        history.replaceState(null, '', to.path)
        return next({
          path: '/sso-error',
          query: {
            error: 'state_mismatch',
            error_description: 'The sign-in did not start in this tab. Please try again.',
          },
        })
      }

      // ========== SSO login: use raw request to get the full response ==========
      const resp = await ssoLoginRaw(code, state, pkce.verifier)

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

          const entry = sessionStorage.getItem(ENTRY_KEY)
          sessionStorage.removeItem(ENTRY_KEY)

          // The sign-in was started by another app on this host (Dispatron, autopilot, the
          // identity service): load its page, which the console router does not know.
          const away = outsideConsoleReturn(
            entry,
            window.location.origin,
            returnPrefixes(envs ?? user.envs),
          )
          if (away) {
            if (leaveConsoleFor(away)) return next(false)
            ElMessage.warning(RETURN_REFUSED_MESSAGE)
          }

          const target = away ? '/' : getSafeRedirect(entry)
          history.replaceState(null, '', target)

          // First-login onboarding: admin → QuickStart, regular user → UserQuickStart
          if (user.shouldAutoShowQuickStart) {
            return next({ path: '/quickstart', query: { next: target } })
          }
          if (user.shouldAutoShowUserQuickStart) {
            return next({ path: '/userquickstart', query: { next: target } })
          }
          // No page to return to: load / itself, which the console's nginx may send elsewhere.
          if (!away && target === '/') {
            window.location.replace('/')
            return next(false)
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

    // C) Session probe. Not only while the session is unknown: a visitor's saved session is
    // 'anonymous', yet an identity provider may have signed them in since (the console's
    // /login redirects there and comes back), so with no userId wait for the probe too. It runs
    // once; later calls get the same answer.
    if (user.session === 'unknown' || !user.userId) {
      try {
        await user.ensureSessionOnce()
      } catch {}
    }

    // D) Already logged in — skip login-related pages
    if (user.isLogin && user.userId) {
      if (!PUBLIC_NAME.has(to.name as string)) return next()

      // The allowed prefixes come from /envs; a fresh page load may not have it yet.
      const envs = user.envs ?? (to.query.redirect ? await envsWithin(ENVS_WAIT_MS) : null)
      const away = outsideConsoleReturn(
        to.query.redirect,
        window.location.origin,
        returnPrefixes(envs),
      )
      if (!away) return next(getSafeRedirect(to.query.redirect))

      // Leaving the console on a session only localStorage vouches for would send a browser
      // whose cookie expired back to the app, which sends it here again: a loop with no sign-in.
      // Ask the server first; without a session, fall through to the sign-in below.
      if (await sessionConfirmed(user)) {
        if (leaveConsoleFor(away)) return next(false)
        ElMessage.warning(RETURN_REFUSED_MESSAGE)
        return next('/')
      }
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
