import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { createPinia, setActivePinia } from 'pinia'
import type { Router } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { EnvsResp, UserSelfData } from '@/services/login/type'

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

// Every function of the service modules the stores and the page can reach. A call to one that is
// not listed here cannot happen quietly: the mock has no such export and the import fails.
const services = {
  getEnvs: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  getUserData: vi.fn(),
  getSelfData: vi.fn(),
  getSelfWorkspaces: vi.fn(),
  getWorkspace: vi.fn(),
  getClusters: vi.fn(),
  copyText: vi.fn(),
}

vi.mock('element-plus', () => ({ ElMessage: () => {}, ElMessageBox: {} }))
vi.mock('@/services/login', () => ({
  getEnvs: (...a: unknown[]) => services.getEnvs(...a),
  login: (...a: unknown[]) => services.login(...a),
  logout: (...a: unknown[]) => services.logout(...a),
  getUserData: (...a: unknown[]) => services.getUserData(...a),
  getSelfData: (...a: unknown[]) => services.getSelfData(...a),
  getSelfWorkspaces: (...a: unknown[]) => services.getSelfWorkspaces(...a),
}))
vi.mock('@/services/base', () => ({
  getWorkspace: (...a: unknown[]) => services.getWorkspace(...a),
  getClusters: (...a: unknown[]) => services.getClusters(...a),
}))
vi.mock('@/utils', () => ({ copyText: (...a: unknown[]) => services.copyText(...a) }))

import { useClusterStore } from '@/stores/cluster'
import { useUserStore } from '@/stores/user'
import { useNoAccess } from './useNoAccess'

const profile: UserSelfData = {
  id: 'u-123',
  name: 'Ada',
  email: 'ada@example.com',
  type: 'default',
  roles: ['first-login', 'viewer'],
  workspaces: [],
  creationTime: '',
  restrictedType: 0,
}

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

