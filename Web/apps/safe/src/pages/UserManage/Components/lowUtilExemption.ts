import type { UserLowUtilization, UserLowUtilizationScope } from '@/services'

/** exemptWorkspaceIds lists the workspaces where the user is exempt from the low utilization policy. */
export function exemptWorkspaceIds(settings?: UserLowUtilization): string[] {
  return Object.entries(settings?.workspaces ?? {})
    .filter(([id, scope]) => id && scope?.exempt)
    .map(([id]) => id)
    .sort()
}

/** buildUserLowUtilization is the PATCH value for the selected workspaces. No selection clears the settings. */
export function buildUserLowUtilization(ids: readonly string[]): UserLowUtilization {
  const workspaces: Record<string, UserLowUtilizationScope> = {}
  for (const id of ids) {
    if (id) workspaces[id] = { exempt: true }
  }
  return Object.keys(workspaces).length ? { workspaces } : {}
}
