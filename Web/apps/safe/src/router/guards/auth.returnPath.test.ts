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

import setupAuthGuard from './auth'

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
    '/dispatron/%2f%2fevil.example.net',
    '/dispatron/../login',
    '/dispatronx',
  ])('never leaves the console router for %j', async (entry) => {
    session.set(ENTRY_KEY, entry)
    const next = await callback()
    expect(replace).not.toHaveBeenCalled()
    const target = next.mock.calls[0][0]
    expect(typeof target).toBe('string')
    // whatever the router gets is a same-origin path
    expect(new URL(target, SELF).origin).toBe(SELF)
  })

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
