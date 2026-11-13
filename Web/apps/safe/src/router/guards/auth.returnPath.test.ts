import type { Router } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// A sign-in started by another app on this host returns there with a full-page navigation.
// getSafeRedirect and outsideConsoleReturn are the real ones here.

const getEnvs = vi.fn()
const ssoLoginRaw = vi.fn()
let userState: Record<string, unknown> = {}

const warning = vi.fn()
vi.mock('element-plus', () => ({
  ElMessage: { warning: (...a: unknown[]) => warning(...a) },
  ElMessageBox: {},
}))
vi.mock('@/stores/user', () => ({ useUserStore: () => userState }))
vi.mock('@/stores/cluster', () => ({ useClusterStore: () => ({ $reset: vi.fn() }) }))
vi.mock('@/stores/workspace', () => ({ useWorkspaceStore: () => ({ $reset: vi.fn() }) }))
vi.mock('@/services/login', () => ({
  getEnvs: () => getEnvs(),
  ssoLoginRaw: (...a: unknown[]) => ssoLoginRaw(...a),
}))
vi.mock('@/services/mcp', () => ({ addUserToGroup: vi.fn(), getUserNTID: vi.fn() }))

import setupAuthGuard, { ENVS_WAIT_MS } from './auth'

const SELF = 'https://console.example.com'
const ENTRY_KEY = 'sso.redirect'

let session: Map<string, string>
let replace: ReturnType<typeof vi.fn>
let replaceState: ReturnType<typeof vi.fn>

type Guard = (to: unknown, from: unknown, next: ReturnType<typeof vi.fn>) => Promise<void>

async function run(to: Record<string, unknown>) {
  let guard!: Guard
  setupAuthGuard({
    beforeEach: (fn: Guard) => {
      guard = fn
    },
  } as unknown as Router)
  const next = vi.fn()
  await guard(to, {}, next)
  return next
}

// The callback of a sign-in this tab started (state matches, no PKCE): both the console's
// OIDC sign-in and the older direct one finish here.
const callback = () =>
  run({
    query: { code: 'CODE', state: 'st' },
    path: '/sso/callback',
    fullPath: '/sso/callback?code=CODE&state=st',
    name: 'SSOCallback',
  })

const signedIn = (extra: Record<string, unknown> = {}) => ({
  session: 'authenticated',
  userId: 'u1',
  isLogin: true,
  ensureSessionOnce: vi.fn(async () => {}),
  $patch: vi.fn(),
  fetchUser: vi.fn(async () => {}),
  shouldAutoShowQuickStart: false,
  shouldAutoShowUserQuickStart: false,
  ...extra,
})

beforeEach(() => {
  getEnvs.mockReset().mockResolvedValue({})
  ssoLoginRaw.mockReset().mockResolvedValue({ status: 200, data: { id: 'u1' } })
  session = new Map([['oauth_state', 'st']])
  replace = vi.fn()
  replaceState = vi.fn()
  vi.stubGlobal('window', { location: { origin: SELF, href: `${SELF}/sso/callback`, replace } })
  vi.stubGlobal('sessionStorage', {
    getItem: (k: string) => session.get(k) ?? null,
    setItem: (k: string, v: string) => session.set(k, v),
    removeItem: (k: string) => session.delete(k),
  })
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() })
  vi.stubGlobal('history', { replaceState })
  userState = signedIn()
  warning.mockReset()
})

