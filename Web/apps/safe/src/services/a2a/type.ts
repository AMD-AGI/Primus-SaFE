export interface A2AService {
  id: number
  serviceName: string
  displayName: string
  description: string
  endpoint: string
  a2aPathPrefix: string
  a2aAgentCard: string
  a2aSkills: string
  a2aHealth: string
  a2aLastSeen: string
  k8sNamespace: string
  k8sService: string
  discoverySource: string
  status: string
  createdAt: string
  updatedAt: string
}

export interface A2AServiceListResponse {
  data: A2AService[]
  total: number
}

export interface A2ARegisterRequest {
  serviceName: string
  displayName: string
  endpoint: string
  a2aPathPrefix: string
  description?: string
}

export interface A2ACallLog {
  id: number
  traceId: string
  callerServiceName: string
  callerUserId: string
  targetServiceName: string
  skillId?: string
  status: string
  latencyMs: number
  requestSizeBytes: number
  responseSizeBytes: number
  errorMessage?: string
  createdAt?: string
}

export interface A2ACallLogListResponse {
  data: A2ACallLog[]
  total: number
}

export interface A2ACallLogParams {
  limit?: number
  offset?: number
  caller?: string
  target?: string
}

export interface A2ATopologyNode {
  serviceName: string
  displayName: string
  a2aHealth: string
}

export interface A2ATopologyEdge {
  caller: string
  target: string
  count: number
}

export interface A2ATopologyResponse {
  nodes: A2ATopologyNode[]
  edges: A2ATopologyEdge[]
}
