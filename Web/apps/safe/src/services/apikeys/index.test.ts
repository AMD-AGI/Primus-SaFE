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

import { createAPIKey, deleteAPIKey, listAPIKeys } from './index'

// Without a provider path the routes are the apiserver's `/apikeys` under the request base `/api`.
// With one (GET /envs apiKeysUrl) they are that path on the console's own origin, called as it is:
// not under the request base, which would make `/api/api/v1/apikeys`.
describe('the API key routes', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    del.mockReset()
  })

  const body = { name: 'ci', ttlDays: 30, whitelist: ['10.0.0.0/24'] }
  const params = { offset: 20, limit: 20, sortBy: 'creationTime' as const, order: 'desc' as const }
  const provider = '/api/v1/apikeys'

  it('the apiserver is the default, with or without an empty path', async () => {
    await createAPIKey(body)
    await createAPIKey(body, '')
    expect(post.mock.calls).toEqual([
      ['/apikeys', body],
      ['/apikeys', body],
    ])
    await listAPIKeys({ offset: 0, limit: 20 })
    expect(get).toHaveBeenCalledWith('/apikeys', { params: { offset: 0, limit: 20 } })
    await deleteAPIKey(7)
    expect(del).toHaveBeenCalledWith('/apikeys/7')
  })

  it('create, list and delete go to the identity provider at the path they are given', async () => {
    const created = { id: 3, name: 'ci', apiKey: 'ak-xyz' }
    post.mockResolvedValue(created)
    await expect(createAPIKey(body, provider)).resolves.toBe(created)
    expect(post).toHaveBeenCalledWith(provider, body, { baseURL: '' })

    await listAPIKeys(params, provider)
    expect(get).toHaveBeenCalledWith(provider, { params, baseURL: '' })

    await deleteAPIKey(7, provider)
    expect(del).toHaveBeenCalledWith(`${provider}/7`, { baseURL: '' })
  })

  it('a deployment can route the provider anywhere on the origin', async () => {
    await listAPIKeys(undefined, '/idp/keys')
    expect(get).toHaveBeenCalledWith('/idp/keys', { params: undefined, baseURL: '' })
  })

  // In external mode both lists are shown, and each key is deleted where it is listed: a key the
  // apiserver issued before the switch is not the provider's, and the other way round.
  it('a key is deleted on the side it is listed from', async () => {
    await deleteAPIKey(5, provider)
    await deleteAPIKey(5)
    expect(del.mock.calls).toEqual([[`${provider}/5`, { baseURL: '' }], ['/apikeys/5']])
  })
})
