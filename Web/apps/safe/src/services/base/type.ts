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

export const SCOPES_KEYS = ['Train', 'Infer', 'Authoring', 'CICD', 'Ray', 'Sandbox', 'Slurm'] as const
export type ScopesKeys = (typeof SCOPES_KEYS)[number]
