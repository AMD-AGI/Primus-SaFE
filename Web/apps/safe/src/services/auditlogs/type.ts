export interface AuditLog {
  id: number
  userId: string
  userName: string
  userType: string
  clientIp: string
  action: string
  httpMethod: string
  requestPath: string
  resourceType: string
  resourceName: string
  requestBody: string
  responseStatus: number
  latencyMs: number
  traceId: string
  createTime: string
}

export interface ListAuditLogsParams {
  userId?: string
  userName?: string
  userType?: string
  resourceType?: string
  resourceName?: string
  httpMethod?: string
  requestPath?: string
  responseStatus?: number
  startTime?: string
  endTime?: string
  limit?: number
  offset?: number
  sortBy?: string
  order?: 'asc' | 'desc'
}

export interface ListAuditLogsResponse {
  totalCount: number
  items: AuditLog[]
}
