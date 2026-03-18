import request from '@/services/request'
import {NodeStatus, WorkloadStatus} from '@/constants'
import { useGlobalCluster } from '@/composables/useGlobalCluster'

// Helper to get cluster and append to params
const withCluster = <T extends Record<string, any>>(params?: T): T & { cluster?: string } => {
  const { selectedCluster } = useGlobalCluster()
  const cluster = selectedCluster.value
  return {
    ...params,
    ...(cluster ? { cluster } : {}),
  } as T & { cluster?: string }
}

export interface ClusterOverviewRes {
  totalNodes: number
  healthyNodes: number
  faultyNodes: number
  fullyIdleNodes: number
  partiallyIdleNodes: number
  busyNodes: number
  allocationRate: number
  utilization: number
  // new
  totalRx: number
  totalTx: number
  readBandwidth: number
  writeBandwidth: number
  usedSpace: number
  totalSpace: number
  usagePercentage: number
  usedInodes: number
  totalInodes: number
  inodesUsagePercentage: number

}

export interface GpuUtilRes {
  allocationRate: number
  utilization: number
}

export interface GpuUtilHistoryParams {
  start: number
  end: number
  step?: number
}

export interface TimePoint {
  timestamp: number
  value: number
}

export interface GpuUtilHistoryRes {
  allocationRate: TimePoint[]
  utilization: TimePoint[]
}

export interface PageRes {
  pageNum?: number
  pageSize?: number
}

export interface WorkloadListRes extends PageRes {
  name?: string
  namespace?: string
  kind?: string
  status?: string
  order_by?: string
  order?: string
}
export interface NodeListRes extends PageRes {
  status?: string[]
  name?: string
  gpuName?: string
}

export interface NodeRowItem {
  name: string
  ip: string
  gpuName: string
  gpuCount: number
  gpuAllocation: number
  gpuUtilization: number
  status: NodeStatus
  statusColor: string
}

export interface WorkloadRowItem {
  kind: string
  name: string
  namespace: string
  uid: string
  gpuAllocated: number
  gpuAllocation: any
  status: WorkloadStatus
  workloadStatus?: WorkloadStatus // Add alternative name for status
  statusColor: string
  startAt: number
  endAt: number
  // Additional utilization metrics
  instantGpuUtilization?: number
  p50GpuUtilization?: number
  p90GpuUtilization?: number
  p95GpuUtilization?: number
  // "gpu_allocation": {
  //     "AMD_Instinct_MI300X_OAM": 8
  // },
}

// dashboard page
export const getClusterOverview = (): Promise<ClusterOverviewRes> =>
  request.get('/clusters/overview', { params: withCluster() })
export const getGpuHeatmap = (): Promise<any> =>
  request.get('/clusters/gpuHeatmap', { params: withCluster() })

export const getGpuUtilization = (): Promise<GpuUtilRes> =>
  request.get('/nodes/gpuUtilization', { params: withCluster() })

export const getGpuUtilHistory = (params: GpuUtilHistoryParams): Promise<GpuUtilHistoryRes> => {
  return request.get('/nodes/gpuUtilizationHistory', { params: withCluster(params) })
}

export const getConsumers = (params: PageRes): Promise<any> => {
  return request.get('/clusters/consumers', { params: withCluster(params) })
}

// node list page
export const getNodesList = (params: NodeListRes): Promise<{ data: NodeRowItem[]; total: number }> => {
  return request.get('/nodes', { 
    params: withCluster(params),
    timeout: 60000  // longer timeout for list APIs (60 seconds)
  })
}

// node detail page
export const getNodeByName = (name: string): Promise<any> => {
  return request.get(`/nodes/${name}`, { params: withCluster() })
}

export const getNodeGpuDevice = (name: string): Promise<any> => {
  return request.get(`/nodes/${name}/gpuDevices`, { params: withCluster() })
}

export const getNodeGpuMetrics = (name: string, params: GpuUtilHistoryParams): Promise<any> => {
  return request.get(`/nodes/${name}/gpuMetrics`, { params: withCluster(params) })
}

export const getWorkloads = (params: PageRes, name: string): Promise<any> => {
  return request.get(`/nodes/${name}/workloads`, { params: withCluster(params) })
}

export const getWorkloadsHistory = (params: PageRes, name: string): Promise<any> => {
  return request.get(`/nodes/${name}/workloadsHistory`, { params: withCluster(params) })
}

// workloads list page
export const getWorkloadsList = (params: WorkloadListRes): Promise<{ data: WorkloadRowItem[]; total: number }> => {
  return request.get('/workloads', { 
    params: withCluster(params),
    timeout: 60000  // longer timeout for list APIs (60 seconds)
  })
}

export const getWorkloadMeta = (): Promise<{namespaces: string[], kinds: string[]}> =>
  request.get('/workloadMetadata', { params: withCluster() })

