import type { Router } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// A sign-in started by another app on this host returns there with a full-page navigation.
// getSafeRedirect and outsideConsoleReturn are the real ones here.

const getEnvs = vi.fn()
const ssoLoginRaw = vi.fn()
let userState: Record<string, unknown> = {}

vi.mock('element-plus', () => ({ ElMessage: {}, ElMessageBox: {} }))
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
  ensureSessionOnce: vi.fn(),
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

  it('loads the allow-listed app page', async () => {
    const next = await login('/dispatron/runs')
    expect(replace).toHaveBeenCalledWith(`${SELF}/dispatron/runs`)
    expect(next).toHaveBeenCalledWith(false)
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
