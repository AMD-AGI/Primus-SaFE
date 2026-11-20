import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()
const post = vi.fn()

vi.mock('axios', () => ({ default: { post: vi.fn() } }))
vi.mock('@/services/request', () => ({
  default: { get: (...a: unknown[]) => get(...a), post: (...a: unknown[]) => post(...a), patch: vi.fn(), delete: vi.fn(), put: vi.fn() },
  postForm: vi.fn(),
}))

import { getSelfWorkspaces, logout } from './index'

describe('getSelfWorkspaces', () => {
  beforeEach(() => get.mockReset())

  it('reads the caller own lists and leaves a failure to the caller', async () => {
    const answer = { workspaces: [], managedWorkspaces: [] }
    get.mockResolvedValue(answer)
    await expect(getSelfWorkspaces()).resolves.toBe(answer)
    // No toast and no session teardown when the route is missing or refuses: the console
    // falls back to the lists of the user record.
    expect(get).toHaveBeenCalledWith('/users/self/workspaces', { skipErrorHandler: true })
  })
})

describe('logout', () => {
  beforeEach(() => post.mockReset().mockResolvedValue(undefined))

  it('goes through the global error handler by default', async () => {
    await logout()
    expect(post).toHaveBeenCalledWith('/logout', null, undefined)
  })

  it('leaves a failure to the caller when quiet, so a 401 does not redirect to the sign-in page', async () => {
    await logout(true)
    expect(post).toHaveBeenCalledWith('/logout', null, { skipErrorHandler: true })
  })
})
