import { toValue, type MaybeRefOrGetter } from 'vue'
import { useUserStore } from '@/stores/user'

type ResumePermissionRow = {
  phase?: string
  workspaceId?: string
  workspace?: string
  userId?: string
}

const RESUMABLE_PHASES = ['Stopped', 'Failed', 'Succeeded']

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
    isManagedWorkspace(row) || isWorkloadOwner(row)

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
