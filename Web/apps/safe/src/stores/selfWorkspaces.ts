import { getSelfWorkspaces } from '@/services/login'
import type { SelfWorkspacesResp, UserSelfData, Workspace } from '@/services/login/type'

/**
 * The console learns which workspaces a user can access and manage from
 * GET /users/self/workspaces. Those lists also ride along in the user record, but a record
 * that SaFE does not serve itself (for example one an external authenticator answers for
 * /users/self) does not carry them, and the workspace administrator entries depend on them.
 *
 * Build with VITE_SELF_WORKSPACES=false to read the lists embedded in the user record
 * instead (the way older consoles did).
 */
export const isSelfWorkspacesEnabled = (): boolean => import.meta.env.VITE_SELF_WORKSPACES !== 'false'

const isWorkspaceList = (v: unknown): v is Workspace[] => Array.isArray(v)

/**
 * Ask the server for the caller's workspace lists. Returns undefined when the switch is
 * off, the route is missing or refused, or the answer is not the expected shape: the
 * caller then keeps the lists of the user record.
 */
export async function loadSelfWorkspaces(
  load: () => Promise<SelfWorkspacesResp> = getSelfWorkspaces,
  enabled: boolean = isSelfWorkspacesEnabled(),
): Promise<SelfWorkspacesResp | undefined> {
  if (!enabled) return undefined
  try {
    const res = await load()
    if (!res || (!isWorkspaceList(res.workspaces) && !isWorkspaceList(res.managedWorkspaces))) {
      return undefined
    }
    return res
  } catch (err) {
    console.warn('[user] workspace lists unavailable, using the user record:', err)
    return undefined
  }
}

/**
 * The user record with the route's lists in place of its embedded ones. A list the route
 * did not return keeps the record's own; the record passed in is not modified.
 */
export function applySelfWorkspaces(
  profile: UserSelfData,
  lists: SelfWorkspacesResp | undefined,
): UserSelfData {
  if (!lists) return profile
  return {
    ...profile,
    workspaces: isWorkspaceList(lists.workspaces) ? lists.workspaces : profile.workspaces,
    managedWorkspaces: isWorkspaceList(lists.managedWorkspaces)
      ? lists.managedWorkspaces
      : profile.managedWorkspaces,
  }
}
