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

// Edit + Create
export interface BaseSubmitWsData {
  description?: string
  flavorId: string
  replica?: number
  queuePolicy?: QueuePolicy
  enablePreempt?: boolean
  isDefault?: boolean
  managers?: string[]
  volumes?: Volume[]
  maxRuntime?: Record<string, number>
  idleTime?: Record<string, string>
}

export interface SubmitWorkspaceRequest extends BaseSubmitWsData {
  name: string
  clusterId: string
}