// Workload statistics
export const getWorkloadStats = (uid: string, params: {
  start?: number
  end?: number
  step?: number
  metrics?: string[]
}): Promise<any> => {
  return request.get(`/workloads/${uid}/stats`, { params: withCluster(params) })
}

export interface WorkloadDetail {
  // Basic info
  uid: string
  name: string
  namespace: string
  kind: string
  
  // Status info
  status: string
  statusReason?: string
  
  // Timestamps
  createdAt?: number
  startAt?: number
  endAt?: number
  
  // GPU allocation
  gpuAllocated: number
  gpuType?: string
  gpuAllocation?: Record<string, number>
  
  // Resource usage statistics
  instantGpuUtilization?: number
  avgGpuUtilization?: number
  p50GpuUtilization?: number
  p90GpuUtilization?: number
  p95GpuUtilization?: number
  maxGpuUtilization?: number
  
  // Memory statistics
  avgGpuMemoryUtilization?: number
  p50GpuMemoryUtilization?: number
  p90GpuMemoryUtilization?: number
  p95GpuMemoryUtilization?: number
  maxGpuMemoryUtilization?: number
  
  // Other monitoring data
  nodeNames?: string[]
  labels?: Record<string, string>
  
  // ... possible additional fields
}

// Get workload details
export const getWorkloadDetail = (uid: string): Promise<WorkloadDetail> =>
  request.get(`/workloads/${uid}`, { params: withCluster() })

// Alias for backward compatibility
export const getWorkloadsDetail = getWorkloadDetail

// Get workload hierarchy (supports query by kind and name)
export const getWorkloadsTree = (params: { kind: string; name: string } | string): Promise<any> => {
  if (typeof params === 'string') {
    // If a uid string is passed
    return request.get(`/workloads/${params}/hierarchy`, { params: withCluster() })
  } else {
    // If a kind and name object is passed
    return request.get('/workloads/hierarchy', { params: withCluster(params) })
  }
}

// Get workload GPU metrics
export const getWorkloadGpuMetrics = (uid: string, params: {
  start: number
  end: number
  step?: number
}): Promise<any> =>
  request.get(`/workloads/${uid}/gpuMetrics`, { params: withCluster(params) })

// Get workload metrics (backward compatible alias, must be defined after the original function)
export const getWorkloadsMetrics = getWorkloadGpuMetrics

// Get workload events
export const getWorkloadEvents = (uid: string, params?: {
  start?: number
  end?: number
}): Promise<any> =>
  request.get(`/workloads/${uid}/events`, { params: withCluster(params) })

// Get workload TraceLens analysis
export const getWorkloadTracelens = (uid: string): Promise<any> =>
  request.get(`/workloads/${uid}/tracelens`, { params: withCluster() })

// Get workload profiler files
export const getWorkloadProfilerFiles = (uid: string): Promise<any> =>
  request.get(`/workloads/${uid}/profiler-files`, { params: withCluster() })

// Get profiler file list (alias for backward compatibility)
export const getProfilerFiles = getWorkloadProfilerFiles

// Download profiling file
export const downloadProfilerFile = (fileId: number, fileName: string, cluster?: string): void => {
  const params = cluster ? { cluster } : withCluster()
  const url = `/profiler-files/${fileId}/download`
  
  // Create a hidden anchor tag to trigger download
  const link = document.createElement('a')
  const baseUrl = `${import.meta.env.BASE_URL}v1`
  const queryString = new URLSearchParams(params).toString()
  link.href = `${baseUrl}${url}?${queryString}`
  link.download = fileName || `profiler-file-${fileId}.tar.gz`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

// API: Get node workload history
export const getNodeWorkloadsHistory = (nodeName: string, params: {
  pageNum?: number
  pageSize?: number
  start?: number
  end?: number
}): Promise<any> =>
  request.get(`/nodes/${nodeName}/workloadsHistory`, { params: withCluster(params) })

// API: Batch get workload statistics
export const getWorkloadsBatchStats = (uids: string[], metrics?: string[]): Promise<any> =>
  request.post('/workloads/batchStats', {
    uids,
    metrics,
    cluster: withCluster().cluster
  })

// Workload Statistics interface
export interface WorkloadStatistics {
  totalWorkloads: number
  runningWorkloadsCount: number
  totalGPUs: number
  avgGpuAllocated: number
  avgGpuUtilization: number
  avgUtilization: number
  p50Utilization: number
  p90Utilization: number
  p95Utilization: number
  maxUtilization: number
  totalNamespaces: number
  avgAllocationRate: number
  peakAllocationRate: number
  lowUtilizationWorkloadsCount: number
}

// Get workload statistics
export const getWorkloadStatistics = (): Promise<WorkloadStatistics> =>
  request.get('/workloads/statistics', { params: withCluster() })

// Get workload GPU utilization history
export const getWorkloadGpuUtilizationHistory = (params: {
  kind: string
  name: string
  start: number
  end: number
  step?: number
}): Promise<any> =>
  request.get('/workloads/gpuUtilizationHistory', { params: withCluster(params) })