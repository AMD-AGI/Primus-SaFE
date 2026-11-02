import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import type { RouteLocationNormalized, Router } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

type Spy = ReturnType<typeof vi.fn>

// The three stores the guard reads, as plain objects a test can shape.
let clusterState: {
  isFetched: boolean
  isReady: boolean
  currentClusterId: string
  fetchClusters: Spy
  $reset: Spy
}
let wsState: { fetchWorkspace: Spy; isCurrentWorkspaceAdmin: () => boolean; $reset: Spy }
let userState: { isLogin: boolean; hasManagerAccess: boolean; $patch: Spy }

vi.mock('@/stores/cluster', () => ({ useClusterStore: () => clusterState }))
vi.mock('@/stores/workspace', () => ({ useWorkspaceStore: () => wsState }))
vi.mock('@/stores/user', () => ({ useUserStore: () => userState }))

import setupClusterGuard from './clusterReady'

// An error as axios raises it for a response with this status and body.
const httpError = (status: number, data: unknown = {}) =>
  new AxiosError(
    `Request failed with status code ${status}`,
    'ERR_BAD_REQUEST',
    undefined,
    undefined,
    {
      status,
      statusText: '',
      data,
      headers: {},
      config: {} as InternalAxiosRequestConfig,
    },
  )

// The 403 the apiserver sends for a user that has no access yet (its own error code, written out
// here on purpose: it is the contract with the server).
const noAccessBody = {
  errorCode: 'Primus.00051',
  errorMessage: 'The user has no access yet. Ask an administrator to grant it',
}
const noAccess = () => httpError(403, noAccessBody)

const page = (path: string, fullPath = path) =>
  ({ path, fullPath, meta: {} }) as unknown as RouteLocationNormalized

type Guard = (to: RouteLocationNormalized, from: unknown, next: Spy) => Promise<void>

async function run(to: RouteLocationNormalized) {
  let guard!: Guard
  setupClusterGuard({
    beforeEach: (fn: Guard) => {
      guard = fn
    },
  } as unknown as Router)
  const next = vi.fn()
  await guard(to, {}, next)
  return next
}

beforeEach(() => {
  clusterState = {
    isFetched: false,
    isReady: true,
    currentClusterId: 'c1',
    fetchClusters: vi.fn().mockResolvedValue(undefined),
    $reset: vi.fn(),
  }
  wsState = {
    fetchWorkspace: vi.fn().mockResolvedValue(undefined),
    isCurrentWorkspaceAdmin: () => false,
    $reset: vi.fn(),
  }
  userState = { isLogin: true, hasManagerAccess: false, $patch: vi.fn() }
})

describe('cluster guard: the "no access yet" refusal of the clusters fetch', () => {
  it.each(['/', '/workloads', '/nodes', '/training/detail', '/cluster/detail', '/nodedetail'])(
    'sends a signed-in user asking for %s to /no-access',
    async (path) => {
      clusterState.fetchClusters.mockRejectedValue(noAccess())
      const next = await run(page(path))
      expect(next).toHaveBeenCalledTimes(1)
      expect(next).toHaveBeenCalledWith('/no-access')
      // Nothing else is asked once the clusters are refused.
      expect(wsState.fetchWorkspace).not.toHaveBeenCalled()
    },
  )

  it('ignores the query of a deep link: the target is always plain /no-access', async () => {
    clusterState.fetchClusters.mockRejectedValue(noAccess())
    const next = await run(page('/training/detail', '/training/detail?id=abc'))
    expect(next).toHaveBeenCalledWith('/no-access')
  })

  it('asks quietly, so that the refusal reaches the guard as an error with its status', async () => {
    clusterState.fetchClusters.mockRejectedValue(noAccess())
    await run(page('/workloads'))
    expect(clusterState.fetchClusters).toHaveBeenCalledTimes(1)
    expect(clusterState.fetchClusters).toHaveBeenCalledWith(true)
  })

  it('does not treat that refusal from the workspace list as "no access yet"', async () => {
    wsState.fetchWorkspace.mockRejectedValue(noAccess())
    const next = await run(page('/workloads'))
    expect(next).toHaveBeenCalledWith('/error')
  })
})

