import request from '@/services/request'
import type {
  AlertEvent,
  AlertCorrelation,
  AlertStatistics,
  AlertTrendPoint,
  TopAlertSource,
  ClusterAlertCount,
  MetricAlertRule,
  LogAlertRule,
  LogAlertRuleVersion,
  AlertSilence,
  LogAlertRuleTemplate,
  AlertRuleAdvice,
  AdviceSummary,
  NotificationChannel,
  ChannelTypeInfo,
  PaginatedResponse,
  ListAlertsParams,
  AlertStatisticsParams,
  ListMetricRulesParams,
  ListLogRulesParams,
  ListSilencesParams,
  ListAdvicesParams,
  ListChannelsParams,
} from './types'

// Re-export types
export * from './types'

// ==================== Alert Events API ====================

export const alertEventsApi = {
  // List alerts with filters
  list(params: ListAlertsParams): Promise<PaginatedResponse<AlertEvent>> {
    return request.get('/alerts', { params }).then(res => ({
      data: res?.alerts || [],
      total: res?.total || 0
    }))
  },

  // Get single alert detail
  get(id: string): Promise<AlertEvent> {
    return request.get(`/alerts/${id}`)
  },

  // Get alert correlations
  getCorrelations(id: string): Promise<AlertCorrelation[]> {
    return request.get(`/alerts/${id}/correlations`).then(res => res?.correlations || [])
  },

  // Get alert statistics (summary by severity with changes)
  getStatistics(params: AlertStatisticsParams): Promise<AlertStatistics> {
    return request.get('/alerts/summary', { params })
  },

  // Get alert trend data
  getTrend(params: AlertStatisticsParams & { groupBy?: string; hours?: number }): Promise<AlertTrendPoint[]> {
    const apiParams = {
      cluster: params.clusterName,
      group_by: params.groupBy || 'hour',
      hours: params.hours || 24
    }
    return request.get('/alerts/trend', { params: apiParams })
  },

  // Get top alert sources
  getTopSources(params: { cluster?: string; limit?: number; hours?: number }): Promise<TopAlertSource[]> {
    return request.get('/alerts/top-sources', { params })
  },

  // Get alerts by cluster
  getByCluster(params: { cluster?: string; hours?: number }): Promise<ClusterAlertCount[]> {
    return request.get('/alerts/by-cluster', { params })
  },
}

// ==================== Metric Alert Rules API ====================

export const metricAlertRulesApi = {
  // List metric alert rules
  list(params: ListMetricRulesParams): Promise<PaginatedResponse<MetricAlertRule>> {
    return request.get('/metric-alert-rules', { params })
  },

  // Get single rule
  get(id: number): Promise<MetricAlertRule> {
    return request.get(`/metric-alert-rules/${id}`)
  },

  // Create rule
  create(rule: Partial<MetricAlertRule>): Promise<MetricAlertRule> {
    return request.post('/metric-alert-rules', rule)
  },

  // Update rule
  update(id: number, rule: Partial<MetricAlertRule>): Promise<MetricAlertRule> {
    return request.put(`/metric-alert-rules/${id}`, rule)
  },

  // Delete rule
  delete(id: number): Promise<void> {
    return request.delete(`/metric-alert-rules/${id}`)
  },

  // Clone rule
  clone(id: number, targetCluster?: string): Promise<MetricAlertRule> {
    return request.post(`/metric-alert-rules/${id}/clone`, { targetCluster })
  },

  // Sync rules to cluster
  sync(params: { cluster?: string; ruleIds?: number[] }): Promise<{ synced: number; failed: number }> {
    return request.post('/metric-alert-rules/sync', params)
  },

  // Get VMRule status from cluster
  getStatus(id: number): Promise<{ status: string; message?: string; lastChecked: string }> {
    return request.get(`/metric-alert-rules/${id}/status`)
  },
}

// ==================== Log Alert Rules API ====================

export const logAlertRulesApi = {
  // List log alert rules
  list(params: ListLogRulesParams): Promise<PaginatedResponse<LogAlertRule>> {
    return request.get('/log-alert-rules', { params })
  },

  // List rules across clusters
  listMultiCluster(params: ListLogRulesParams): Promise<PaginatedResponse<LogAlertRule>> {
    return request.get('/log-alert-rules/multi-cluster', { params })
  },

  // Get single rule
  get(id: number): Promise<LogAlertRule> {
    return request.get(`/log-alert-rules/${id}`)
  },

  // Create rule
  create(rule: Partial<LogAlertRule>): Promise<LogAlertRule> {
    return request.post('/log-alert-rules', rule)
  },

  // Update rule
  update(id: number, rule: Partial<LogAlertRule>): Promise<LogAlertRule> {
    return request.put(`/log-alert-rules/${id}`, rule)
  },

  // Delete rule
  delete(id: number): Promise<void> {
    return request.delete(`/log-alert-rules/${id}`)
  },

  // Batch enable/disable
  batchUpdate(ruleIds: number[], updates: { enabled?: boolean }): Promise<{ updated: number }> {
    return request.post('/log-alert-rules/batch-update', { ruleIds, updates })
  },

  // Test rule against sample log
  test(params: { pattern: string; sampleLog: string; flags?: any }): Promise<{ matched: boolean; captures?: string[] }> {
    return request.post('/log-alert-rules/test', params)
  },

  // Get rule statistics
  getStatistics(id: number): Promise<{ triggerCount: number; lastTriggeredAt?: string; trend: number[] }> {
    return request.get(`/log-alert-rules/${id}/statistics`)
  },

  // Get version history
  getVersions(id: number): Promise<LogAlertRuleVersion[]> {
    return request.get(`/log-alert-rules/${id}/versions`)
  },

  // Rollback to version
  rollback(id: number, version: number): Promise<LogAlertRule> {
    return request.post(`/log-alert-rules/${id}/rollback/${version}`)
  },

  // Clone rule
  clone(id: number, params: { name?: string; targetCluster?: string }): Promise<LogAlertRule> {
    return request.post(`/log-alert-rules/${id}/clone`, params)
  },
}

