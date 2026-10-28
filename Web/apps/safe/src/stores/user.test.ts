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

vi.mock('element-plus', () => ({ ElMessage: () => {}, ElMessageBox: {} }))
vi.mock('@/services/login', () => ({
  login: vi.fn(),
  logout: vi.fn(),
  getEnvs: (...a: unknown[]) => getEnvs(...a),
  getUserData: (...a: unknown[]) => getUserData(...a),
  getSelfWorkspaces: vi.fn().mockResolvedValue(undefined),
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

  it('does not ask /envs for a visitor who is not signed in', async () => {
    const store = useUserStore()
    await store.ensureSessionOnce()
    expect(store.session).toBe('anonymous')
    expect(getEnvs).not.toHaveBeenCalled()
  })
})