describe('sign-in callback returning outside the console', () => {
  it.each([
    ['/dispatron/runs/42?tab=log', `${SELF}/dispatron/runs/42?tab=log`],
    ['/autopilot/', `${SELF}/autopilot/`],
    ['/blog-drafts/', `${SELF}/blog-drafts/`],
    ['/binding/sign-in', `${SELF}/binding/sign-in`],
  ])('loads %s with a full-page navigation', async (entry, want) => {
    session.set(ENTRY_KEY, entry)
    const next = await callback()
    expect(ssoLoginRaw).toHaveBeenCalledTimes(1)
    expect(replace).toHaveBeenCalledWith(want)
    expect(next).toHaveBeenCalledWith(false)
    expect(replaceState).not.toHaveBeenCalled()
    expect(session.has(ENTRY_KEY)).toBe(false)
  })

  it('goes there before the first-login onboarding', async () => {
    userState = signedIn({ shouldAutoShowQuickStart: true })
    session.set(ENTRY_KEY, '/autopilot/')
    const next = await callback()
    expect(replace).toHaveBeenCalledWith(`${SELF}/autopilot/`)
    expect(next).toHaveBeenCalledWith(false)
  })

  it('keeps a console target in the router', async () => {
    session.set(ENTRY_KEY, '/workloads?x=1')
    const next = await callback()
    expect(replace).not.toHaveBeenCalled()
    expect(replaceState).toHaveBeenCalledWith(null, '', '/workloads?x=1')
    expect(next).toHaveBeenCalledWith('/workloads?x=1')
  })

  it.each([
    '//evil.example.net/dispatron',
    '/\\evil.example.net/dispatron',
    '/\t/evil.example.net',
    'https://evil.example.net/dispatron',
  ])('loads / of this origin instead of %j', async (entry) => {
    session.set(ENTRY_KEY, entry)
    const next = await callback()
    expect(replace).toHaveBeenCalledWith('/')
    expect(next).toHaveBeenCalledWith(false)
  })

  it.each(['/dispatron/%2f%2fevil.example.net', '/dispatron/../login', '/dispatronx'])(
    'never leaves the console router for %j',
    async (entry) => {
      session.set(ENTRY_KEY, entry)
      const next = await callback()
      expect(replace).not.toHaveBeenCalled()
      const target = next.mock.calls[0][0]
      expect(typeof target).toBe('string')
      // whatever the router gets is a same-origin path
      expect(new URL(target, SELF).origin).toBe(SELF)
    },
  )

  it('stays in the console when the app sends the browser straight back', async () => {
    session.set(ENTRY_KEY, '/dispatron/runs')
    await callback()
    expect(replace).toHaveBeenCalledTimes(1)

    replace.mockClear()
    replaceState.mockClear()
    session.set('oauth_state', 'st')
    session.set(ENTRY_KEY, '/dispatron/runs')
    const next = await callback()
    expect(replace).not.toHaveBeenCalled()
    expect(warning).toHaveBeenCalledTimes(1)
    expect(replaceState).toHaveBeenCalledWith(null, '', '/')
    expect(next).toHaveBeenCalledWith('/')
  })

  it('does not navigate away when the sign-in fails', async () => {
    ssoLoginRaw.mockResolvedValue({ status: 401, data: {} })
    session.set(ENTRY_KEY, '/dispatron/')
    const next = await callback()
    expect(replace).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith(expect.objectContaining({ path: '/sso-error' }))
  })
})

// The console's nginx may redirect the bare / of the host (web.root_redirect), so a sign-in with
// nowhere to return to loads / instead of routing to it.
describe('sign-in callback with no page to return to', () => {
  it('loads / with a full-page navigation', async () => {
    const next = await callback()
    expect(replaceState).toHaveBeenCalledWith(null, '', '/')
    expect(replace).toHaveBeenCalledWith('/')
    expect(next).toHaveBeenCalledWith(false)
  })

  it('still starts the first-login onboarding in the router', async () => {
    userState = signedIn({ shouldAutoShowQuickStart: true })
    const next = await callback()
    expect(replace).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith({ path: '/quickstart', query: { next: '/' } })
  })
})

describe('/login with a session already present', () => {
  const login = (redirect: unknown) =>
    run({ query: { redirect }, path: '/login', fullPath: '/login', name: 'Login' })

  it('loads the allow-listed app page once the server confirms the session', async () => {
    const order: string[] = []
    userState = signedIn({
      ensureSessionOnce: vi.fn(async () => {
        await Promise.resolve()
        order.push('probe')
      }),
    })
    replace.mockImplementation(() => order.push('replace'))
    const next = await login('/dispatron/runs')
    expect(replace).toHaveBeenCalledWith(`${SELF}/dispatron/runs`)
    expect(order).toEqual(['probe', 'replace'])
    expect(next).toHaveBeenCalledWith(false)
  })

  it('loads blog-drafts once the server confirms the session', async () => {
    const next = await login('/blog-drafts/')
    expect(replace).toHaveBeenCalledWith(`${SELF}/blog-drafts/`)
    expect(next).toHaveBeenCalledWith(false)
  })

  // localStorage still says 'authenticated', but the session cookie has expired: the server
  // refuses it and ensureSessionOnce clears the saved session, as the real store does.
  const stale = () =>
    signedIn({
      ensureSessionOnce: vi.fn(async () => {
        Object.assign(userState, {
          session: 'anonymous',
          userId: '',
          isLogin: false,
          profile: null,
        })
      }),
    })

  it('signs in instead when the saved session is stale', async () => {
    userState = stale()
    const next = await login('/dispatron/runs')
    expect(userState.ensureSessionOnce).toHaveBeenCalledTimes(1)
    expect(replace).not.toHaveBeenCalled()
    // the Login page renders, which starts the sign-in with the same redirect
    expect(next).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledWith()
  })

  it('signs in instead when the session probe fails', async () => {
    userState = signedIn({
      ensureSessionOnce: vi.fn(async () => {
        throw new Error('network')
      }),
    })
    const next = await login('/dispatron/runs')
    expect(replace).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith()
  })

  it('a stale session never reaches the app however often it comes back', async () => {
    for (let i = 0; i < 3; i++) {
      userState = stale()
      await login('/dispatron/runs')
    }
    expect(replace).not.toHaveBeenCalled()
  })

  it('stays in the console when the app sends the browser straight back', async () => {
    await login('/dispatron/runs')
    replace.mockClear()
    const next = await login('/dispatron/runs')
    expect(replace).not.toHaveBeenCalled()
    expect(warning).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledWith('/')
  })

  it('a console target does not need the probe', async () => {
    const next = await login('/workloads')
    expect(userState.ensureSessionOnce).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith('/workloads')
  })

  it.each(['//evil.example.net', '/\\evil.example.net', 'https://evil.example.net/autopilot'])(
    'keeps %j in the console',
    async (redirect) => {
      const next = await login(redirect)
      expect(replace).not.toHaveBeenCalled()
      expect(next).toHaveBeenCalledWith('/')
    },
  )

  it('keeps a console target in the router', async () => {
    const next = await login('/workloads')
    expect(replace).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith('/workloads')
  })
})

