import type { Router } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

type Guard = (to: object, from: object, next: (to?: unknown) => void) => Promise<void>

const prompt = vi.fn()
const showError = vi.fn()
const getUserNTID = vi.fn()
const addUserToGroup = vi.fn()

vi.mock('element-plus', () => ({
  ElMessage: { info: vi.fn(), error: (...a: unknown[]) => showError(...a) },
  ElMessageBox: { prompt: (...a: unknown[]) => prompt(...a) },
}))
vi.mock('@/stores/user', () => ({ useUserStore: () => ({ session: 'anonymous' }) }))
vi.mock('@/services/login', () => ({ getEnvs: vi.fn(), ssoLoginRaw: vi.fn() }))
vi.mock('@/services/mcp', () => ({
  getUserNTID: (...a: unknown[]) => getUserNTID(...a),
  addUserToGroup: (...a: unknown[]) => addUserToGroup(...a),
}))
vi.mock('@/utils', () => ({ getSafeRedirect: (v?: string) => v || '/' }))

import setupAuthGuard from './auth'

// Where identity sends someone Okta refused because they are not in the group yet.
async function join(identifier: string) {
  let guard!: Guard
  setupAuthGuard({ beforeEach: (fn: Guard) => (guard = fn) } as unknown as Router)
  prompt.mockResolvedValue({ value: identifier })
  const next = vi.fn()
  const query = {
    error: 'access_denied',
    error_description: 'User is not assigned to the client application.',
  }
  await guard({ query, path: '/sso/callback', fullPath: '/sso/callback' }, {}, next)
  return next
}

describe('Join on an access_denied callback', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('sessionStorage', { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() })
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() })
    vi.stubGlobal('history', { replaceState: vi.fn() })
    vi.spyOn(console, 'error').mockImplementation(() => {})
    getUserNTID.mockResolvedValue('NTID: jashish')
  })

  it.each([
    "Successfully added 'jashish' to 'dl.primus-safe-users'",
    'Error: Failed to add user: entryAlreadyExists',
  ])(
    'adds the NTID the lookup found and shows it as joined when the service answers %s',
    async (answer) => {
      addUserToGroup.mockResolvedValue(answer)
      const next = await join(' Ashish.Jain@amd.com ')
      expect(getUserNTID).toHaveBeenCalledWith('Ashish.Jain@amd.com')
      expect(addUserToGroup).toHaveBeenCalledWith('jashish', 'dl.primus-safe-users')
      expect(next).toHaveBeenCalledWith({
        path: '/sso-error',
        query: { error: 'joined', error_description: 'jashish' },
      })
    },
  )

  it('does not show an add the service refused as joined', async () => {
    addUserToGroup.mockResolvedValue('Error: Failed to add user: insufficientAccessRights')
    const next = await join('jashish')
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('insufficientAccessRights'))
    expect(next).toHaveBeenCalledWith(
      expect.objectContaining({
        path: '/sso-error',
        query: expect.objectContaining({ error: 'access_denied' }),
      }),
    )
  })
})