// ==================== Alert Silences API ====================

export const alertSilencesApi = {
  // List silences
  list(params: ListSilencesParams): Promise<PaginatedResponse<AlertSilence>> {
    return request.get('/alert-silences', { params }).then(res => ({
      data: res?.data || [],
      total: res?.total || 0
    }))
  },

  // Get single silence
  get(id: string): Promise<AlertSilence> {
    return request.get(`/alert-silences/${id}`)
  },

  // Create silence
  create(silence: Partial<AlertSilence>): Promise<AlertSilence> {
    return request.post('/alert-silences', silence)
  },

  // Update silence
  update(id: string, silence: Partial<AlertSilence>): Promise<AlertSilence> {
    return request.put(`/alert-silences/${id}`, silence)
  },

  // Delete silence
  delete(id: string): Promise<void> {
    return request.delete(`/alert-silences/${id}`)
  },

  // End silence early (disable)
  end(id: string): Promise<AlertSilence> {
    return request.patch(`/alert-silences/${id}/disable`)
  },

  // Get silenced alerts
  getSilencedAlerts(params: { silenceId?: string; clusterName?: string }): Promise<AlertEvent[]> {
    return request.get('/alert-silences/silenced-alerts', { params }).then(res => res?.data || [])
  },
}

// ==================== Alert Templates API ====================

export const alertTemplatesApi = {
  // List templates
  list(params: { category?: string; builtIn?: boolean }): Promise<LogAlertRuleTemplate[]> {
    return request.get('/log-alert-rule-templates', { params })
  },

  // Get single template
  get(id: number): Promise<LogAlertRuleTemplate> {
    return request.get(`/log-alert-rule-templates/${id}`)
  },

  // Create template
  create(template: Partial<LogAlertRuleTemplate>): Promise<LogAlertRuleTemplate> {
    return request.post('/log-alert-rule-templates', template)
  },

  // Delete template (only custom templates)
  delete(id: number): Promise<void> {
    return request.delete(`/log-alert-rule-templates/${id}`)
  },

  // Instantiate template (create rule from template)
  instantiate(id: number, params: { name: string; clusterName: string; overrides?: any }): Promise<LogAlertRule> {
    return request.post(`/log-alert-rule-templates/${id}/instantiate`, params)
  },
}

// ==================== Alert Advices API ====================

export const alertAdvicesApi = {
  // List advices
  list(params: ListAdvicesParams): Promise<PaginatedResponse<AlertRuleAdvice>> {
    return request.get('/alert-rule-advices', { params })
  },

  // Get single advice
  get(id: number): Promise<AlertRuleAdvice> {
    return request.get(`/alert-rule-advices/${id}`)
  },

  // Update advice status (accept/reject)
  updateStatus(id: number, status: 'accepted' | 'rejected', reason?: string): Promise<AlertRuleAdvice> {
    return request.post(`/alert-rule-advices/${id}/status`, { status, reason })
  },

  // Apply advice (create rule from advice)
  apply(id: number, overrides?: Partial<LogAlertRule>): Promise<LogAlertRule> {
    return request.post(`/alert-rule-advices/${id}/apply`, { overrides })
  },

  // Get summary statistics
  getSummary(params: { cluster?: string }): Promise<AdviceSummary> {
    return request.get('/alert-rule-advices/summary', { params })
  },

  // Refresh advices (trigger AI analysis)
  refresh(params: { cluster?: string; workloadId?: string }): Promise<{ triggered: boolean }> {
    return request.post('/alert-rule-advices/refresh', params)
  },
}

// ==================== Notification Channels API ====================

export const notificationChannelsApi = {
  // List notification channels
  list(params?: ListChannelsParams): Promise<PaginatedResponse<NotificationChannel>> {
    return request.get('/notification-channels', { params }).then(res => ({
      data: res?.items || [],
      total: res?.total || 0,
      offset: params?.offset || 0,
      limit: params?.limit || 20
    }))
  },

  // Get single channel
  get(id: number): Promise<NotificationChannel> {
    return request.get(`/notification-channels/${id}`)
  },

  // Create channel
  create(channel: Partial<NotificationChannel>): Promise<NotificationChannel> {
    return request.post('/notification-channels', channel)
  },

  // Update channel
  update(id: number, channel: Partial<NotificationChannel>): Promise<NotificationChannel> {
    return request.put(`/notification-channels/${id}`, channel)
  },

  // Delete channel
  delete(id: number): Promise<void> {
    return request.delete(`/notification-channels/${id}`)
  },

  // Test channel
  test(id: number): Promise<{ success: boolean; message: string }> {
    return request.post(`/notification-channels/${id}/test`)
  },

  // Get supported channel types
  getTypes(): Promise<ChannelTypeInfo[]> {
    return request.get('/notification-channels/types')
  },
}
