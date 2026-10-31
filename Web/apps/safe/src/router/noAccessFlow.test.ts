import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, type Router } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// The whole path from "open a page" to "no access yet", through the real router, the real guards,
// the real stores and the real request layer. Only the network is replaced: the adapter of the
// request instance answers per URL and records what was asked.
vi.hoisted(() => {
  const storage = new Map<string, string>()
  globalThis.localStorage = {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
    key: (index: number) => Array.from(storage.keys())[index] ?? null,
    get length() {
      return storage.size
    },
  } as Storage
})

vi.mock('element-plus', () => ({
  ElMessage: Object.assign(vi.fn(), {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  }),
  ElMessageBox: {},
}))
vi.mock('@/services/mcp', () => ({ addUserToGroup: vi.fn(), getUserNTID: vi.fn() }))
// The app uses the browser history; there is no browser here.
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  createWebHistory: () => createMemoryHistory(),
}))

const self = {
  id: 'u-1',
  name: 'Ada',
  email: 'ada@example.com',
  type: 'default',
  roles: ['first-login'],
}

type Reply = { status: number; data?: unknown }
let replies: Record<string, Reply>
let asked: string[]

const httpError = (config: InternalAxiosRequestConfig, status: number, data: unknown) =>
  new AxiosError(
    `Request failed with status code ${status}`,
    'ERR_BAD_REQUEST',
    config,
    undefined,
    {
      status,
      statusText: '',
      data,
      headers: {},
      config,
    },
  )

async function boot() {
  vi.resetModules()
  setActivePinia(createPinia())
  const { default: request } = await import('@/services/request')
  request.defaults.adapter = async (config: InternalAxiosRequestConfig) => {
    const url = String(config.url)
    asked.push(`${String(config.method).toUpperCase()} ${url}`)
    // Answer on a later turn, as a network does: a redirect loop then times the test out
    // instead of starving the event loop.
    await new Promise((resolve) => setTimeout(resolve, 0))
    const reply = replies[url] ?? { status: 404, data: { errorMessage: 'not found' } }
    if (reply.status >= 400) throw httpError(config, reply.status, reply.data)
    return { status: reply.status, statusText: 'OK', data: reply.data, headers: {}, config }
  }
  const { default: router } = await import('@/router')
  const { useUserStore } = await import('@/stores/user')
  const { useClusterStore } = await import('@/stores/cluster')
  const user = useUserStore()
  // As main.ts does before the first navigation settles.
  await user.ensureSessionOnce()
  return { router, user, cluster: useClusterStore() }
}

