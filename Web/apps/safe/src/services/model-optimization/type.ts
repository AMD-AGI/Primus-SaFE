// ── Task status & phase enums ──

export enum OptimizationStatus {
  Pending = 'Pending',
  Running = 'Running',
  Succeeded = 'Succeeded',
  Failed = 'Failed',
  Interrupted = 'Interrupted',
}

export const OptimizationStatusTagType: Record<string, 'success' | 'danger' | 'info' | 'primary' | 'warning'> = {
  Succeeded: 'success',
  Failed: 'danger',
  Pending: 'info',
  Running: 'primary',
  Interrupted: 'warning',
}

// ── Request / response types ──

export interface OptimizationTaskParams {
  workspace?: string
  status?: string
  modelId?: string
  search?: string
  offset?: number
  limit?: number
  sortBy?: string
  order?: 'asc' | 'desc'
}

export interface CreateOptimizationTaskPayload {
  modelId: string
  workspace: string
  displayName?: string
  mode?: 'local' | 'claw'
  framework?: string
  precision?: string
  tp?: number
  ep?: number
  gpuType?: string
  isl?: number
  osl?: number
  concurrency?: number
  kernelBackends?: string[]
  geakStepLimit?: number
  image?: string
  inferencexPath?: string
  resultsPath?: string
  rayReplica?: number
  rayGpu?: number
  rayCpu?: number
  rayMemory?: number
  targetGpu?: string
  baselineCSV?: string
  baselineCount?: number
}

export interface OptimizationTask {
  id: string
  clawSessionId?: string
  displayName: string
  modelId: string
  workspace: string
  status: OptimizationStatus
  currentPhase?: number
  currentPhaseName?: string
  message?: string
  createdAt: string
  updatedAt: string
  [key: string]: unknown
}

export interface OptimizationTaskListResponse {
  items: OptimizationTask[]
  totalCount: number
}

// ── SSE event types ──

export interface OptimizationEvent {
  id: string
  taskId: string
  type: 'phase' | 'benchmark' | 'kernel' | 'log' | 'status' | 'done'
  timestamp: number
  payload: PhasePayload | BenchmarkPayload | KernelPayload | LogPayload | StatusPayload | DonePayload
}

export interface PhasePayload {
  phase: number
  phaseName: string
  status: string
  message: string
}

export interface BenchmarkPayload {
  round?: number
  label?: string
  inputTokensPerSec?: number
  outputTokensPerSec?: number
  totalTokensPerSec?: number
  tpotMs?: number
  ttftMs?: number
  concurrency?: number
  isl?: number
  osl?: number
  framework?: string
}

export interface KernelPayload {
  name: string
  backend: string
  status: string
  source?: string
  gpuPercent?: number
  baselineUs?: number
  optimizedUs?: number
}

export interface LogPayload {
  level: string
  source: string
  message: string
}

export interface StatusPayload {
  status: string
  message: string
}

export interface DonePayload {
  status: string
  message: string
}

export interface ArtifactItem {
  path: string
  size?: number
  modTime?: string
}

export interface ApplyPayload {
  displayName?: string
  workspace?: string
  image?: string
  cpu?: number
  memory?: string
  gpu?: number
  replica?: number
  port?: number
}
