export interface ClusterItem {
  clusterId: string
  phase: string
  isProtected: boolean
}

export interface WorkspaceVolume {
  id: number
  type: string
  mountPath: string
  accessMode: string
}

export interface WorkspaceItem {
  workspaceName: string
  workspaceId: string
  flavorId: string
  currentNodeCount: number
  isDefault?: boolean
  managers?: string[]
  scopes: ScopesKeys[]
  clusterId?: string
  gpuProduct?: string
  volumes?: WorkspaceVolume[]
}

export const SCOPES_KEYS = ['Train', 'Infer', 'Authoring', 'CICD', 'Ray', 'Sandbox'] as const
export type ScopesKeys = (typeof SCOPES_KEYS)[number]

// Scopes a user can select when creating/editing a workspace. Ray and Sandbox
// are experimental and excluded from GA, so they are not offered in the UI
// (they remain in ScopesKeys for backward compatibility with existing objects).
export const SELECTABLE_SCOPES = ['Train', 'Infer', 'Authoring', 'CICD'] as const
