import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

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

const getUserData = vi.fn()
const getEnvs = vi.fn()
const getSelfData = vi.fn()
const getSelfWorkspaces = vi.fn()

vi.mock('element-plus', () => ({ ElMessage: () => {}, ElMessageBox: {} }))
vi.mock('@/services/login', () => ({
  login: vi.fn(),
  logout: vi.fn(),
  getEnvs: (...a: unknown[]) => getEnvs(...a),
  getUserData: (...a: unknown[]) => getUserData(...a),
  getSelfData: (...a: unknown[]) => getSelfData(...a),
  getSelfWorkspaces: (...a: unknown[]) => getSelfWorkspaces(...a),
}))
vi.mock('@/services/base', () => ({ getWorkspace: vi.fn() }))

import { useUserStore } from './user'

const profile = { id: 'u1', name: 'user-one', roles: ['default'], workspaces: [], managedWorkspaces: [] }

// A promise the test settles by hand, so that "has the session finished yet" is observable.
const deferred = <T>() => {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const flush = () => new Promise((r) => setTimeout(r, 0))

describe('ensureSessionOnce and the auth mode', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getUserData.mockReset().mockResolvedValue(profile)
    getEnvs.mockReset()
    getSelfData.mockReset()
    getSelfWorkspaces.mockReset().mockResolvedValue(undefined)
  })

  const signedIn = () => {
    const store = useUserStore()
    store.userId = 'u1'
    return store
  }

  it('is not done until /envs has answered, so a reload never renders builtin-mode UI first', async () => {
    const envs = deferred<{ authMode: string }>()
    getEnvs.mockReturnValue(envs.promise)
    const store = signedIn()

    let done = false
    const session = store.ensureSessionOnce().then(() => {
      done = true
    })
    await flush()
    expect(store.session).toBe('authenticated')
    expect(done).toBe(false)

    envs.resolve({ authMode: 'external' })
    await session
    expect(done).toBe(true)
    expect(store.isExternalAuth).toBe(true)
  })

  it('keeps the session when /envs fails: the mode is unknown, not the user signed out', async () => {
    getEnvs.mockRejectedValue(new Error('503'))
    const store = signedIn()

    await store.ensureSessionOnce()
    expect(store.session).toBe('authenticated')
    expect(store.userId).toBe('u1')
    expect(store.envs).toBeNull()
    expect(store.isExternalAuth).toBe(false)
  })

  it('asks /envs, quietly, and nothing else of a visitor in builtin mode', async () => {
    getEnvs.mockResolvedValue({ authMode: 'builtin' })
    const store = useUserStore()
    await store.ensureSessionOnce()
    expect(store.session).toBe('anonymous')
    expect(getEnvs).toHaveBeenCalledWith(true)
    // Its only way in is POST /login, which sets userId itself: no probe.
    expect(getSelfData).not.toHaveBeenCalled()
    expect(getUserData).not.toHaveBeenCalled()
  })

  it('treats a visitor as anonymous when /envs does not answer', async () => {
    getEnvs.mockRejectedValue(new Error('503'))
    const store = useUserStore()
    await store.ensureSessionOnce()
    expect(store.session).toBe('anonymous')
    expect(getSelfData).not.toHaveBeenCalled()
  })
})

// With an identity provider the browser holds a session without POST /login ever having run (the
// server refuses it), so the console has no userId and must ask the server who the caller is.
describe('ensureSessionOnce without a userId in external mode', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getUserData.mockReset().mockResolvedValue(profile)
    getEnvs.mockReset().mockResolvedValue({ authMode: 'external' })
    getSelfData.mockReset()
    getSelfWorkspaces.mockReset().mockResolvedValue(undefined)
  })

  it('signs the caller in from GET /users/self', async () => {
    getSelfData.mockResolvedValue(profile)
    const store = useUserStore()
    expect(store.userId).toBe('')

    await store.ensureSessionOnce()
    expect(store.session).toBe('authenticated')
    expect(store.isLogin).toBe(true)
    expect(store.userId).toBe('u1')
    expect(store.profile?.name).toBe('user-one')
    expect(store.isExternalAuth).toBe(true)
    // The probe's answer is the record: nothing is asked by id.
    expect(getUserData).not.toHaveBeenCalled()
  })

  it("takes the workspace lists from the user's own route", async () => {
    getSelfData.mockResolvedValue(profile)
    getSelfWorkspaces.mockResolvedValue({
      workspaces: [{ id: 'ws-a', name: 'ws-a' }],
      managedWorkspaces: [{ id: 'ws-a', name: 'ws-a' }],
    })
    const store = useUserStore()
    await store.ensureSessionOnce()
    expect(store.profile?.workspaces.map((w) => w.id)).toEqual(['ws-a'])
    expect(store.profile?.managedWorkspaces?.map((w) => w.id)).toEqual(['ws-a'])
  })

  it('stays anonymous when the provider has no session for the caller (401)', async () => {
    getSelfData.mockRejectedValue({ response: { status: 401 } })
    const store = useUserStore()
    await store.ensureSessionOnce()
    expect(store.session).toBe('anonymous')
    expect(store.userId).toBe('')
    expect(store.profile).toBeNull()
    expect(store.isLogin).toBe(false)
  })

  it('stays anonymous on an answer that names no user', async () => {
    getSelfData.mockResolvedValue({})
    const store = useUserStore()
    await store.ensureSessionOnce()
    expect(store.session).toBe('anonymous')
    expect(store.userId).toBe('')
  })

  it('probes once however many callers ask', async () => {
    getSelfData.mockResolvedValue(profile)
    const store = useUserStore()
    await Promise.all([store.ensureSessionOnce(), store.ensureSessionOnce()])
    expect(getSelfData).toHaveBeenCalledTimes(1)
  })

  it('does not probe for a console that already has a userId', async () => {
    const store = useUserStore()
    store.userId = 'u1'
    await store.ensureSessionOnce()
    expect(store.session).toBe('authenticated')
    expect(getSelfData).not.toHaveBeenCalled()
  })
})
