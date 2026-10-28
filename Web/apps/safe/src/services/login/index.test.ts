import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()

vi.mock('axios', () => ({ default: { post: vi.fn() } }))
vi.mock('@/services/request', () => ({
  default: { get: (...a: unknown[]) => get(...a), post: vi.fn(), patch: vi.fn(), delete: vi.fn(), put: vi.fn() },
  postForm: vi.fn(),
}))

import { getSelfWorkspaces } from './index'

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