// A promise the test settles by hand.
const deferred = <T>() => {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

const calledServices = () =>
  Object.entries(services)
    .filter(([, fn]) => fn.mock.calls.length > 0)
    .map(([name]) => name)

let replace: ReturnType<typeof vi.fn>

// What /envs answered; the page reads only accessContact, so a test gives only what it needs.
const envsOf = (envs: Record<string, unknown> | null) => envs as unknown as EnvsResp | null

const setup = (opts: { envs?: Record<string, unknown> | null } = {}) => {
  const user = useUserStore()
  user.$patch({ session: 'authenticated', userId: profile.id, profile })
  user.envs = envsOf(opts.envs === undefined ? {} : opts.envs)
  const router = { replace } as unknown as Pick<Router, 'replace'>
  return { user, cluster: useClusterStore(), page: useNoAccess(router) }
}

beforeEach(() => {
  setActivePinia(createPinia())
  for (const fn of Object.values(services)) fn.mockReset()
  services.getClusters.mockResolvedValue({ items: [], totalCount: 0 })
  services.logout.mockResolvedValue(undefined)
  replace = vi.fn().mockResolvedValue(undefined)
})

describe('profile fields', () => {
  it('come from the user store record', () => {
    const { page } = setup()
    expect(page.name.value).toBe('Ada')
    expect(page.email.value).toBe('ada@example.com')
    expect(page.id.value).toBe('u-123')
    expect(page.rolesText.value).toBe('first-login, viewer')
  })

  it('are empty, not a failure, when there is no record', () => {
    const { user, page } = setup()
    user.profile = null
    expect([page.name.value, page.email.value, page.id.value, page.rolesText.value]).toEqual([
      '',
      '',
      '',
      '',
    ])
  })

  it('show no roles for a record without any', () => {
    const { user, page } = setup()
    user.profile = { ...profile, roles: undefined } as unknown as UserSelfData
    expect(page.rolesText.value).toBe('')
  })
})

describe('contact line', () => {
  it.each([
    ['text', { accessContact: 'Ask the platform team' }, 'Ask the platform team'],
    [
      'text with surrounding space',
      { accessContact: '  mail ada@example.com \n' },
      'mail ada@example.com',
    ],
    ['markup, kept as the text it is', { accessContact: '<b>x</b>' }, '<b>x</b>'],
    ['an empty string', { accessContact: '' }, ''],
    ['an absent field (older server)', {}, ''],
    ['a value that is not a string', { accessContact: 42 }, ''],
  ])('is %s', (_label, envs, want) => {
    const { page } = setup({ envs })
    expect(page.contact.value).toBe(want)
  })

  it('is empty while /envs has not been read', () => {
    const { page } = setup({ envs: null })
    expect(page.contact.value).toBe('')
  })

  it('is read from the envs the console already holds, without another request', async () => {
    const { page } = setup({ envs: { accessContact: 'team@example.com' } })
    await page.loadContact()
    expect(page.contact.value).toBe('team@example.com')
    expect(calledServices()).toEqual([])
  })

  it('is fetched once through getEnvs, quietly, when start-up did not get /envs', async () => {
    services.getEnvs.mockResolvedValue({ accessContact: 'team@example.com' })
    const { page } = setup({ envs: null })
    await page.loadContact()
    expect(page.contact.value).toBe('team@example.com')
    expect(services.getEnvs).toHaveBeenCalledTimes(1)
    expect(services.getEnvs).toHaveBeenCalledWith(true)
  })

  it('stays empty when /envs cannot be read', async () => {
    services.getEnvs.mockRejectedValue(new Error('down'))
    const { page } = setup({ envs: null })
    await expect(page.loadContact()).resolves.toBeUndefined()
    expect(page.contact.value).toBe('')
  })
})

describe('copy id', () => {
  it('copies the id', async () => {
    const { page } = setup()
    await page.copyId()
    expect(services.copyText).toHaveBeenCalledTimes(1)
    expect(services.copyText).toHaveBeenCalledWith('u-123')
  })

  it('copies nothing when there is no id', async () => {
    const { user, page } = setup()
    user.profile = null
    await page.copyId()
    expect(services.copyText).not.toHaveBeenCalled()
  })
})

describe('Check again', () => {
  it('goes home when the clusters can be read', async () => {
    const { cluster, page } = setup()
    await page.checkAgain()
    expect(services.getClusters).toHaveBeenCalledTimes(1)
    expect(services.getClusters).toHaveBeenCalledWith(true)
    expect(cluster.isFetched).toBe(true)
    expect(replace).toHaveBeenCalledTimes(1)
    expect(replace).toHaveBeenCalledWith('/')
    expect(page.stillNoAccess.value).toBe(false)
    expect(page.checking.value).toBe(false)
  })

  it('stays, and says so, when refused with "no access yet"', async () => {
    services.getClusters.mockRejectedValue(noAccess())
    const { cluster, page } = setup()
    await page.checkAgain()
    expect(services.getClusters).toHaveBeenCalledTimes(1)
    expect(replace).not.toHaveBeenCalled()
    expect(page.stillNoAccess.value).toBe(true)
    expect(page.checking.value).toBe(false)
    expect(cluster.isFetched).toBe(false)
  })

  it('goes home after a "no access yet" once access has been granted', async () => {
    services.getClusters.mockRejectedValueOnce(noAccess())
    const { page } = setup()
    await page.checkAgain()
    expect(replace).not.toHaveBeenCalled()
    await page.checkAgain()
    expect(replace).toHaveBeenCalledWith('/')
    expect(page.stillNoAccess.value).toBe(false)
  })

  it('keeps the saved cluster of a user who does have access', async () => {
    services.getClusters.mockResolvedValue({
      items: [
        { clusterId: 'c1', phase: 'Ready' },
        { clusterId: 'c2', phase: 'Ready' },
      ],
      totalCount: 2,
    })
    const { cluster, page } = setup()
    cluster.currentClusterId = 'c2'
    await page.checkAgain()
    expect(cluster.currentClusterId).toBe('c2')
  })

  it.each([
    ['a 500', httpError(500)],
    ['a 401', httpError(401)],
    [
      'a 403 with another error code',
      httpError(403, { errorCode: 'Primus.00003', errorMessage: 'denied' }),
    ],
    ['a 403 with an empty body', httpError(403, {})],
    ['a 403 with no body', httpError(403, undefined)],
    ['a 403 with a string body', httpError(403, 'Forbidden')],
    ['a network error', new AxiosError('Network Error', 'ERR_NETWORK')],
  ])(
    'hands %s to the guards by going home, which treats it as it always did',
    async (_label, error) => {
      services.getClusters.mockRejectedValue(error)
      const { page } = setup()
      await page.checkAgain()
      expect(replace).toHaveBeenCalledWith('/')
      expect(page.stillNoAccess.value).toBe(false)
    },
  )

  it('ignores a second press while the first is being answered', async () => {
    const pending = deferred<{ items: never[]; totalCount: number }>()
    services.getClusters.mockReturnValue(pending.promise)
    const { page } = setup()
    const first = page.checkAgain()
    expect(page.checking.value).toBe(true)
    await page.checkAgain()
    pending.resolve({ items: [], totalCount: 0 })
    await first
    expect(services.getClusters).toHaveBeenCalledTimes(1)
    expect(replace).toHaveBeenCalledTimes(1)
  })
})

describe('Sign out', () => {
  it('uses the user store logout and then goes to /login', async () => {
    const { user, page } = setup()
    await page.signOut()
    expect(services.logout).toHaveBeenCalledTimes(1)
    expect(user.session).toBe('anonymous')
    expect(user.profile).toBeNull()
    expect(replace).toHaveBeenCalledWith('/login')
  })

  it('still goes to /login when the logout request fails, as the store clears the session anyway', async () => {
    services.logout.mockRejectedValue(new Error('down'))
    const { user, page } = setup()
    await page.signOut()
    expect(user.session).toBe('anonymous')
    expect(replace).toHaveBeenCalledWith('/login')
    expect(page.signingOut.value).toBe(false)
  })
})

describe('what the page may ask the server', () => {
  it('opening it with /envs already known asks nothing', async () => {
    const { page } = setup({ envs: { accessContact: 'team@example.com' } })
    await page.loadContact()
    expect(calledServices()).toEqual([])
  })

  it('opening it without /envs asks /envs and nothing else', async () => {
    services.getEnvs.mockResolvedValue({})
    const { page } = setup({ envs: null })
    await page.loadContact()
    expect(calledServices()).toEqual(['getEnvs'])
  })

  it('Check again asks the clusters and nothing else', async () => {
    const { page } = setup()
    await page.checkAgain()
    expect(calledServices()).toEqual(['getClusters'])
  })

  it('Sign out only logs out', async () => {
    const { page } = setup()
    await page.signOut()
    expect(calledServices()).toEqual(['logout'])
  })

  it('never reads another user, a user list, the workspaces or the user record', async () => {
    services.getEnvs.mockResolvedValue({})
    services.getClusters.mockRejectedValue(noAccess())
    const { page } = setup({ envs: null })
    await page.loadContact()
    await page.copyId()
    await page.checkAgain()
    await page.signOut()
    for (const name of [
      'getUserData',
      'getSelfData',
      'getSelfWorkspaces',
      'getWorkspace',
      'login',
    ]) {
      expect(services[name as keyof typeof services]).not.toHaveBeenCalled()
    }
  })
})
