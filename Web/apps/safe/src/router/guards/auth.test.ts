import { beforeEach, describe, expect, it, vi } from 'vitest'

const getEnvs = vi.fn()
const ssoLoginRaw = vi.fn()

// The store the guard sees; a test replaces it to play a saved session.
let userState: Record<string, unknown> = { session: 'unknown' }

vi.mock('element-plus', () => ({ ElMessage: {}, ElMessageBox: {} }))
vi.mock('@/stores/user', () => ({ useUserStore: () => userState }))
vi.mock('@/services/login', () => ({ getEnvs: () => getEnvs(), ssoLoginRaw: (...a: any[]) => ssoLoginRaw(...a) }))
vi.mock('@/services/mcp', () => ({ addUserToGroup: vi.fn(), getUserNTID: vi.fn() }))
vi.mock('@/utils', () => ({ getSafeRedirect: (v: any) => v || '/' }))

import setupAuthGuard from './auth'

const SELF = 'https://console.example.com'

async function runGuard(state: string) {
  let guard: any
  setupAuthGuard({ beforeEach: (fn: any) => (guard = fn) } as any)
  const next = vi.fn()
  await guard({ query: { code: 'CODE123', state }, path: '/', fullPath: '/', name: 'Home' }, {}, next)
  return next
}

const stateFor = (bridge: string) => `claw:csrf:${encodeURIComponent(bridge)}`

describe('auth guard SSO bridge forwarding', () => {
  beforeEach(() => {
    getEnvs.mockReset()
    ssoLoginRaw.mockReset()
    ssoLoginRaw.mockResolvedValue({ status: 401, data: {} })
    const loc = { origin: SELF, href: SELF + '/' }
    vi.stubGlobal('window', { location: loc })
    vi.stubGlobal('sessionStorage', { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() })
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() })
    vi.stubGlobal('history', { replaceState: vi.fn() })
  })

  it('forwards the code to an allow-listed bridge', async () => {
    getEnvs.mockResolvedValue({ ssoBridgeAllowlist: ['https://app.example.com/portal'] })
    await runGuard(stateFor('https://app.example.com/portal/sso-bridge'))
    const href = (window as any).location.href as string
    expect(href.startsWith('https://app.example.com/portal/sso-bridge?')).toBe(true)
    expect(new URL(href).searchParams.get('code')).toBe('CODE123')
  })

  it('forwards to the console origin with an empty allow-list', async () => {
    getEnvs.mockResolvedValue({ ssoBridgeAllowlist: [] })
    await runGuard(stateFor(`${SELF}/lens/sso-bridge`))
    expect((window as any).location.href).toContain('/lens/sso-bridge?code=CODE123')
  })

  it.each([
    'https://evil.example.net/sso-bridge',
    'javascript:alert(1)',
    'data:text/html,x',
    '//evil.example.net/x',
    'https://app.example.com@evil.example.net/portal',
    'https://app.example.com.evil.example.net/portal/x',
  ])('never forwards the code to %s', async (bridge) => {
    getEnvs.mockResolvedValue({ ssoBridgeAllowlist: ['https://app.example.com/portal'] })
    const next = await runGuard(stateFor(bridge))
    expect((window as any).location.href).toBe(SELF + '/')
    expect(ssoLoginRaw).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith(expect.objectContaining({ path: '/sso-error' }))
  })

  it('falls back to same origin only when /envs fails', async () => {
    getEnvs.mockRejectedValue(new Error('down'))
    const next = await runGuard(stateFor('https://app.example.com/portal/sso-bridge'))
    expect((window as any).location.href).toBe(SELF + '/')
    expect(next).toHaveBeenCalledWith(expect.objectContaining({ path: '/sso-error' }))
  })
})

// A visitor's saved session is 'anonymous', not 'unknown': the page the identity provider sends
// the browser back to loads with that, and the provider may have signed the caller in meanwhile.
describe('auth guard session probe', () => {
  const page = { query: {}, path: '/workloads', fullPath: '/workloads', name: 'Workloads' }

  const run = async () => {
    let guard: any
    setupAuthGuard({ beforeEach: (fn: any) => (guard = fn) } as any)
    const next = vi.fn()
    await guard(page, {}, next)
    return next
  }

  // A store whose probe signs the caller in, as GET /users/self does behind the provider.
  const probeSignsIn = () => {
    const state: Record<string, unknown> = {
      session: 'anonymous',
      userId: '',
      isLogin: false,
      ensureSessionOnce: vi.fn(async () => {
        state.session = 'authenticated'
        state.userId = 'u1'
        state.isLogin = true
      }),
    }
    return state
  }

  it('waits for the probe when the saved session is anonymous and there is no userId', async () => {
    const state = probeSignsIn()
    userState = state
    const next = await run()
    expect(state.ensureSessionOnce).toHaveBeenCalledTimes(1)
    // Decided after the probe: the caller goes where they asked, not to /login.
    expect(next).toHaveBeenCalledWith()
  })

  it('still sends a caller the probe does not sign in to /login', async () => {
    const state = probeSignsIn()
    ;(state.ensureSessionOnce as ReturnType<typeof vi.fn>).mockImplementation(async () => {})
    userState = state
    const next = await run()
    expect(state.ensureSessionOnce).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledWith({ path: '/login', query: { redirect: '/workloads' } })
  })

  it('does not probe again for a session that is already known with a userId', async () => {
    const state = probeSignsIn()
    Object.assign(state, { session: 'authenticated', userId: 'u1', isLogin: true })
    userState = state
    const next = await run()
    expect(state.ensureSessionOnce).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith()
  })
})
