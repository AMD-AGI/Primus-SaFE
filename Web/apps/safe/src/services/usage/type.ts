export interface UsageQuery {
  /** Resource name, defaults to amd.com/gpu on the server */
  resource?: string
  /** RFC3339, defaults to end - 24h */
  start?: string
  /** RFC3339, defaults to now */
  end?: string
  /** Go duration such as 15m, 1h, 4h; never finer than the data source */
  step?: string
  sortBy?: UsageSortKey
  /** Defaults to desc; without sortBy the order is by name */
  order?: 'asc' | 'desc'
  /** Workload list length, 1..100, default 10 */
  limit?: number
}

export type UsageSortKey = 'occupancy' | 'utilization' | 'effectiveUtilization' | 'allocatedHours'

/** Ratios are 0..1 and null when the denominator is 0 or there is no GPU data. */
export interface UsagePoint {
  time: string
  quota: number
  allocated: number
  busy: number | null
  /** Share of the allocated time that has GPU metrics */
  coverage: number | null
  queued: number
  queuedQuota: number | null
  queuedUserQos: number | null
  queuedExternal: number | null
  queuedOther: number | null
  /** allocated / quota */
  occupancy: number | null
  /** busy / allocated (with data) */
  utilization: number | null
  /** busy / quota */
  effectiveUtilization: number | null
}

export interface UsageSummary {
  occupancy: number | null
  utilization: number | null
  effectiveUtilization: number | null
  allocatedHours: number
  busyHours: number | null
  queuedAvg: number
  queuedMax: number
}

export interface UsageRange {
  resource: string
  start: string
  end: string
  /** Go duration string, e.g. "15m0s", "1h0m0s" */
  step: string
  source: 'sample' | 'hourly' | 'mixed'
}

export interface UsageClusterItem {
  cluster: string
  workspaces: number
  /** Latest sample, null when there is none recently */
  current: UsagePoint | null
  summary: UsageSummary
  series: UsagePoint[]
}

export interface ListUsageClustersResp extends UsageRange {
  items: UsageClusterItem[]
}

export interface UsageWorkspaceItem {
  workspace: string
  current: UsagePoint | null
  summary: UsageSummary
}

export interface ListUsageWorkspacesResp extends UsageRange {
  cluster: string
  /** Over the listed workspaces only */
  total: UsageSummary
  items: UsageWorkspaceItem[]
}

export interface UsageWorkloadItem {
  workloadId: string
  displayName: string
  userName: string
  phase: string
  allocated: number
  /** Percent 0..100 (not a ratio), null when unknown */
  gpuUtilization: number | null
}

export interface GetUsageWorkspaceResp extends UsageRange {
  workspace: string
  cluster: string
  current: UsagePoint | null
  summary: UsageSummary
  series: UsagePoint[]
  topAllocated: UsageWorkloadItem[]
  /** Empty for a non-GPU resource */
  lowestUtilization: UsageWorkloadItem[]
}
