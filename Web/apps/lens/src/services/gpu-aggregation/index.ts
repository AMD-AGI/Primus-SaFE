import request from '@/services/request'

// Type definitions
export interface ClusterGpuHourlyStats {
  id: number
  clusterName: string
  statHour: string
  totalGpuCapacity: number
  allocatedGpuCount: number
  allocationRate: number
  avgUtilization: number
  maxUtilization: number
  minUtilization: number
  p50Utilization: number
  p95Utilization: number
  sampleCount: number
  createdAt: string
  updatedAt: string
}

export interface NamespaceGpuHourlyStats {
  id: number
  clusterName: string
  namespace: string
  statHour: string
  totalGpuCapacity: number
  allocatedGpuCount: number
  avgUtilization: number
  maxUtilization: number
  minUtilization: number
  activeWorkloadCount: number
  createdAt: string
  updatedAt: string
}

export interface LabelGpuHourlyStats {
  id: number
  clusterName: string
  dimensionType: 'label' | 'annotation'
  dimensionKey: string
  dimensionValue: string
  statHour: string
  allocatedGpuCount: number
  avgUtilization: number
  maxUtilization: number
  minUtilization: number
  activeWorkloadCount: number
  createdAt: string
  updatedAt: string
}

export interface WorkloadGpuHourlyStats {
  id: number
  clusterName: string
  namespace: string
  workloadName: string
  workloadType: string
  statHour: string
  allocatedGpuCount: number
  requestedGpuCount: number
  avgUtilization: number
  maxUtilization: number
  minUtilization: number
  p50Utilization: number
  p95Utilization: number
  avgGpuMemoryUsed: number
  maxGpuMemoryUsed: number
  avgGpuMemoryTotal: number
  avgReplicaCount: number
  maxReplicaCount: number
  minReplicaCount: number
  workloadStatus: string
  sampleCount: number
  ownerUid: string
  ownerName: string
  labels: Record<string, any>
  annotations: Record<string, any>
  createdAt: string
  updatedAt: string
}

export interface GpuAllocationSnapshot {
  id: number
  clusterName: string
  snapshotTime: string
  dimensionType: string
  dimensionKey: string
  dimensionValue: string
  totalGpuCapacity: number
  allocatedGpuCount: number
  allocationDetails: Record<string, any>
  createdAt: string
}

// Paginated response
export interface PaginatedResponse<T> {
  total: number
  page: number
  page_size: number
  total_pages: number
  data: T[]
}

// Get cluster hourly statistics
export function getClusterHourlyStats(params: {
  cluster?: string
  startTime: string
  endTime: string
  page?: number
  page_size?: number
  order_by?: 'time' | 'utilization'
  order_direction?: 'asc' | 'desc'
}) {
  return request.get<any, PaginatedResponse<ClusterGpuHourlyStats>>('/gpu-aggregation/cluster/hourly-stats', {
    params: {
      cluster: params.cluster,
      start_time: params.startTime,
      end_time: params.endTime,
      page: params.page,
      page_size: params.page_size,
      order_by: params.order_by,
      order_direction: params.order_direction
    },
    timeout: 60000  // statistics API may need longer timeout
  })
}

// Get namespace hourly statistics
export function getNamespaceHourlyStats(params: {
  cluster?: string
  namespace?: string
  startTime: string
  endTime: string
  page?: number
  page_size?: number
  order_by?: 'time' | 'utilization'
  order_direction?: 'asc' | 'desc'
}) {
  return request.get<any, PaginatedResponse<NamespaceGpuHourlyStats>>('/gpu-aggregation/namespaces/hourly-stats', {
    params: {
      cluster: params.cluster,
      namespace: params.namespace,
      start_time: params.startTime,
      end_time: params.endTime,
      page: params.page,
      page_size: params.page_size,
      order_by: params.order_by,
      order_direction: params.order_direction
    }
  })
}

// Get label/annotation hourly statistics
export function getLabelHourlyStats(params: {
  cluster?: string
  dimensionType: 'label' | 'annotation'
  dimensionKey: string
  dimensionValue?: string
  startTime: string
  endTime: string
  page?: number
  page_size?: number
  order_by?: 'time' | 'utilization'
  order_direction?: 'asc' | 'desc'
}) {
  return request.get<any, PaginatedResponse<LabelGpuHourlyStats>>('/gpu-aggregation/labels/hourly-stats', {
    params: {
      cluster: params.cluster,
      dimension_type: params.dimensionType,
      dimension_key: params.dimensionKey,
      dimension_value: params.dimensionValue,
      start_time: params.startTime,
      end_time: params.endTime,
      page: params.page,
      page_size: params.page_size,
      order_by: params.order_by,
      order_direction: params.order_direction
    }
  })
}

// Get latest snapshot
export function getLatestSnapshot(params?: {
  cluster?: string
}) {
  return request.get<any, GpuAllocationSnapshot>('/gpu-aggregation/snapshots/latest', {
    params
  })
}

// Get snapshot list
export function getSnapshots(params?: {
  cluster?: string
  startTime?: string
  endTime?: string
}) {
  return request.get<any, GpuAllocationSnapshot[]>('/gpu-aggregation/snapshots', {
    params
  })
}

// Get clusters
export function getClusters() {
  return request.get<any, string[]>('/gpu-aggregation/clusters')
}

// Get namespaces
export function getNamespaces(params: {
  cluster?: string
  startTime: string
  endTime: string
}) {
  return request.get<any, string[]>('/gpu-aggregation/namespaces', {
    params
  })
}

// Get dimension keys
export function getDimensionKeys(params: {
  cluster?: string
  dimensionType: 'label' | 'annotation'
  startTime: string
  endTime: string
}) {
  return request.get<any, string[]>('/gpu-aggregation/dimension-keys', {
    params: {
      cluster: params.cluster,
      dimension_type: params.dimensionType,
      start_time: params.startTime,
      end_time: params.endTime
    }
  })
}

// Get workload hourly statistics
export function getWorkloadHourlyStats(params: {
  cluster?: string
  namespace?: string
  workloadName?: string
  workloadType?: string
  startTime: string
  endTime: string
  page?: number
  page_size?: number
  order_by?: 'time' | 'utilization'
  order_direction?: 'asc' | 'desc'
}) {
  return request.get<any, PaginatedResponse<WorkloadGpuHourlyStats>>('/gpu-aggregation/workloads/hourly-stats', {
    timeout: 60000,  // statistics API may need longer timeout
    params: {
      cluster: params.cluster,
      namespace: params.namespace,
      workload_name: params.workloadName,
      workload_type: params.workloadType,
      start_time: params.startTime,
      end_time: params.endTime,
      page: params.page,
      page_size: params.page_size,
      order_by: params.order_by,
      order_direction: params.order_direction
    }
  })
}
