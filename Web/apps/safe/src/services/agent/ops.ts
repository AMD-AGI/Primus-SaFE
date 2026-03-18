import request from '@/services/request'

export interface AgentHealthCheckResponse {
  status: string
  version: string
  timestamp: string
}

/**
 * Agent health check
 */
export function checkAgentHealth(): Promise<AgentHealthCheckResponse> {
  return request.get('/agent/ops/api/health')
}
