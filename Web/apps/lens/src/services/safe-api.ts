import axios from 'axios'
import type { AxiosRequestConfig } from 'axios'

// Create axios instance dedicated to Safe API
const safeRequest = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
  headers: {
    'userId': '21232f297a57a5a743894a0e4a801fc3' // Temp backdoor auth
  }
})

// Response interceptor
safeRequest.interceptors.response.use(
  (response) => {
    // Safe API may return data directly, unlike lens API which has meta wrapper
    return response.data
  },
  (error) => {
    return Promise.reject(error.message || 'Network Error')
  }
)

// Workspace list item interface
export type ScopesKeys = string // define based on actual usage

export interface WorkspaceItem {
  workspaceName: string
  workspaceId: string
  flavorId: string
  currentNodeCount: number
  targetNodeCount: number
  abnormalNodeCount: number
  isDefault?: boolean
  managers?: string[]
  scopes: ScopesKeys[]
  clusterId?: string // cluster this workspace belongs to
}

// Workspaces list response interface
export interface WorkspacesResponse {
  totalCount: number
  items: WorkspaceItem[]
}

// Workspace detail interface
export interface WorkspaceDetail {
  workspaceId: string
  workspaceName: string
  clusterId: string
  flavorId: string
  userId: string
  targetNodeCount: number
  currentNodeCount: number
  abnormalNodeCount: number
  phase: string
  creationTime: string
  description: string
  queuePolicy: string
  scopes: string[]
  volumes: any[]
  enablePreempt: boolean
  managers: any[]
  isDefault: boolean
  totalQuota: {
    'amd.com/gpu': number
    cpu: number
    'ephemeral-storage': number
    memory: number
    'rdma/hca': number
  }
  availQuota: {
    'amd.com/gpu': number
    cpu: number
    'ephemeral-storage': number
    memory: number
    'rdma/hca': number
  }
  abnormalQuota: {
    'amd.com/gpu': number
    cpu: number
    'ephemeral-storage': number
    memory: number
    'rdma/hca': number
  }
  usedQuota: {
    'amd.com/gpu': number
    cpu: number
    'ephemeral-storage': number
    memory: number
    'rdma/hca': number
  }
  usedNodeCount: number
  imageSecretIds: string[]
}

// Get workspaces list
export const getWorkspaces = async (clusterId?: string): Promise<WorkspacesResponse> => {
  const params: AxiosRequestConfig['params'] = {}
  if (clusterId) {
    params.clusterId = clusterId
  }
  const response: any = await safeRequest.get('/workspaces', { params })
  // Ensure correct return data structure
  return {
    totalCount: response?.totalCount || 0,
    items: response?.items || []
  }
}

export const getWorkspaceDetail = (id: string): Promise<WorkspaceDetail> => {
  return safeRequest.get(`/workspaces/${id}`)
}

// Export safeRequest instance for use by other modules
export default safeRequest