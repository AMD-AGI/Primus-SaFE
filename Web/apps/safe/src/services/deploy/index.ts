import request from '@/services/request'
import type {
  DeploymentRequest,
  DeploymentListParams,
  DeploymentListResponse,
  CreateDeploymentRequest,
  ApprovalRequest,
  DeploymentType,
  EnvConfigResponse,
  ComponentsResponse,
} from './type'

/**
 * Get deployment requests list
 */
export const getDeployments = (params?: DeploymentListParams): Promise<DeploymentListResponse> =>
  request.get('/cd/deployments', { params })

/**
 * Create a new deployment request
 */
export const createDeployment = (data: CreateDeploymentRequest): Promise<DeploymentRequest> =>
  request.post('/cd/deployments', data)

/**
 * Get current environment configuration
 */
export const getEnvConfig = (type: DeploymentType = 'safe'): Promise<EnvConfigResponse> =>
  request.get('/cd/env-config', { params: { type } })

/**
 * Get available components list
 */
export const getComponents = (type: DeploymentType = 'safe'): Promise<ComponentsResponse> =>
  request.get('/cd/components', { params: { type } })

/**
 * Approve or reject a deployment request
 */
export const approveDeployment = (id: string, data: ApprovalRequest): Promise<DeploymentRequest> =>
  request.post(`/cd/deployments/${id}/approve`, data)

/**
 * Rollback to a specific deployment
 */
export const rollbackDeployment = (id: string): Promise<DeploymentRequest> =>
  request.post(`/cd/deployments/${id}/rollback`)

/**
 * Retry a failed deployment
 */
export const retryDeployment = (id: string): Promise<DeploymentRequest> =>
  request.post(`/cd/deployments/${id}/retry`)

/**
 * Get deployment request detail
 */
export const getDeploymentDetail = (id: string | number): Promise<DeploymentRequest> =>
  request.get(`/cd/deployments/${id}`)
