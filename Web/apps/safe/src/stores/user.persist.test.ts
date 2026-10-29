import { createApp } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import piniaPersist from 'pinia-plugin-persistedstate'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// The other store tests build pinia without the persistence plugin, so they cannot see what is
// saved to localStorage or what a reload restores. This one installs the real plugin, as main.ts
// does, and plays a first visit and a reload against one shared storage.
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

const record = (id: string) => ({
  id,
  name: `user-${id}`,
  roles: ['default'],
  workspaces: [],
  managedWorkspaces: [],
})

// Writes to storage happen after the state change that causes them.
const flush = () => new Promise((r) => setTimeout(r, 0))

// One page load: a fresh pinia with the plugin, which restores what the last load saved. As in
// main.ts the pinia is installed in an app: pinia holds a plugin back until then.
const loadPage = () => {
  const pinia = createPinia()
  pinia.use(piniaPersist)
  createApp({}).use(pinia)
  setActivePinia(pinia)
  return useUserStore(pinia)
}

describe('what the user store saves, and what a reload does with it', () => {
  beforeEach(() => {
    localStorage.clear()
    getUserData.mockReset().mockImplementation(async (id: string) => record(id))
    getEnvs.mockReset().mockResolvedValue({ authMode: 'external' })
    getSelfData.mockReset()
    getSelfWorkspaces.mockReset().mockResolvedValue(undefined)
  })

  it('saves the session, the user, the profile and the onboarding state, and nothing else', async () => {
    getSelfData.mockRejectedValue({ response: { status: 401 } })
    const store = loadPage()
    await store.ensureSessionOnce()
    await flush()

    const saved = JSON.parse(localStorage.getItem('user') ?? '{}')
    // `_initPromise` is a promise: saved, it comes back as `{}`, which is truthy.
    expect(Object.keys(saved).sort()).toEqual(['onboarding', 'profile', 'session', 'userId'])
  })

  it('asks the server who the caller is on the reload after signing in at the identity provider', async () => {
    // First visit: nobody is signed in at the provider yet.
    getSelfData.mockRejectedValue({ response: { status: 401 } })
    const first = loadPage()
    await first.ensureSessionOnce()
    await flush()
    expect(first.session).toBe('anonymous')
    expect(getSelfData).toHaveBeenCalledTimes(1)

    // The browser goes to the provider and comes back: a new page load, the session now exists.
    getSelfData.mockReset().mockResolvedValue(record('u1'))
    const second = loadPage()
    await second.ensureSessionOnce()

    expect(getSelfData).toHaveBeenCalledTimes(1)
    expect(second.session).toBe('authenticated')
    expect(second.userId).toBe('u1')
  })

  it('shows the account that signed in at the provider, not the one the last visit saved', async () => {
    getSelfData.mockResolvedValue(record('u1'))
    const first = loadPage()
    await first.ensureSessionOnce()
    await flush()
    expect(first.userId).toBe('u1')

    // The first account's provider session ended and another account signed in.
    getSelfData.mockReset().mockResolvedValue(record('u2'))
    const second = loadPage()
    expect(second.userId).toBe('u1') // restored from storage, until the server answers
    await second.ensureSessionOnce()

    expect(getUserData).not.toHaveBeenCalled()
    expect(second.userId).toBe('u2')
    expect(second.profile?.name).toBe('user-u2')
  })

  it('verifies a saved builtin session on the reload instead of trusting storage', async () => {
    getEnvs.mockResolvedValue({ authMode: 'builtin' })
    const first = loadPage()
    first.userId = 'u1'
    first.session = 'authenticated'
    await first.ensureSessionOnce()
    await flush()
    expect(getUserData).toHaveBeenCalledTimes(1)

    const second = loadPage()
    await second.ensureSessionOnce()
    expect(getUserData).toHaveBeenCalledTimes(2)
    expect(second.isExternalAuth).toBe(false)
    expect(getEnvs).toHaveBeenCalledTimes(2)
  })
})