// The prefixes come from /envs (consoleReturnPrefixes); the console's defaults apply when it says
// nothing, cannot be read, or is too slow.
describe('returning to a prefix the deployment configured', () => {
  const CLAW_LINK = '/claw-api/v1/sessions/s-1/deliveries/0f8fad5b-d9cb-469f-a165-70867728950e'
  const CONFIGURED = {
    consoleReturnPrefixes: ['/dispatron', '/autopilot', '/binding', '/blog-drafts', '/claw-api'],
  }
  const login = (redirect: unknown) =>
    run({ query: { redirect }, path: '/login', fullPath: '/login', name: 'Login' })

  it('the sign-in callback loads a configured prefix', async () => {
    getEnvs.mockResolvedValue(CONFIGURED)
    session.set(ENTRY_KEY, CLAW_LINK)
    const next = await callback()
    expect(getEnvs).toHaveBeenCalledTimes(1)
    expect(replace).toHaveBeenCalledWith(`${SELF}${CLAW_LINK}`)
    expect(next).toHaveBeenCalledWith(false)
  })

  it('the sign-in callback keeps an unconfigured prefix in the console', async () => {
    getEnvs.mockResolvedValue({ consoleReturnPrefixes: ['/dispatron'] })
    session.set(ENTRY_KEY, '/autopilot/')
    const next = await callback()
    expect(replace).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith('/autopilot/')
  })

  it('the sign-in callback ignores invalid configured entries', async () => {
    getEnvs.mockResolvedValue({ consoleReturnPrefixes: ['/', '//evil.example.net', '/claw-api/'] })
    session.set(ENTRY_KEY, CLAW_LINK)
    await callback()
    expect(replace).not.toHaveBeenCalled()
  })

  it('the sign-in callback falls back to the defaults when /envs fails', async () => {
    getEnvs.mockRejectedValue(new Error('down'))
    session.set(ENTRY_KEY, '/dispatron/runs')
    const next = await callback()
    expect(replace).toHaveBeenCalledWith(`${SELF}/dispatron/runs`)
    expect(next).toHaveBeenCalledWith(false)
  })

  it('/login with a session loads a configured prefix', async () => {
    getEnvs.mockResolvedValue(CONFIGURED)
    const next = await login(CLAW_LINK)
    expect(replace).toHaveBeenCalledWith(`${SELF}${CLAW_LINK}`)
    expect(next).toHaveBeenCalledWith(false)
  })

  it('/login with a session uses the /envs the store already holds', async () => {
    userState = signedIn({ envs: CONFIGURED })
    const next = await login(CLAW_LINK)
    expect(getEnvs).not.toHaveBeenCalled()
    expect(replace).toHaveBeenCalledWith(`${SELF}${CLAW_LINK}`)
    expect(next).toHaveBeenCalledWith(false)
  })

  it('/login with a session keeps an unconfigured prefix in the console', async () => {
    const next = await login(CLAW_LINK)
    expect(replace).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith(CLAW_LINK)
  })

  it('/login with a session falls back to the defaults when /envs fails', async () => {
    getEnvs.mockRejectedValue(new Error('down'))
    const next = await login('/dispatron/runs')
    expect(replace).toHaveBeenCalledWith(`${SELF}/dispatron/runs`)
    expect(next).toHaveBeenCalledWith(false)
  })

  it('/login with a session does not wait for a slow /envs past ENVS_WAIT_MS', async () => {
    vi.useFakeTimers()
    try {
      getEnvs.mockReturnValue(new Promise(() => {}))
      const pending = login('/dispatron/runs')
      await vi.advanceTimersByTimeAsync(ENVS_WAIT_MS)
      const next = await pending
      expect(replace).toHaveBeenCalledWith(`${SELF}/dispatron/runs`)
      expect(next).toHaveBeenCalledWith(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('stays in the console when a configured app sends the browser straight back', async () => {
    getEnvs.mockResolvedValue(CONFIGURED)
    await login(CLAW_LINK)
    replace.mockClear()
    const next = await login(CLAW_LINK)
    expect(replace).not.toHaveBeenCalled()
    expect(warning).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledWith('/')
  })
})
