import request from '@/services/request'

// Export auth-related functions
export { preAuthCheck } from './auth'

/**
 * TraceLens Session interface definition
 */
export interface TraceLensSession {
  sessionId: string
  workloadUid: string
  workloadName?: string
  profilerFileId: number
  fileName?: string
  status: 'pending' | 'creating' | 'initializing' | 'ready' | 'failed' | 'expired' | 'deleted'
  statusMessage?: string
  resourceProfile: string
  ttlMinutes: number
  createdAt: string
  expiresAt: string
  uiUrl?: string
}

export type SessionStatus = TraceLensSession['status']

/**
 * Create session request parameters
 */
export interface CreateSessionParams {
  workloadUid: string
  profilerFileId: number
  resourceProfile?: 'small' | 'medium' | 'large'
  ttlMinutes?: number
}

/**
 * Create analysis session
 */
export function createSession(params: CreateSessionParams, cluster: string): Promise<TraceLensSession> {
  return request.post<any, TraceLensSession>('/tracelens/sessions', {
    ...params,
    resourceProfile: params.resourceProfile || 'medium',
    ttlMinutes: params.ttlMinutes || 60
  }, {
    params: { cluster }
  })
}

/**
 * Query session status
 */
export function getSession(sessionId: string, cluster: string): Promise<TraceLensSession> {
  return request.get<any, TraceLensSession>(`/tracelens/sessions/${sessionId}`, {
    params: { cluster }
  })
}

/**
 * Extend session duration
 */
export function extendSession(sessionId: string, cluster: string, minutes: number): Promise<TraceLensSession> {
  return request.patch<any, TraceLensSession>(`/tracelens/sessions/${sessionId}`, {
    extendMinutes: minutes
  }, {
    params: { cluster }
  })
}

/**
 * Delete session
 */
export function deleteSession(sessionId: string, cluster: string): Promise<void> {
  return request.delete(`/tracelens/sessions/${sessionId}`, {
    params: { cluster }
  })
}

/**
 * Query all sessions for a workload
 */
export interface WorkloadSessionsResponse {
  sessions: TraceLensSession[]
}

export function listWorkloadSessions(workloadUid: string, cluster: string): Promise<WorkloadSessionsResponse> {
  return request.get<any, WorkloadSessionsResponse>(`/tracelens/workloads/${workloadUid}/sessions`, {
    params: { cluster }
  })
}

/**
 * Get TraceLens UI URL
 * Note: UI URL requires direct access to TraceLens Service
 */
export function getUIUrl(sessionId: string, cluster: string): string {
  // Determine URL based on environment
  // Dev: use relative path, handled by vite proxy (including WebSocket)
  // Prod: use relative path, handled by nginx proxy
  const baseURL = import.meta.env.BASE_URL || '/'
  const isDev = import.meta.env.DEV
  
  if (isDev) {
    // Dev environment uses relative path, let vite proxy handle it
    return `/lens/v1/tracelens/sessions/${sessionId}/ui/?cluster=${cluster}`
  } else {
    // Prod also uses relative path, ensuring cookies are sent correctly
    return `${baseURL}v1/tracelens/sessions/${sessionId}/ui/?cluster=${cluster}`
  }
}

/**
 * Calculate session remaining time
 */
export function calculateRemainingTime(expiresAt: string): {
  minutes: number
  text: string
  isExpiring: boolean
  expired: boolean
} {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diffMs = expires.getTime() - now.getTime()
  const minutes = Math.floor(diffMs / 60000)

  if (minutes <= 0) {
    return { minutes: 0, text: 'Expired', isExpiring: true, expired: true }
  }

  if (minutes < 10) {
    return { minutes, text: `Expires in ${minutes} min`, isExpiring: true, expired: false }
  }

  if (minutes < 60) {
    return { minutes, text: `${minutes} min`, isExpiring: false, expired: false }
  }

  const hours = Math.floor(minutes / 60)
  const remainingMinutes = minutes % 60
  
  if (remainingMinutes === 0) {
    return { minutes, text: `${hours}h`, isExpiring: false, expired: false }
  }
  
  return { minutes, text: `${hours}h ${remainingMinutes}m`, isExpiring: false, expired: false }
}

/**
 * Format relative time
 */
export function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const minutes = Math.floor(diffMs / 60000)

  if (minutes < 1) {
    return 'Just now'
  }

  if (minutes < 60) {
    return `${minutes} minutes ago`
  }

  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    return `${hours} hours ago`
  }

  const days = Math.floor(hours / 24)
  return `${days} days ago`
}

/**
 * Resource Profile configuration from backend
 */
export interface ResourceProfile {
  value: string
  label: string
  description: string
  memory: string
  memoryBytes: number
  cpu: number
  isDefault?: boolean
}

export interface ResourceProfilesResponse {
  profiles: ResourceProfile[]
}

// Cached resource profiles
let cachedResourceProfiles: ResourceProfile[] | null = null

/**
 * Get resource profiles from backend
 */
export async function getResourceProfiles(): Promise<ResourceProfile[]> {
  if (cachedResourceProfiles) {
    return cachedResourceProfiles
  }
  
  const response = await request.get<any, ResourceProfilesResponse>('/tracelens/resource-profiles')
  cachedResourceProfiles = response.profiles
  return cachedResourceProfiles
}

/**
 * Clear cached resource profiles (call when needed to refresh)
 */
export function clearResourceProfilesCache(): void {
  cachedResourceProfiles = null
}

// Default resource profiles (fallback when backend is not available)
export const DEFAULT_RESOURCE_PROFILES: ResourceProfile[] = [
  {
    value: 'small',
    label: 'Small (8GB Memory, 1 CPU)',
    description: 'Suitable for small trace files (< 5MB)',
    memory: '8Gi',
    memoryBytes: 8 * 1024 * 1024 * 1024,
    cpu: 1
  },
  {
    value: 'medium',
    label: 'Medium (16GB Memory, 2 CPU)',
    description: 'Recommended (5-20MB)',
    memory: '16Gi',
    memoryBytes: 16 * 1024 * 1024 * 1024,
    cpu: 2,
    isDefault: true
  },
  {
    value: 'large',
    label: 'Large (32GB Memory, 4 CPU)',
    description: 'Suitable for large trace files (> 20MB)',
    memory: '32Gi',
    memoryBytes: 32 * 1024 * 1024 * 1024,
    cpu: 4
  }
]

// Backward compatible export (deprecated, use getResourceProfiles() instead)
export const RESOURCE_PROFILES = DEFAULT_RESOURCE_PROFILES

// Export session status configuration
export const SESSION_STATUS: Record<SessionStatus, {
  label: string
  icon: string
  color: 'warning' | 'success' | 'danger' | 'info'
}> = {
  pending: { label: 'Pending', icon: '○', color: 'warning' },
  creating: { label: 'Creating', icon: '◐', color: 'warning' },
  initializing: { label: 'Initializing', icon: '◑', color: 'warning' },
  ready: { label: 'Ready', icon: '●', color: 'success' },
  failed: { label: 'Failed', icon: '✕', color: 'danger' },
  expired: { label: 'Expired', icon: '-', color: 'info' },
  deleted: { label: 'Deleted', icon: '-', color: 'info' }
}