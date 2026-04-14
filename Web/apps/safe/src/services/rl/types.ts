export interface RlConfigResponse {
  supported: boolean
  reason?: string
  model: {
    id: string
    displayName: string
    modelName: string
    accessMode: string
    phase: string
    workspace: string
  }
  datasetFilter: {
    datasetType: string
    workspace: string
    status: string
  }
  defaults?: RlConfigDefaults
  options: RlConfigOptions
}

export interface RlConfigDefaults {
  exportModel: boolean
  image: string
  nodeCount: number
  gpuCount: number
  cpu: string
  memory: string
  sharedMemory: string
  ephemeralStorage: string
  priority: number
  trainConfig: RlTrainConfig
}

export interface RlTrainConfig {
  algorithm: string
  strategy: string
  rewardType: string
  trainBatchSize: number
  maxPromptLength: number
  maxResponseLength: number
  actorLr: number
  miniPatchSize: number
  microBatchSizePerGpu: number
  gradClip: number
  paramOffload: boolean
  optimizerOffload: boolean
  gradientCheckpointing: boolean
  useTorchCompile: boolean
  megatronTpSize: number
  megatronPpSize: number
  megatronCpSize: number
  megatronEpSize: number
  gradOffload: boolean
  useKlLoss: boolean
  klLossCoef: number
  rolloutN: number
  rolloutTpSize: number
  rolloutGpuMemory: number
  refParamOffload: boolean
  refReshardAfterForward: boolean
  totalEpochs: number
  saveFreq: number
  testFreq: number
}

export interface RlConfigOptions {
  algorithmOptions: string[]
  strategyOptions: string[]
  rewardTypeOptions: string[]
  priorityOptions: Array<{ label: string; value: number }>
}

export interface CreateRlJobRequest {
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
  sharedMemory: string
  ephemeralStorage: string
  priority: number
  timeout?: number
  trainConfig: RlTrainConfig
}

export interface CreateRlJobResponse {
  workloadId: string
}
