export interface TemplateOptionsType {
  templateId: string
  addOnTemplates: string[]
}

// **********NodeFlavor

export interface FlavorItemResp {
  totalCount: number
  items: FlavorOptionsType[]
}

export type DiskType = 'ssd' | 'nvme' | 'hdd' | string

export interface DiskSpec {
  type: DiskType
  quantity: string
  count: number
}

export interface ExtendsSpec {
  'ephemeral-storage'?: string
  'rdma/hca'?: string
}

export interface CPUSpec {
  product: string
  quantity: string
}

export interface GPUSpec extends CPUSpec {
  resourceName: string
}

export interface BaseFlavor {
  cpu: CPUSpec
  gpu: GPUSpec
  memory: string
  rootDisk: DiskSpec
  dataDisk: DiskSpec
  extendedResources: ExtendsSpec
}
// List response values
export interface FlavorOptionsType extends BaseFlavor {
  flavorId: string
}
// createPass parameters
export interface CreateFlavorPayload extends BaseFlavor {
  name: string
}

export interface PatchNodeFlavorRequest {
  cpu?: number
  cpuProduct?: string
  memory?: number
  rootDisk?: DiskSpec
  dataDisk?: DiskSpec
  extends?: ExtendsSpec
}

// **********Secret

export type SSHParam = {
  username: string
  privateKey: string
  publicKey: string
}
export type ImageParam = {
  username: string
  server: string
  password: string
}

export interface SecretOptionsType {
  secretId: string
  secretName: string
  type: string
}
export interface CreateSecretPayload {
  name: string
  type: string
  bindAllWorkspaces: boolean
  params: Array<SSHParam | ImageParam>
}

export interface GetNodePodLogResponse {
  clusterId: string
  nodeId: string
  podId: string
  logs: string[]
}

export const TAINT_EFFECTS = ['NoSchedule', 'PreferNoSchedule', 'NoExecute'] as const

export type TaintEffects = (typeof TAINT_EFFECTS)[number]

export interface Taint {
  key: string
  effect: TaintEffects
  timeAdded?: string
}

export interface TaintListItem {
  key: TaintEffects
  value: string
  _uid?: string
}

export interface KeyValueOption {
  key: string
  value: string
}

export const taintOptions = TAINT_EFFECTS.map((v) => ({ label: v, value: v }))

export interface NodeEditData {
  labels?: Record<string, string>
  taints?: Taint[]
  flavorId: string
  templateId: string
  port: number
  privateIP: string
}

export interface NodesParams {
  clusterId?: string
  workspaceId?: string

  brief?: boolean
  nodeId?: string
  available?: boolean
  isAddonsInstalled?: boolean
  phase?: string
  search?: string

  offset?: number
  limit?: number
}

// new cluster
export interface CreateClusterPayload {
  name: string
  description?: string
  sshSecretId: string
  isProtected?: boolean
  kubeNetworkPlugin: string
  nodes: string[] // e.g. ["smc300x-ccs-aus-a16-10", ...]
  kubeSprayImage: string
  kubePodsSubnet: string // e.g. "10.0.0.0/16"
  kubeServiceAddress: string // e.g. "10.254.0.0/16"
  kubernetesVersion: string
  kubeApiServerArgs?: Record<string, string>
}
export const NODE_PHASE = [
  'Ready',
  'SSHFailed',
  'HostnameFailed',
  'Managing',
  'ManagedFailed',
  'Unmanaging',
  'UnmanagedFailed',
] as const

export interface RebootNodesParams {
  type: string
  name: string
  inputs: { name: string; value: string }[]
  timeoutSecond?: number
}

export interface NodeRebootData {
  sinceTime?: string
  untilTime?: string
  offset?: number
  limit?: number
  order?: string
  sortBy?: string
}
export interface RebootLogsItem {
  userId: string
  userName: string
  creationTime: string
}
export interface GetRebootLogsResp {
  totalCount: number
  items: RebootLogsItem[]
}
