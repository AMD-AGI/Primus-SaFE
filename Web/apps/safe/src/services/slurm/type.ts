// A "Slurm cluster" is a per-workspace Helm release of the Slinky `slurm` chart
// (v1.2.0) deployed into the workspace namespace via the Addon mechanism. It is
// modeled around node pools: each pool becomes a NodeSet (slurmd) + a Slurm
// partition of the same name.

export interface NodePool {
  name: string
  nodes: number
  gpu?: number
  cpu?: string
  memory?: string
}

// SlurmPod is a live pod belonging to a cluster's helm release (detail view).
export interface SlurmPod {
  name: string
  role: string
  node?: string
  phase: string
  podIP?: string
  hostIP?: string
}

export interface SlurmClusterItem {
  name: string
  workspace: string
  namespace: string
  cluster: string
  phase: string
  accountingEnabled: boolean
  pools?: NodePool[]
  partitions?: string[]
  nodesReady: number
  nodesDesired: number
  stopped?: boolean
  imageTag?: string
  description?: string
  pods?: SlurmPod[]
  creationTime: string
}

export interface SlurmClusterListResp {
  items: SlurmClusterItem[]
  totalCount: number
}

export interface CreateSlurmClusterData {
  workspaceId: string
  name: string
  accountingEnabled?: boolean
  pools: NodePool[]
  imageTag?: string
  description?: string
}

export interface EditSlurmClusterData {
  pools?: NodePool[]
  accountingEnabled?: boolean
  imageTag?: string
  description?: string
}

// SlurmLoginInfo describes how to SSH into a cluster's login node. The command
// routes through the apiserver SSH gateway (same mechanism as workload SSH).
export interface SlurmLoginInfo {
  enabled: boolean
  ready: boolean
  sshCommand?: string
  podName?: string
  container?: string
  message?: string
}
