import request from '@/services/request'

/**
 * Perfetto Session interface
 */
export interface PerfettoSession {
  sessionId: string
  workloadUid: string
  profilerFileId: number
  status: 'pending' | 'creating' | 'initializing' | 'ready' | 'failed' | 'expired' | 'deleted'
  statusMessage?: string
  viewerType: string
  uiPath?: string
  podName?: string
  podIp?: string
  createdAt: string
  readyAt?: string
  expiresAt: string
  lastAccessedAt?: string
  estimatedReadySeconds?: number
}

export type SessionStatus = PerfettoSession['status']

/**
 * Create session request params
 */
export interface CreateSessionParams {
  workloadUid: string
  profilerFileId: number
  ttlMinutes?: number
}

/**
 * Create a Perfetto viewer session
 */
export function createSession(params: CreateSessionParams, cluster: string): Promise<PerfettoSession> {
  return request.post<any, PerfettoSession>('/perfetto/sessions', {
    ...params,
    ttlMinutes: params.ttlMinutes || 30
  }, {
    params: { cluster }
  })
}

/**
 * Get session status
 */
export function getSession(sessionId: string, cluster: string): Promise<PerfettoSession> {
  return request.get<any, PerfettoSession>(`/perfetto/sessions/${sessionId}`, {
    params: { cluster }
  })
}

/**
 * Extend session TTL
 */
export function extendSession(sessionId: string, cluster: string, minutes: number): Promise<PerfettoSession> {
  return request.patch<any, PerfettoSession>(`/perfetto/sessions/${sessionId}`, {
    extendMinutes: minutes
  }, {
    params: { cluster }
  })
}

/**
 * Delete session
 */
export function deleteSession(sessionId: string, cluster: string): Promise<void> {
  return request.delete(`/perfetto/sessions/${sessionId}`, {
    params: { cluster }
  })
}

/**
 * Get Perfetto UI URL
 */
export function getUIUrl(sessionId: string, cluster: string): string {
  const baseURL = import.meta.env.BASE_URL || '/'
  const isDev = import.meta.env.DEV
  
  if (isDev) {
    return `/lens/v1/perfetto/sessions/${sessionId}/ui/?cluster=${cluster}`
  } else {
    return `${baseURL}v1/perfetto/sessions/${sessionId}/ui/?cluster=${cluster}`
  }
}

/**
 * Calculate remaining time
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

  if (minutes < 5) {
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

// Session status configuration
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

