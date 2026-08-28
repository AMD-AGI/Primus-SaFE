import type { WorkspaceItem } from '@/services'

export const NODE_BIND_ACTIONS = ['add', 'remove', 'migrate'] as const
export type NodeBindAction = (typeof NODE_BIND_ACTIONS)[number]

export interface NodeRelateRequest {
  action: NodeBindAction
  nodeIds: string[]
  targetWorkspaceId?: string
  force?: boolean
}

/**
 * Which workspace the request is addressed to.
 *
 * Add is a request made of the workspace taking the node on, so it goes to the target. Remove
 * and migrate are requests made of the workspace giving it up, so they go to the source -- a
 * migration addressed to its target would be read as that workspace trying to migrate a node
 * it does not have.
 */
export const relateWorkspaceId = (
  action: NodeBindAction,
  sourceWorkspaceId: string,
  targetWorkspaceId: string,
): string => (action === 'add' ? targetWorkspaceId : sourceWorkspaceId)

/**
 * The request body. Migrate is the only action carrying a target, and the API refuses one on
 * any other action rather than ignoring it, so it is only ever set here for migrate.
 */
export const buildNodeRelateRequest = (params: {
  action: NodeBindAction
  nodeIds: string[]
  targetWorkspaceId: string
  force: boolean
}): NodeRelateRequest => {
  const { action, nodeIds, targetWorkspaceId, force } = params
  const request: NodeRelateRequest = { action, nodeIds }
  if (action === 'migrate') {
    request.targetWorkspaceId = targetWorkspaceId
  }
  if (action === 'remove' || action === 'migrate') {
    // Both take the node away from whatever is running on it, and the API gates both on the
    // same check.
    request.force = force
  }
  return request
}

/**
 * The workspaces a node may be migrated to: another workspace in the same cluster.
 *
 * Cluster only. Whether a target can take a given flavor is a rule the API owns, and it has
 * more to it than it looks -- a workspace scaled to zero may take any flavor, whatever it
 * still records -- so a copy of it here is a copy that goes out of date, and it goes out of
 * date by hiding options that would in fact have worked. What is left is the one condition
 * this side can be sure of, and the API answers the rest where its answer can say why.
 *
 * The source workspace stands in for the node's own cluster, which the node list does not
 * carry. When it cannot be found -- a node in a workspace the viewer has no access to --
 * nothing is filtered rather than everything: an empty list with no explanation is a worse
 * answer than a list the API will narrow down itself.
 */
export const migrateTargetOptions = (
  workspaces: WorkspaceItem[],
  sourceWorkspaceId: string,
): WorkspaceItem[] => {
  const others = (workspaces || []).filter((ws) => ws.workspaceId !== sourceWorkspaceId)
  const source = (workspaces || []).find((ws) => ws.workspaceId === sourceWorkspaceId)
  if (!source) {
    return others
  }
  // Both sides have to actually say which cluster they are in. clusterId is optional on the
  // list response, and treating two absences as a match turns the one condition this side is
  // sure of into no condition at all -- every workspace in the system, other clusters
  // included, offered as somewhere this node could go.
  if (!source.clusterId) {
    return others
  }
  return others.filter((ws) => ws.clusterId === source.clusterId)
}
