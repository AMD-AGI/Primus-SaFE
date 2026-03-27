export interface SftConfigResponse {
  supported: boolean
  reason: string
  model: {
    id: string
    displayName: string
    modelName: string
    accessMode: string
    phase: string
    workspace: string
    maxTokens: number
  }
  datasetFilter: {
    datasetType: string
    workspace: string
    status: string
  }
  defaults: SftDefaults
  options: SftOptions
}

export interface SftDefaults {
  exportModel: boolean
  image: string
  nodeCount: number
  gpuCount: number
  cpu: string
  memory: string
  ephemeralStorage: string
  priority: number
  trainConfig: SftTrainConfig
}

export interface SftTrainConfig {
  peft: string
  datasetFormat: string
  trainIters: number
  globalBatchSize: number
  microBatchSize: number
  seqLength: number
  finetuneLr: number
  minLr: number
  lrWarmupIters: number
  evalInterval: number
  saveInterval: number
  precisionConfig: string
  tensorModelParallelSize: number
  pipelineModelParallelSize: number
  contextParallelSize: number
  sequenceParallel: boolean
  peftDim: number
  peftAlpha: number
  packedSequence: boolean
}

export interface SftOptions {
  peftOptions: string[]
  datasetFormatOptions: string[]
  priorityOptions: Array<{ label: string; value: number }>
}

export interface CreateSftJobRequest {
  displayName: string
  modelId: string
  datasetId: string
  workspace: string
  exportModel: boolean
  image: string
  nodeCount: number
  gpuCount: number
  cpu: string
  memory: string
  ephemeralStorage: string
  priority: number
  trainConfig: SftTrainConfig
  timeout?: number
  env?: Record<string, string>
  hostpath?: string
  forceHostNetwork?: boolean
}

export interface CreateSftJobResponse {
  workloadId: string
  message?: string
}
