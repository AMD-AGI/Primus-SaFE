import { beforeEach, describe, expect, it, vi } from 'vitest'

const ssoLoginRaw = vi.fn()

vi.mock('@/stores/user', () => ({ useUserStore: () => ({ session: 'unknown' }) }))
vi.mock('@/services/login', () => ({ ssoLoginRaw: (...a: any[]) => ssoLoginRaw(...a) }))
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

const stateFor = (bridge: string) => `lens:csrf:${encodeURIComponent(bridge)}`

describe('auth guard SSO bridge forwarding', () => {
  beforeEach(() => {
    ssoLoginRaw.mockReset()
    ssoLoginRaw.mockResolvedValue({ status: 401, data: {} })
    const loc = { origin: SELF, href: SELF + '/' }
    vi.stubGlobal('window', { location: loc })
    vi.stubGlobal('sessionStorage', { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() })
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() })
    vi.stubGlobal('history', { replaceState: vi.fn() })
  })

  it('forwards the code to the Lens app on the console origin', async () => {
    await runGuard(stateFor(`${SELF}/lens/sso-bridge`))
    const href = (window as any).location.href as string
    expect(href.startsWith(`${SELF}/lens/sso-bridge?`)).toBe(true)
    expect(new URL(href).searchParams.get('code')).toBe('CODE123')
    expect(ssoLoginRaw).not.toHaveBeenCalled()
  })

  it.each([
    'https://app.example.com/portal/sso-bridge',
    'https://evil.example.net/sso-bridge',
    'javascript:alert(1)',
    'data:text/html,x',
    '//evil.example.net/x',
    'https://console.example.com@evil.example.net/portal',
    'https://console.example.com.evil.example.net/portal/x',
  ])('never forwards the code to %s', async (bridge) => {
    const next = await runGuard(stateFor(bridge))
    expect((window as any).location.href).toBe(SELF + '/')
    expect(ssoLoginRaw).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith(expect.objectContaining({ path: '/sso-error' }))
  })
})
