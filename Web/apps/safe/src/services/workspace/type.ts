// export type StorageType = 'rbd' | 'obs' | 'cephfs' | 'hostpath' | 'juicefs'
export type VolumeType = 'pfs' | 'hostpath'
export type AccessMode = 'ReadWriteOnce' | 'ReadOnlyMany' | 'ReadWriteMany' | 'ReadWriteOncePod'
export type CapUnit = 'Pi' | 'Ti' | 'Gi'
export type QueuePolicy = 'fifo' | 'balance'
export interface SelectorKV {
  key: string
  value: string
}
export type UserT = { id: string; name: string }
type VolumeCommon = {
  uid: string
  type: VolumeType
  mountPath: string
  enableUserDir?: boolean
}
type VolumeHostPath = VolumeCommon & {
  type: 'hostpath'
  hostPath: string
  accessMode?: AccessMode
  disabled?: boolean
}
type VolumeBlock = VolumeCommon & {
  // storageType: Exclude<StorageType, 'hostpath'>
  type: Exclude<VolumeType, 'hostpath'>
  subPath: string
  capacity: string
  capacityAppend: string // Additional selectable suffix unit
  storageClass: string
  accessMode: AccessMode
  provisioningStrategy?: string
  selector?: Record<string, string>
  selectorKV?: SelectorKV
  disabled?: boolean
}
export type Volume = VolumeHostPath | VolumeBlock
export type VolumeWithStrategy = Volume & {
  provisioningStrategy?: string
  selectorKV?: SelectorKV
  storageClass?: string
  selector?: Record<string, string>
  disabled?: boolean
}
export type PfsVolumeWithStrategy = Extract<VolumeWithStrategy, { type: 'pfs' }>

export type PersistentVolumeItem = {
  labels?: Record<string, string>
  storageClassName?: string
  capacity?: { storage?: string }
  accessModes?: AccessMode[]
}
export type PersistentVolumeResponse = {
  totalCount?: number
  items?: PersistentVolumeItem[] | null
}
export type PvPrefill = {
  labelKV?: SelectorKV
  storageClassName?: string
  capacity?: { value: string; unit: CapUnit }
  accessMode?: AccessMode
}

export type LowUtilizationMode = 'Average' | 'Min'
export interface LowUtilizationRule {
  threshold: number
  window: string
}
export interface LowUtilizationPolicy {
  // Legacy single rule, only used when rules is empty
  threshold?: number
  window?: string
  rules?: LowUtilizationRule[]
  mode: LowUtilizationMode
  gracePeriod?: string
  notifyBefore?: string
  cooldown?: string
  dryRun?: boolean
}

// Edit + Create
export interface BaseSubmitWsData {
  description?: string
  flavorId: string
  replica?: number
  queuePolicy?: QueuePolicy
  enablePreempt?: boolean
  external?: boolean
  isDefault?: boolean
  managers?: string[]
  volumes?: Volume[]
  idleTime?: Record<string, string>
  lowUtilization?: Record<string, LowUtilizationPolicy>
}

export interface SubmitWorkspaceRequest extends BaseSubmitWsData {
  name: string
  clusterId: string
  // Teams whose members access the workspace. A manager binding needs a system administrator
  teams?: WorkspaceTeam[]
  accessPolicy?: WorkspaceAccessPolicy
}

export type WorkspaceTeamRole = 'member' | 'manager'
export type WorkspaceAccessPolicy = 'closed' | 'approval'

// A team bound to a workspace
export interface WorkspaceTeam {
  name: string
  role?: WorkspaceTeamRole
}

// PUT /workspaces/:name/teams. `add` of a bound team changes its role; `remove` holds team ids.
export interface UpdateWorkspaceTeamsRequest {
  add?: WorkspaceTeam[]
  remove?: string[]
}

export interface UpdateWorkspaceTeamsResp {
  teams: WorkspaceTeam[]
}

// A user with direct access to a workspace
export interface WorkspaceMemberUser {
  id: string
  name: string
  email: string
  preferredName?: string
  role: WorkspaceTeamRole
  source: string
}

// A team manager as the workspace members view describes it
export interface WorkspaceMemberManager {
  id: string
  name?: string
  email?: string
  preferredName?: string
}

// A team bound to a workspace. Everything but id and role is absent when the identity service is unreachable.
export interface WorkspaceMemberTeam {
  id: string
  displayName?: string
  role: WorkspaceTeamRole
  memberCount?: number
  managers?: WorkspaceMemberManager[]
  source: string
}

// A platform group whose mapping reaches a workspace
export interface WorkspaceMemberGroup {
  name: string
  role: WorkspaceTeamRole
  source: string
}

// GET /workspaces/:name/members
export interface WorkspaceMembersResp {
  users: WorkspaceMemberUser[]
  teams: WorkspaceMemberTeam[]
  groups: WorkspaceMemberGroup[]
}
