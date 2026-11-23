import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AxiosError, type AxiosAdapter, type InternalAxiosRequestConfig } from 'axios'

const message = vi.fn()
vi.mock('element-plus', () => ({ ElMessage: (opts: unknown) => message(opts) }))
vi.mock('@/stores/cluster', () => ({ useClusterStore: () => ({ $reset: vi.fn() }) }))
vi.mock('@/stores/workspace', () => ({ useWorkspaceStore: () => ({ $reset: vi.fn() }) }))
vi.mock('@/stores/user', () => ({ useUserStore: () => ({ $patch: vi.fn() }) }))

import { request } from './request'

/** An adapter that fails every request with status, or without an answer when status is undefined. */
function failWith(status?: number): AxiosAdapter {
  return (config: InternalAxiosRequestConfig) => {
    if (status === undefined) {
      return Promise.reject(new AxiosError('timeout', 'ECONNABORTED', config))
    }
    const response = { status, statusText: '', headers: {}, config, data: { errorMessage: 'nope' } }
    return Promise.reject(new AxiosError('failed', 'ERR_BAD_RESPONSE', config, null, response))
  }
}

describe('request silentErrors', () => {
  const location = { pathname: '/post-training/pt-1', search: '', href: '' }

  beforeEach(() => {
    message.mockClear()
    location.href = ''
    vi.stubGlobal('location', location)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it.each([403, 404, 500, 503, undefined])(
    'rejects a failure with status %s with the raw error and shows nothing',
    async (status) => {
      const err = await request
        .get('/post-training/runs/pt-1', { adapter: failWith(status), silentErrors: true })
        .catch((e) => e)
      expect(err).toBeInstanceOf(AxiosError)
      expect((err as AxiosError).response?.status).toBe(status)
      expect(message).not.toHaveBeenCalled()
    },
  )

  it('still ends the session on a 401', async () => {
    const err = await request
      .get('/post-training/runs/pt-1', { adapter: failWith(401), silentErrors: true })
      .catch((e) => e)
    expect((err as AxiosError).response?.status).toBe(401)
    expect(location.href).toBe('/login?redirect=%2Fpost-training%2Fpt-1')
  })

  it('shows the error of a request without it', async () => {
    await request.get('/post-training/runs/pt-1', { adapter: failWith(500) }).catch(() => undefined)
    expect(message).toHaveBeenCalledTimes(1)
  })
})
