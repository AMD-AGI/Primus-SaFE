import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useUserStore } from '@/stores/user'
import { useWorkloadResumePermission } from './useWorkloadResumePermission'

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

describe('useWorkloadResumePermission', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('allows system administrators to resume workloads outside managed workspaces', () => {
    const userStore = useUserStore()
    userStore.$patch({
      userId: 'admin-user',
      profile: {
        id: 'admin-user',
        roles: ['system-admin'],
        managedWorkspaces: [],
      },
    })

    const { canResumeWorkload, getResumeDisabled, getResumeTooltip } = useWorkloadResumePermission(true)
    const row = {
      phase: 'Stopped',
      workspaceId: 'workspace-owned-by-someone-else',
      userId: 'workload-owner',
    }

    expect(canResumeWorkload(row)).toBe(true)
    expect(getResumeDisabled(row)).toBe(false)
    expect(getResumeTooltip(row)).toBe('Resume')
  })
})
