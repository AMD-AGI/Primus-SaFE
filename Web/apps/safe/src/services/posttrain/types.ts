export interface PostTrainRunItem {
  runId: string
  workloadId: string
  workloadUid?: string
  displayName: string
  trainType: 'sft' | 'rl'
  strategy: 'full' | 'lora' | 'fsdp2' | 'megatron'
  algorithm?: string
  workspace: string
  cluster: string
  userId?: string
  userName?: string
  baseModelId: string
  baseModelName: string
  datasetId: string
  datasetName?: string
  image?: string
  nodeCount?: number
  gpuPerNode?: number
  cpu?: string
  memory?: string
  sharedMemory?: string
  ephemeralStorage?: string
  priority?: number
  timeout?: number
  exportModel: boolean
  outputPath?: string
  status: string
  message?: string
  createdAt?: string
  startTime?: string
  endTime?: string
  duration?: string
  modelId?: string
  modelDisplayName?: string
  modelPhase?: string
  modelOrigin?: string
  parameterSummary?: string
  parameterSnapshot?: Record<string, unknown>
  availableMetrics?: string[]
  latestLoss?: number | null
  lossMetricName?: string
  lossDataSource?: string
}

export interface PostTrainListParams {
  trainType?: string
  strategy?: string
  status?: string
  workspace?: string
  baseModel?: string
  dataset?: string
  owner?: string
  since?: string
  until?: string
  search?: string
  offset?: number
  limit?: number
}

export interface PostTrainListResp {
  total: number
  items: PostTrainRunItem[]
}

export interface MetricPoint {
  step: number
  value: number
  timestamp?: string
}

export interface PostTrainMetricsResp {
  runId: string
  latestLoss?: number | null
  source?: string
  availableMetrics?: string[]
  series?: Record<string, MetricPoint[]>
}
