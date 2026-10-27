import { beforeEach, describe, expect, it, vi } from 'vitest'

const getEnvs = vi.fn()
const ssoLoginRaw = vi.fn()

vi.mock('element-plus', () => ({ ElMessage: {}, ElMessageBox: {} }))
vi.mock('@/stores/user', () => ({ useUserStore: () => ({ session: 'unknown' }) }))
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
