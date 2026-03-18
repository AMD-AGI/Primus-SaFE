export interface ClusterItem {
  clusterId: string
  phase: string
  isProtected: boolean
}

export interface WorkspaceItem {
  workspaceName: string
  workspaceId: string
  flavorId: string
  currentNodeCount: number
  isDefault?: boolean
  managers?: string[]
  scopes: ScopesKeys[]
  clusterId?: string // Cluster the workspace belongs to
}

export const SCOPES_KEYS = ['Train', 'Infer', 'Authoring', 'CICD', 'Ray'] as const
export type ScopesKeys = (typeof SCOPES_KEYS)[number]
