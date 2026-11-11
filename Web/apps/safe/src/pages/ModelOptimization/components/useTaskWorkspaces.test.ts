import { AxiosError, AxiosHeaders, type InternalAxiosRequestConfig } from 'axios'
import { createPinia, setActivePinia } from 'pinia'
import { reactive } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CreateOptimizationTaskPayload } from '@/services/model-optimization/type'
import type { UserSelfData } from '@/services/login/type'
import type { WorkspaceItem } from '@/services/base/type'

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

// The HTTP layer, so the real optimization service and the real self-workspaces loader run.
const http = { get: vi.fn(), post: vi.fn() }
vi.mock('@/services/request', () => ({
  default: {
    get: (...a: unknown[]) => http.get(...a),
    post: (...a: unknown[]) => http.post(...a),
    defaults: { baseURL: '/api/v1' },
  },
}))
vi.mock('element-plus', () => ({ ElMessage: () => {}, ElMessageBox: {} }))

import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import { newTaskForm, workspaceChoices } from './taskWorkspace'
import { useTaskWorkspaces } from './useTaskWorkspaces'

const refused = (message: string) => {
  const config = { headers: new AxiosHeaders() } as InternalAxiosRequestConfig
  return new AxiosError('Request failed with status code 403', 'ERR_BAD_REQUEST', config, null, {
    status: 403,
    statusText: 'Forbidden',
    headers: {},
    config,
    data: { errorCode: 'Primus.00004', errorMessage: message },
  })
}

function setup(
  opts: {
    roles?: string[]
    current?: string
    all?: { workspaceId: string; workspaceName: string }[]
  } = {},
) {
  setActivePinia(createPinia())
  const user = useUserStore()
  user.$patch({ session: 'authenticated', userId: 'u-1' })
  user.profile = {
    id: 'u-1',
    name: 'eval',
    roles: opts.roles ?? ['default'],
    workspaces: [{ id: 'from-record', name: 'from-record' }],
    creationTime: '',
    restrictedType: 0,
  } as unknown as UserSelfData
  const ws = useWorkspaceStore()
  ws.items = (opts.all ?? []) as unknown as WorkspaceItem[]
  ws.currentWorkspaceId = opts.current
  const form = reactive<CreateOptimizationTaskPayload>(newTaskForm(''))
  return { form, ...useTaskWorkspaces(form) }
}

beforeEach(() => {
  http.get.mockReset()
  http.post.mockReset()
})

describe("the create form offers the caller's own workspaces", () => {
  it('lists GET /users/self/workspaces (members, then managed) and preselects the current one', async () => {
    http.get.mockResolvedValue({
      workspaces: [
        { id: 'ws-a', name: 'Team A' },
        { id: 'ws-b', name: 'ws-b' },
      ],
      managedWorkspaces: [
        { id: 'ws-b', name: 'ws-b' },
        { id: 'ws-m', name: 'Managed' },
      ],
    })
    const s = setup({ current: 'ws-b' })
    await s.load()
    expect(http.get).toHaveBeenCalledWith('/users/self/workspaces', { skipErrorHandler: true })
    expect(s.options.value).toEqual([
      { id: 'ws-a', label: 'Team A (ws-a)' },
      { id: 'ws-b', label: 'ws-b' },
      { id: 'ws-m', label: 'Managed (ws-m)' },
    ])
    expect(s.form.workspace).toBe('ws-b')
  })

  it("preselects the first workspace when the current one is not the caller's, and never a fixed one", async () => {
    http.get.mockResolvedValue({
      workspaces: [{ id: 'ws-a', name: 'ws-a' }],
      managedWorkspaces: [],
    })
    const s = setup({ current: 'someone-elses' })
    expect(s.form.workspace).toBe('')
    await s.load()
    expect(s.form.workspace).toBe('ws-a')
    expect(newTaskForm('x').workspace).toBe('x')
    expect(JSON.stringify(newTaskForm(''))).not.toContain('core42-sandbox')
  })

  it('falls back to the user record when the route is unavailable', async () => {
    http.get.mockRejectedValue(new Error('404'))
    const s = setup()
    await s.load()
    expect(s.options.value).toEqual([{ id: 'from-record', label: 'from-record' }])
    expect(s.form.workspace).toBe('from-record')
  })

  it('offers nothing to a user with no workspace', async () => {
    http.get.mockResolvedValue({ workspaces: [], managedWorkspaces: [] })
    const s = setup({ all: [{ workspaceId: 'ws-x', workspaceName: 'X' }] })
    await s.load()
    expect(s.options.value).toEqual([])
    expect(s.form.workspace).toBe('')
  })

  it('adds every listed workspace for a system administrator only', async () => {
    http.get.mockResolvedValue({
      workspaces: [{ id: 'ws-a', name: 'ws-a' }],
      managedWorkspaces: [],
    })
    const s = setup({
      roles: ['system-admin'],
      all: [
        { workspaceId: 'ws-x', workspaceName: 'X' },
        { workspaceId: 'ws-a', workspaceName: 'ws-a' },
      ],
    })
    await s.load()
    expect(s.options.value.map((o) => o.id)).toEqual(['ws-a', 'ws-x'])
    expect(workspaceChoices(null)).toEqual([])
  })
})

describe('submitting creates the task in the chosen workspace', () => {
  it('sends the chosen workspace, quietly so the form can explain a refusal', async () => {
    http.get.mockResolvedValue({
      workspaces: [
        { id: 'ws-a', name: 'ws-a' },
        { id: 'ws-b', name: 'ws-b' },
      ],
      managedWorkspaces: [],
    })
    http.post.mockResolvedValue({ id: 'opt-1' })
    const s = setup()
    await s.load()
    s.form.workspace = 'ws-b'
    s.form.modelId = 'm-1'
    const res = await s.submit()
    expect(res.task).toEqual({ id: 'opt-1' })
    expect(http.post).toHaveBeenCalledTimes(1)
    const [url, body, config] = http.post.mock.calls[0]
    expect(url).toBe('/optimization/tasks')
    expect(body.workspace).toBe('ws-b')
    expect(body.modelId).toBe('m-1')
    expect(body).not.toHaveProperty('image')
    expect(body).not.toHaveProperty('displayName')
    expect(config).toEqual({ skipErrorHandler: true })
    expect(s.submitError.value).toBe('')
  })

  it('shows why a workspace was refused (403), naming it', async () => {
    http.get.mockResolvedValue({
      workspaces: [{ id: 'ws-a', name: 'ws-a' }],
      managedWorkspaces: [],
    })
    http.post.mockRejectedValue(refused('not allowed to create workload in workspace ws-a'))
    const s = setup()
    await s.load()
    const res = await s.submit()
    expect(res.task).toBeUndefined()
    expect(s.submitError.value).toContain('"ws-a"')
    expect(s.submitError.value).toContain('not allowed to create workload in workspace ws-a')
  })

  it('leaves any other failure to the caller', async () => {
    http.get.mockResolvedValue({
      workspaces: [{ id: 'ws-a', name: 'ws-a' }],
      managedWorkspaces: [],
    })
    http.post.mockRejectedValue(new Error('Network Error'))
    const s = setup()
    await s.load()
    const res = await s.submit()
    expect(res.error).toBe('Network Error')
    expect(s.submitError.value).toBe('')
  })
})
