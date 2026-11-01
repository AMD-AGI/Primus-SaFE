import request from '@/services/request'

// Model related APIs
export interface PlaygroundModel {
  id: string
  displayName: string
  description?: string
  accessMode?: 'remote_api' | 'local' | 'local_path' | 's3_sync' | 'cloud'
  phase: 'Ready' | 'Pending' | 'Failed' | 'Running' | 'Stopped' | 'Downloading' | 'Uploading'
  message?: string
  icon?: string
  tags?: string
  categorizedTags?: Array<{
    value: string
    color: string
  }>
  maxTokens?: number

  // Resource configuration
  cpu?: string
  gpu?: string
  memory?: string

  // Inference related
  serviceID?: string
  inferencePhase?: string
  workloadID?: string

  // Source related
  sourceURL?: string
  sourceToken?: string
  downloadType?: string
  localPath?: string
  s3Config?: string

  // Target volume info (returned by API for s3_sync / local models)
  targetVolume?: string
  targetSubpath?: string

  // Time related
  createdAt?: string
  updatedAt?: string
  deletionTime?: {
    Time: string
    Valid: boolean
  }

  // Others
  label?: string
  version?: string
  isDeleted?: boolean

  // Fine-tuning metadata
  origin?: 'external' | 'fine_tuned' | 'rl_trained'
  sftJobId?: string
  baseModel?: string
  userId?: string
  userName?: string
  workspace?: string
}

// ── Create model payload (covers all 4 access modes) ──

export interface CreateModelS3Source {
  uri: string
  accessKeyId?: string
  secretAccessKey?: string
  region?: string
  endpoint?: string
}

export interface CreateModelTarget {
  volume?: string
  subpath?: string
}

export interface CreateModelPayload {
  displayName?: string
  description?: string
  icon?: string
  label?: string
  tags?: string[]
  maxTokens?: number
  origin?: string
  workspace?: string
  source: {
    accessMode: 'local' | 'remote_api' | 'local_path' | 's3_sync'
    url?: string
    token?: string
    localPath?: string
    modelName?: string
    apiKey?: string
  }
  s3Source?: CreateModelS3Source
  target?: CreateModelTarget
}

// ── Patch model payload ──

export interface PatchModelPayload {
  displayName?: string
  description?: string
  modelName?: string
  icon?: string
  label?: string
  tags?: string[]
  maxTokens?: number
}

/**
 * Whether the model is a deployable local model (supports both imported and SFT-produced).
 */
export function isDeployableLocalModel(model: PlaygroundModel): boolean {
  return model.accessMode === 'local' || model.accessMode === 'local_path' || model.accessMode === 's3_sync'
}

/**
 * Whether the model supports SFT (only HuggingFace-imported base models).
 */
export function canSft(model: PlaygroundModel): boolean {
  return model.accessMode === 'local' && model.phase === 'Ready'
}

/**
 * Whether the model supports training (SFT + RL).
 */
export function canTrain(model: PlaygroundModel): boolean {
  return (model.accessMode === 'local' || model.accessMode === 'local_path') && model.phase === 'Ready'
}

export interface ModelsListParams {
  inferenceStatus?: string
  accessMode?: string
  origin?: string
  workspace?: string
  search?: string
  userId?: string
  page?: number
  pageSize?: number
}

export interface ModelsListResp {
  total: number
  items: PlaygroundModel[]
}

export function getModelsList(params?: ModelsListParams) {
  return request.get<ModelsListResp>(`/playground/models`, { params })
}

export function getModelDetail(id: string) {
  return request.get<PlaygroundModel>(`/playground/models/${id}`)
}

export function createModel(data: Partial<PlaygroundModel> | CreateModelPayload) {
  return request.post<PlaygroundModel>(`/playground/models`, data)
}

export function patchModel(id: string, data: PatchModelPayload) {
  return request.patch<PlaygroundModel>(`/playground/models/${id}`, data)
}

export function deleteModel(id: string) {
  return request.delete<void>(`/playground/models/${id}`)
}

export interface ToggleModelParams {
  enabled: boolean
  resource?: {
    workspace: string
    replica: number
    cpu: number | string
    memory: number | string
    gpu: string
  }
  config?:
    | {
        image: string
        entryPoint: string
      }
    | {
        apiKey: string
        model: string
      }
}

export function toggleModel(id: string, params: ToggleModelParams) {
  return request.post<void>(`/playground/models/${id}/toggle`, params)
}

export function retryModel(id: string) {
  return request.post<void>(`/playground/models/${id}/retry`, {}, { timeout: 60000 })
}

export interface WorkloadConfig {
  displayName: string
  description: string
  labels?: Record<string, string>
  env?: Record<string, string>
  modelId: string
  modelName: string
  modelPath: string
  accessMode: string
  maxTokens: number
  image: string
  entryPoint: string
  workspace: string
  cpu: string
  memory: string
  gpu: string
}

export function getModelWorkloadConfig(id: string, workspaceId: string) {
  return request.get<WorkloadConfig>(`/playground/models/${id}/workload-config`, {
    params: { workspace: workspaceId },
  })
}

export interface ModelWorkload {
  workloadId: string
  displayName: string
  workspace: string
  phase?: string
  createdat?: string
  [key: string]: unknown
}

export interface ModelWorkloadsResp {
  total: number
  items: ModelWorkload[]
}

export function getModelWorkloads(id: string) {
  return request.get<ModelWorkloadsResp>(`/playground/models/${id}/workloads`)
}
