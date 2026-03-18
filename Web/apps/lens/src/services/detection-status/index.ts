import request from '@/services/request'

// Detection Status Types
export type DetectionStatusValue = 'unknown' | 'suspected' | 'confirmed' | 'verified' | 'conflict'
export type DetectionState = 'pending' | 'in_progress' | 'completed' | 'failed'
export type CoverageSource = 'process' | 'log' | 'image' | 'label' | 'wandb' | 'import'
export type CoverageStatus = 'pending' | 'collecting' | 'collected' | 'failed' | 'not_applicable'

export interface DetectionCoverage {
  source: CoverageSource
  status: CoverageStatus
  attemptCount: number
  lastAttemptAt?: string
  lastSuccessAt?: string
  lastError?: string
  evidenceCount: number
  coveredFrom?: string
  coveredTo?: string
  logAvailableFrom?: string
  logAvailableTo?: string
  hasGap?: boolean
}

export interface DetectionTask {
  taskType: string
  status: string
  lockOwner?: string
  createdAt: string
  updatedAt: string
  attemptCount?: number
  nextAttemptAt?: string
  coordinatorState?: string
  ext?: Record<string, any>
}

export interface DetectionEvidence {
  id: number
  workloadUid: string
  source: string
  sourceType: 'passive' | 'active'
  framework: string
  workloadType: 'training' | 'inference'
  confidence: number
  frameworkLayer: 'wrapper' | 'base'
  wrapperFramework?: string
  baseFramework?: string
  evidence: Record<string, any>
  detectedAt: string
  createdAt: string
}

export interface DetectionStatus {
  workloadUid: string
  status: DetectionStatusValue
  detectionState: DetectionState
  framework: string
  frameworks: string[]
  workloadType: 'training' | 'inference'
  confidence: number
  frameworkLayer: 'wrapper' | 'base'
  wrapperFramework?: string
  baseFramework?: string
  evidenceCount: number
  evidenceSources: string[]
  attemptCount: number
  maxAttempts: number
  lastAttemptAt?: string
  nextAttemptAt?: string
  confirmedAt?: string
  createdAt: string
  updatedAt: string
  coverage: DetectionCoverage[]
  tasks: DetectionTask[]
  hasConflicts: boolean
  conflicts?: any[]
}

export interface DetectionStatusListParams {
  cluster?: string
  status?: DetectionStatusValue
  state?: DetectionState
  page?: number
  pageSize?: number
}

export interface DetectionStatusListRes {
  data: DetectionStatus[]
  total: number
  page: number
  pageSize: number
}

export interface DetectionSummary {
  totalWorkloads: number
  statusCounts: Record<DetectionStatusValue, number>
  detectionStateCounts: Record<DetectionState, number>
  recentDetections: Array<{
    workloadUid: string
    status: DetectionStatusValue
    detectionState: DetectionState
    framework: string
    confidence: number
    updatedAt: string
  }>
}

export interface LogGapInfo {
  workloadUid: string
  hasGap: boolean
  gapFrom?: string
  gapTo?: string
  gapDurationSeconds?: number
}

// Get detection summary
export function getDetectionSummary(cluster?: string) {
  return request.get<any, DetectionSummary>('/detection-status/summary', {
    params: { cluster }
  })
}

// List detection statuses
export function getDetectionStatusList(params?: DetectionStatusListParams) {
  return request.get<any, DetectionStatusListRes>('/detection-status', {
    params
  })
}

// Get detection status by workload UID
export function getDetectionStatus(workloadUid: string, cluster?: string) {
  return request.get<any, DetectionStatus>(`/detection-status/${encodeURIComponent(workloadUid)}`, {
    params: { cluster }
  })
}

// Get detection coverage
export function getDetectionCoverage(workloadUid: string, cluster?: string) {
  return request.get<any, { workloadUid: string; coverage: DetectionCoverage[]; total: number }>(
    `/detection-status/${encodeURIComponent(workloadUid)}/coverage`,
    { params: { cluster } }
  )
}

// Initialize detection coverage
export function initializeDetectionCoverage(workloadUid: string, cluster?: string) {
  return request.post<any, { message: string; coverage?: DetectionCoverage[]; count: number }>(
    `/detection-status/${encodeURIComponent(workloadUid)}/coverage/initialize`,
    null,
    { params: { cluster } }
  )
}

// Get uncovered log window
export function getLogGap(workloadUid: string, cluster?: string) {
  return request.get<any, LogGapInfo>(
    `/detection-status/${encodeURIComponent(workloadUid)}/coverage/log-gap`,
    { params: { cluster } }
  )
}

// Get detection tasks
export function getDetectionTasks(workloadUid: string, cluster?: string) {
  return request.get<any, { workloadUid: string; tasks: DetectionTask[]; total: number }>(
    `/detection-status/${encodeURIComponent(workloadUid)}/tasks`,
    { params: { cluster } }
  )
}

// Get detection evidence
export function getDetectionEvidence(workloadUid: string, source?: string, cluster?: string) {
  return request.get<any, { workloadUid: string; evidence: DetectionEvidence[]; total: number }>(
    `/detection-status/${encodeURIComponent(workloadUid)}/evidence`,
    { params: { source, cluster } }
  )
}

// Trigger detection
export function triggerDetection(workloadUid: string) {
  return request.post<any, { message: string; workloadUid: string; taskType: string; status: string }>(
    `/detection-status/${encodeURIComponent(workloadUid)}/trigger`
  )
}

