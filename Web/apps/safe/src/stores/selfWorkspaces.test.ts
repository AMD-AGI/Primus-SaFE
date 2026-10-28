import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

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
const getSelfWorkspaces = vi.fn()

vi.mock('element-plus', () => ({ ElMessage: () => {}, ElMessageBox: {} }))
vi.mock('@/services/login', () => ({
  login: vi.fn(),
  logout: vi.fn(),
  getEnvs: vi.fn(),
  getUserData: (...a: unknown[]) => getUserData(...a),
  getSelfWorkspaces: (...a: unknown[]) => getSelfWorkspaces(...a),
}))
vi.mock('@/services/base', () => ({ getWorkspace: vi.fn() }))

import { applySelfWorkspaces, isSelfWorkspacesEnabled, loadSelfWorkspaces } from './selfWorkspaces'
import { useUserStore } from './user'
import { useWorkspaceStore } from './workspace'
import type { SelfWorkspacesResp, UserSelfData } from '@/services/login/type'

const ws = (id: string, name = id) => ({ id, name })

const record = (over: Partial<UserSelfData> = {}): UserSelfData => ({
  id: 'u1',
  name: 'user-one',
  email: 'u1@example.com',
  type: 'default',
  roles: ['default'],
  workspaces: [ws('ws-embedded')],
  managedWorkspaces: [],
  creationTime: '2026-01-01T00:00:00',
  restrictedType: 0,
  ...over,
})

describe('isSelfWorkspacesEnabled', () => {
  afterEach(() => vi.unstubAllEnvs())

  it.each([
    [undefined, true],
    ['', true],
    ['true', true],
    ['false', false],
  ])('VITE_SELF_WORKSPACES=%s -> %s', (value, want) => {
    vi.stubEnv('VITE_SELF_WORKSPACES', value as string)
    expect(isSelfWorkspacesEnabled()).toBe(want)
  })
})

describe('applySelfWorkspaces', () => {
  const lists = { workspaces: [ws('ws-a', 'Team A')], managedWorkspaces: [ws('ws-a', 'Team A')] }

  it('puts the route lists in place of the embedded ones', () => {
    const out = applySelfWorkspaces(record(), lists)
    expect(out.workspaces).toEqual(lists.workspaces)
    expect(out.managedWorkspaces).toEqual(lists.managedWorkspaces)
  })

  it('shows an empty route list as empty, not as the embedded one', () => {
    const out = applySelfWorkspaces(record(), { workspaces: [], managedWorkspaces: [] })
    expect(out.workspaces).toEqual([])
    expect(out.managedWorkspaces).toEqual([])
  })

  it('keeps the record when the route gave nothing', () => {
    const rec = record()
    expect(applySelfWorkspaces(rec, undefined)).toBe(rec)
  })

  it('keeps the record list for a field the route did not return as a list', () => {
    const out = applySelfWorkspaces(record(), { workspaces: null, managedWorkspaces: [ws('m')] } as unknown as SelfWorkspacesResp)
    expect(out.workspaces).toEqual([ws('ws-embedded')])
    expect(out.managedWorkspaces).toEqual([ws('m')])
  })

  it('does not modify the record passed in', () => {
    const rec = record()
    applySelfWorkspaces(rec, lists)
    expect(rec.workspaces).toEqual([ws('ws-embedded')])
    expect(rec.managedWorkspaces).toEqual([])
  })
})

describe('loadSelfWorkspaces', () => {
  const answer = { workspaces: [ws('a')], managedWorkspaces: [ws('b')] }

  beforeEach(() => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })
  afterEach(() => vi.restoreAllMocks())

  it('asks the route and returns its lists', async () => {
    const load = vi.fn().mockResolvedValue(answer)
    await expect(loadSelfWorkspaces(load, true)).resolves.toEqual(answer)
    expect(load).toHaveBeenCalledTimes(1)
  })

  it('does not call the route when the switch is off', async () => {
    const load = vi.fn().mockResolvedValue(answer)
    await expect(loadSelfWorkspaces(load, false)).resolves.toBeUndefined()
    expect(load).not.toHaveBeenCalled()
  })

  it.each([
    ['a refusal', () => Promise.reject(new Error('404'))],
    ['an empty answer', () => Promise.resolve(undefined)],
    ['an answer of another shape', () => Promise.resolve({ items: [] })],
    ['a string', () => Promise.resolve('<html></html>')],
  ])('falls back on %s', async (_name, load) => {
    await expect(loadSelfWorkspaces(load as unknown as () => Promise<SelfWorkspacesResp>, true)).resolves.toBeUndefined()
  })
})

