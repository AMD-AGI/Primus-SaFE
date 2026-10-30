import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()
const post = vi.fn()
const del = vi.fn()

vi.mock('@/services/request', () => ({
  default: {
    get: (...a: unknown[]) => get(...a),
    post: (...a: unknown[]) => post(...a),
    delete: (...a: unknown[]) => del(...a),
  },
}))

import { apiKeysPath, createAPIKey, deleteAPIKey, listAPIKeys } from './index'

// The request base is `/api`, so `/v1/apikeys` is the public `/api/v1/apikeys`, the path a gateway
// can route to the identity provider; `/apikeys` is the apiserver's compatibility alias.
describe('the API key routes', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    del.mockReset()
  })

  it('apiKeysPath picks the identity provider route only in external mode', () => {
    expect(apiKeysPath(false)).toBe('/apikeys')
    expect(apiKeysPath(true)).toBe('/v1/apikeys')
  })

  const body = { name: 'ci', ttlDays: 30, whitelist: ['10.0.0.0/24'] }

  it('built-in accounts are unchanged, with or without the flag', async () => {
    await createAPIKey(body)
    await createAPIKey(body, false)
    expect(post.mock.calls).toEqual([
      ['/apikeys', body],
      ['/apikeys', body],
    ])
    await listAPIKeys({ offset: 0, limit: 20 })
    expect(get).toHaveBeenCalledWith('/apikeys', { params: { offset: 0, limit: 20 } })
    await deleteAPIKey(7)
    expect(del).toHaveBeenCalledWith('/apikeys/7')
  })

  it('create, list and delete go to the identity provider in external mode', async () => {
    const created = { id: 3, name: 'ci', apiKey: 'ak-xyz' }
    post.mockResolvedValue(created)
    await expect(createAPIKey(body, true)).resolves.toBe(created)
    expect(post).toHaveBeenCalledWith('/v1/apikeys', body)

    await listAPIKeys({ offset: 20, limit: 20, sortBy: 'creationTime', order: 'desc' }, true)
    expect(get).toHaveBeenCalledWith('/v1/apikeys', {
      params: { offset: 20, limit: 20, sortBy: 'creationTime', order: 'desc' },
    })

    await deleteAPIKey(7, true)
    expect(del).toHaveBeenCalledWith('/v1/apikeys/7')
  })
})
