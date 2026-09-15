import { toValue, type MaybeRefOrGetter } from 'vue'
import { ElMessage } from 'element-plus'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { useUserStore } from '@/stores/user'

dayjs.extend(utc)

type ResumePermissionRow = {
  phase?: string
  workspaceId?: string
  workspace?: string
  userId?: string
}

export const RESUMABLE_PHASES = ['Stopped', 'Failed', 'Succeeded']

export const RESUME_COOLDOWN_SECONDS = 10

/**
 * The backend rejects a resume that lands too soon after the stop it follows.
 * Warns and returns false while the workload is still inside that window, so
 * list pages and detail pages gate the action the same way.
 */
export function ensureResumeCooldownElapsed(endTime?: string): boolean {
  if (!endTime) return true
  if (dayjs().diff(dayjs.utc(endTime), 'second') >= RESUME_COOLDOWN_SECONDS) return true

  ElMessage.warning(
    `Please wait ${RESUME_COOLDOWN_SECONDS} seconds after stopping before resuming the workload.`,
  )
  return false
}

export function useWorkloadResumePermission(canWrite?: MaybeRefOrGetter<boolean>) {
  const userStore = useUserStore()

  const getRowWorkspaceId = (row: ResumePermissionRow): string | undefined =>
    row.workspaceId || row.workspace

  const isManagedWorkspace = (row: ResumePermissionRow): boolean => {
    const workspaceId = getRowWorkspaceId(row)
    if (!workspaceId) return false
    const managedWorkspaces = userStore.profile?.managedWorkspaces ?? []
    return managedWorkspaces.some((workspace) =>
      workspace.id === workspaceId || workspace.name === workspaceId,
    )
  }

  const isWorkloadOwner = (row: ResumePermissionRow): boolean =>
    !!row.userId && !!userStore.userId && row.userId === userStore.userId

  const canResumeWorkload = (row: ResumePermissionRow): boolean =>
    userStore.isManager || isManagedWorkspace(row) || isWorkloadOwner(row)

  const getResumeDisabled = (row: ResumePermissionRow): boolean =>
    (canWrite !== undefined && !toValue(canWrite)) ||
    !RESUMABLE_PHASES.includes(row.phase ?? '') ||
    !canResumeWorkload(row)

  const getResumeTooltip = (row: ResumePermissionRow): string => {
    if (!RESUMABLE_PHASES.includes(row.phase ?? '')) {
      return 'Resume is unavailable for this workload state'
    }
    if (!canResumeWorkload(row)) {
      return 'Only queue managers or the workload owner can resume this workload'
    }
    return 'Resume'
  }

  return {
    canResumeWorkload,
    getResumeDisabled,
    getResumeTooltip,
    getRowWorkspaceId,
  }
}
