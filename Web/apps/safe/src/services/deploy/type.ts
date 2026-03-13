export type DeploymentType = 'safe' | 'lens'

export interface DeploymentRequest {
  id: number
  deploy_type?: DeploymentType
  deploy_name: string
  description: string
  status: DeploymentStatus
  rollback_from_id?: number
  created_at: string
  updated_at: string
  approved_at?: string
  approver_name?: string
  approval_result?: string
  rejection_reason?: string
  // safe
  image_versions?: Record<string, string>
  env_file_config?: string
  // lens
  branch?: string
  control_plane_config?: string
  data_plane_config?: string
  control_plane_diff?: string
  data_plane_diff?: string
  snapshot_id?: number
  workload_id?: string
}

export type DeploymentStatus =
  | 'pending_approval'
  | 'approved'
  | 'rejected'
  | 'deploying'
  | 'deployed'
  | 'failed'

export interface CreateDeploymentRequest {
  type: DeploymentType
  description?: string
  image_versions?: Record<string, string>
  env_file_config?: string
  branch?: string
  control_plane_config?: string
  data_plane_config?: string
  rollback_from_id?: number
}

export interface ApprovalRequest {
  approved: boolean
  reason?: string
}

export interface DeploymentListParams {
  offset?: number
  limit?: number
  status?: DeploymentStatus
  type?: DeploymentType
  sortBy?: string
  order?: 'asc' | 'desc'
}

export interface DeploymentListResponse {
  items: DeploymentRequest[]
  total_count: number
}

export interface Component {
  name: string
}

export type SafeEnvConfigResponse = {
  type?: 'safe'
  env_file_config: string
  image_versions?: Record<string, string>
}

export type LensEnvConfigResponse = {
  type: 'lens'
  branch: string
  control_plane_config: string
  data_plane_config: string
  snapshot_id?: number
  created_at?: string
}

export type EnvConfigResponse = SafeEnvConfigResponse | LensEnvConfigResponse

export type SafeComponentsResponse = {
  type?: 'safe'
  components: string[]
}

export type LensComponentsResponse = {
  type: 'lens'
  message: string
}

export type ComponentsResponse = SafeComponentsResponse | LensComponentsResponse