beforeEach(() => {
  asked = []
  replies = {
    '/envs': { status: 200, data: { authMode: 'external', accessContact: 'team@example.com' } },
    '/users/self': { status: 200, data: self },
    '/users/self/workspaces': { status: 200, data: { workspaces: [], managedWorkspaces: [] } },
    '/clusters': { status: 403, data: { errorCode: 'Primus.00005', errorMessage: 'forbidden' } },
    '/logout': { status: 200, data: {} },
  }
  vi.spyOn(console, 'warn').mockImplementation(() => {})
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('opening a page as an account that may read only its own record', () => {
  it.each(['/', '/workloads', '/nodes', '/training/detail?id=abc', '/cluster/detail'])(
    'lands on /no-access from %s, having asked exactly envs, self, workspaces and clusters once',
    async (path) => {
      const { router } = await boot()
      await router.push(path)
      expect(router.currentRoute.value.fullPath).toBe('/no-access')
      expect(router.currentRoute.value.name).toBe('NoAccess')
      expect(asked).toEqual([
        'GET /envs',
        'GET /users/self',
        'GET /users/self/workspaces',
        'GET /clusters',
      ])
    },
  )

  it('stays on /no-access without asking anything more', async () => {
    const { router } = await boot()
    await router.push('/workloads')
    asked.length = 0
    // Reload-like visit and a click on the same page: the cluster guard skips /no-access.
    await router.push('/no-access')
    await router.replace('/no-access')
    expect(router.currentRoute.value.fullPath).toBe('/no-access')
    expect(asked).toEqual([])
  })

  it('shows no error toast for the refusal', async () => {
    const { ElMessage } = await import('element-plus')
    const { router } = await boot()
    await router.push('/workloads')
    expect(ElMessage).not.toHaveBeenCalled()
  })
})

describe('other failures of the clusters fetch are unchanged', () => {
  it('a 500 goes to /error', async () => {
    replies['/clusters'] = { status: 500, data: { errorMessage: 'boom' } }
    const { router } = await boot()
    await router.push('/workloads')
    expect(router.currentRoute.value.fullPath).toBe('/error')
  })

  it('a 401 goes to /login with the page asked for as the way back', async () => {
    const { router, user } = await boot()
    // The session ends after the console has started: the server refuses everything now.
    replies['/clusters'] = { status: 401, data: {} }
    replies['/users/self'] = { status: 401, data: {} }
    await router.push('/workloads')
    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.redirect).toBe(encodeURIComponent('/workloads'))
    expect(user.session).toBe('anonymous')
  })
})

describe('/no-access needs a session', () => {
  it('sends an anonymous visitor to /login', async () => {
    replies['/users/self'] = { status: 401, data: {} }
    const { router } = await boot()
    await router.push('/no-access')
    expect(router.currentRoute.value.path).toBe('/login')
    expect(asked).not.toContain('GET /clusters')
  })

  it('lets a user who does have access open it (a bookmark) without asking the clusters', async () => {
    replies['/clusters'] = {
      status: 200,
      data: { items: [{ clusterId: 'c1', phase: 'Ready' }], totalCount: 1 },
    }
    const { router } = await boot()
    await router.push('/no-access')
    expect(router.currentRoute.value.name).toBe('NoAccess')
    expect(asked).toEqual(['GET /envs', 'GET /users/self', 'GET /users/self/workspaces'])
  })

  it('is registered outside the layout, hidden from menus', async () => {
    const { router } = await boot()
    const resolved = router.resolve('/no-access')
    expect(resolved.name).toBe('NoAccess')
    expect(resolved.meta.hideMenu).toBe(true)
    // Not a child of the layout route that carries the menu and the header.
    expect(resolved.matched).toHaveLength(1)
    expect(resolved.matched[0].path).toBe('/no-access')
  })
})

describe('Check again through the real request layer', () => {
  const useIt = async () => {
    const booted = await boot()
    await booted.router.push('/workloads')
    asked.length = 0
    const { useNoAccess } = await import('@/pages/NoAccess/useNoAccess')
    const replace = vi.fn().mockResolvedValue(undefined)
    return {
      ...booted,
      replace,
      page: useNoAccess({ replace } as unknown as Pick<Router, 'replace'>),
    }
  }

  it('stays when the clusters still answer 403', async () => {
    const { page, replace } = await useIt()
    await page.checkAgain()
    expect(asked).toEqual(['GET /clusters'])
    expect(page.stillNoAccess.value).toBe(true)
    expect(replace).not.toHaveBeenCalled()
  })

  it('goes home when they answer', async () => {
    const { page, replace, cluster } = await useIt()
    replies['/clusters'] = {
      status: 200,
      data: { items: [{ clusterId: 'c1', phase: 'Ready' }], totalCount: 1 },
    }
    await page.checkAgain()
    expect(asked).toEqual(['GET /clusters'])
    expect(replace).toHaveBeenCalledWith('/')
    expect(cluster.isFetched).toBe(true)
  })

  it('signs out through POST /logout and goes to /login', async () => {
    const { page, replace, user } = await useIt()
    await page.signOut()
    expect(asked).toEqual(['POST /logout'])
    expect(user.session).toBe('anonymous')
    expect(replace).toHaveBeenCalledWith('/login')
  })
})