describe('cluster guard: a 403 that is not "no access yet" is not sent to /no-access', () => {
  // A user who does hold other access (a workspace manager, say) can be refused the cluster list
  // by the handler's own role check; /no-access would strand them.
  it.each([
    ['another error code', httpError(403, { errorCode: 'Primus.00003', errorMessage: 'denied' })],
    ['an empty object body', httpError(403, {})],
    ['no body', httpError(403, undefined)],
    ['a null body', httpError(403, null)],
    ['a string body', httpError(403, 'Forbidden')],
    ['a string body that is the code', httpError(403, 'Primus.00051')],
    ['an array body', httpError(403, [noAccessBody])],
    ['a numeric error code', httpError(403, { errorCode: 51 })],
  ])('goes to /error, like a 500, for a 403 with %s', async (_label, error) => {
    clusterState.fetchClusters.mockRejectedValue(error)
    const next = await run(page('/workloads'))
    expect(next).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledWith('/error')
    expect(next).not.toHaveBeenCalledWith('/no-access')
    expect(wsState.fetchWorkspace).not.toHaveBeenCalled()
  })

  it('stays on /error when it is already there', async () => {
    clusterState.fetchClusters.mockRejectedValue(httpError(403, { errorCode: 'Primus.00003' }))
    const next = await run(page('/error'))
    expect(next).toHaveBeenCalledWith()
  })

  it.each([401, 500])(
    'does not take the no-access code of a %s for "no access yet"',
    async (status) => {
      clusterState.fetchClusters.mockRejectedValue(httpError(status, noAccessBody))
      const next = await run(page('/workloads'))
      expect(next).not.toHaveBeenCalledWith('/no-access')
    },
  )
})

describe('cluster guard: other outcomes are unchanged', () => {
  beforeEach(() => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  it.each(['/', '/workloads', '/nodes'])(
    '401 on %s clears the session and goes to /login',
    async (path) => {
      clusterState.fetchClusters.mockRejectedValue(httpError(401))
      const next = await run(page(path))
      expect(next).toHaveBeenCalledWith({
        path: '/login',
        query: { redirect: encodeURIComponent(path) },
      })
      expect(userState.$patch).toHaveBeenCalledWith(
        expect.objectContaining({ session: 'anonymous', userId: '', profile: null }),
      )
      expect(clusterState.$reset).toHaveBeenCalled()
      expect(wsState.$reset).toHaveBeenCalled()
    },
  )

  it('401 keeps the full path (with its query) as the sign-in redirect', async () => {
    clusterState.fetchClusters.mockRejectedValue(httpError(401))
    const next = await run(page('/training/detail', '/training/detail?id=abc'))
    expect(next).toHaveBeenCalledWith({
      path: '/login',
      query: { redirect: encodeURIComponent('/training/detail?id=abc') },
    })
  })

  it.each([
    ['a 500', httpError(500)],
    ['a 502', httpError(502)],
    ['a network error', new AxiosError('Network Error', 'ERR_NETWORK')],
    ['a plain string, as a response interceptor that is not skipped rejects', 'Request failed.'],
  ])('%s goes to /error', async (_label, error) => {
    clusterState.fetchClusters.mockRejectedValue(error)
    const next = await run(page('/workloads'))
    expect(next).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledWith('/error')
  })

  it('a failure while already on /error stays there', async () => {
    clusterState.fetchClusters.mockRejectedValue(httpError(500))
    const next = await run(page('/error'))
    expect(next).toHaveBeenCalledWith()
  })

  it('a signed-in user whose clusters answer is not sent to /no-access', async () => {
    const next = await run(page('/workloads'))
    expect(clusterState.fetchClusters).toHaveBeenCalledTimes(1)
    expect(wsState.fetchWorkspace).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledWith()
  })

  it('a user whose clusters were already fetched does not ask again', async () => {
    clusterState.isFetched = true
    const next = await run(page('/workloads'))
    expect(clusterState.fetchClusters).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith()
  })

  it('an anonymous user is sent to /login, never to /no-access, and nothing is fetched', async () => {
    userState.isLogin = false
    const next = await run(page('/workloads'))
    expect(clusterState.fetchClusters).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith('/login')
    expect(next).not.toHaveBeenCalledWith('/no-access')
  })
})

describe('cluster guard: /no-access', () => {
  it('is let through without fetching the clusters, so landing on it cannot loop', async () => {
    clusterState.fetchClusters.mockRejectedValue(noAccess())
    const next = await run(page('/no-access'))
    expect(clusterState.fetchClusters).not.toHaveBeenCalled()
    expect(wsState.fetchWorkspace).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledWith()
  })
})
