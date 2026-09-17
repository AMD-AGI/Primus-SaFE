import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ElMessage } from 'element-plus'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { useUserStore } from '@/stores/user'
import {
  useWorkloadResumePermission,
  ensureResumeCooldownElapsed,
  RESUME_COOLDOWN_SECONDS,
} from './useWorkloadResumePermission'

dayjs.extend(utc)

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

describe('ensureResumeCooldownElapsed', () => {
  const utcEndTime = (secondsAgo: number) =>
    dayjs.utc().subtract(secondsAgo, 'second').format('YYYY-MM-DD HH:mm:ss')

  it('blocks and warns while the workload is still inside the cooldown window', () => {
    const warning = vi.spyOn(ElMessage, 'warning').mockImplementation(() => undefined as never)

    expect(ensureResumeCooldownElapsed(utcEndTime(RESUME_COOLDOWN_SECONDS - 5))).toBe(false)
    expect(warning).toHaveBeenCalledTimes(1)

    warning.mockRestore()
  })

  it('allows the resume once the cooldown elapsed or the workload never ended', () => {
    expect(ensureResumeCooldownElapsed(utcEndTime(RESUME_COOLDOWN_SECONDS + 5))).toBe(true)
    expect(ensureResumeCooldownElapsed(undefined)).toBe(true)
  })
})
