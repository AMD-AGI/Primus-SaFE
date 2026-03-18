import request from '@/services/request'
import type { ListAuditLogsParams, ListAuditLogsResponse } from './type'

export const listAuditLogs = (params?: ListAuditLogsParams): Promise<ListAuditLogsResponse> =>
  request.get('/auditlogs', { params })