describe('the user store takes the workspace lists from the route', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getUserData.mockReset()
    getSelfWorkspaces.mockReset()
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllEnvs()
  })

  // A user record that SaFE did not serve itself carries no workspace lists; the workspace
  // administrator entry of the console comes from the route.
  it('shows the management entry of a workspace administrator whose record has no lists', async () => {
    getUserData.mockResolvedValue(record({ workspaces: undefined as unknown as UserSelfData['workspaces'], managedWorkspaces: undefined }))
    getSelfWorkspaces.mockResolvedValue({
      workspaces: [ws('ws-a', 'Team A')],
      managedWorkspaces: [ws('ws-a', 'Team A')],
    })
    const user = useUserStore()
    user.$patch({ userId: 'u1' })
    await user.fetchUser(true)

    const wsStore = useWorkspaceStore()
    wsStore.$patch({ currentWorkspaceId: 'ws-a' })
    expect(wsStore.isCurrentWorkspaceAdmin()).toBe(true)
    expect(user.displayRole).toBe('workspace-admin')
    expect(getSelfWorkspaces).toHaveBeenCalledTimes(1)
  })

  it('shows no management entry for a plain member', async () => {
    getUserData.mockResolvedValue(record())
    getSelfWorkspaces.mockResolvedValue({ workspaces: [ws('ws-a', 'Team A')], managedWorkspaces: [] })
    const user = useUserStore()
    user.$patch({ userId: 'u1' })
    await user.fetchUser(true)

    const wsStore = useWorkspaceStore()
    wsStore.$patch({ currentWorkspaceId: 'ws-a' })
    expect(wsStore.isCurrentWorkspaceAdmin()).toBe(false)
  })

  it('falls back to the lists of the user record when the route fails', async () => {
    getUserData.mockResolvedValue(record({ managedWorkspaces: [ws('ws-embedded')] }))
    getSelfWorkspaces.mockRejectedValue(new Error('404'))
    const user = useUserStore()
    user.$patch({ userId: 'u1' })
    await user.fetchUser(true)

    expect(user.profile?.workspaces).toEqual([ws('ws-embedded')])
    const wsStore = useWorkspaceStore()
    wsStore.$patch({ currentWorkspaceId: 'ws-embedded' })
    expect(wsStore.isCurrentWorkspaceAdmin()).toBe(true)
    expect(user.session).toBe('authenticated')
  })

  it('reads the user record only when the switch is off', async () => {
    vi.stubEnv('VITE_SELF_WORKSPACES', 'false')
    getUserData.mockResolvedValue(record({ managedWorkspaces: [ws('ws-embedded')] }))
    getSelfWorkspaces.mockResolvedValue({ workspaces: [], managedWorkspaces: [] })
    const user = useUserStore()
    user.$patch({ userId: 'u1' })
    await user.fetchUser(true)

    expect(getSelfWorkspaces).not.toHaveBeenCalled()
    expect(user.profile?.managedWorkspaces).toEqual([ws('ws-embedded')])
  })

  it('does not hide a failure of the user record itself', async () => {
    getUserData.mockRejectedValue(new Error('401'))
    getSelfWorkspaces.mockResolvedValue({ workspaces: [], managedWorkspaces: [] })
    const user = useUserStore()
    user.$patch({ userId: 'u1' })
    await expect(user.fetchUser(true)).rejects.toThrow('401')
    expect(user.profile).toBeNull()
  })
})
