// Notification Channel Types
export type NotificationChannelType = 'email' | 'webhook' | 'dingtalk' | 'wechat' | 'slack' | 'alertmanager'

export interface EmailChannelConfig {
  smtp_host: string
  smtp_port: number
  username?: string
  password?: string
  from: string
  from_name?: string
  use_starttls?: boolean
  skip_verify?: boolean
}

export interface WebhookChannelConfig {
  url: string
  method?: string
  headers?: Record<string, string>
  timeout?: number
}

export interface DingTalkChannelConfig {
  webhook_url: string
  secret?: string
}

export interface SlackChannelConfig {
  webhook_url: string
  channel?: string
  username?: string
}

export interface WeChatChannelConfig {
  corp_id: string
  agent_id: number
  secret: string
  to_user?: string
  to_party?: string
  to_tag?: string
}

export interface AlertManagerChannelConfig {
  url: string
  timeout?: number
  api_path?: string
}

export type ChannelConfigType = 
  | EmailChannelConfig 
  | WebhookChannelConfig 
  | DingTalkChannelConfig 
  | SlackChannelConfig 
  | WeChatChannelConfig 
  | AlertManagerChannelConfig

export interface NotificationChannel {
  id: number
  name: string
  type: NotificationChannelType
  enabled: boolean
  config: Record<string, any>
  description?: string
  created_at: string
  updated_at: string
  created_by?: string
  updated_by?: string
}

export interface ChannelTypeInfo {
  type: NotificationChannelType
  name: string
  description: string
  config_schema: Record<string, string>
}

export interface ListChannelsParams {
  type?: NotificationChannelType
  enabled?: boolean
  search?: string
  offset?: number
  limit?: number
}

// Alert Event Types
export type AlertSource = 'metric' | 'log' | 'trace'
export type AlertSeverity = 'critical' | 'high' | 'warning' | 'info'
export type AlertStatus = 'firing' | 'resolved' | 'silenced'

export interface AlertEvent {
  id: string
  source: AlertSource
  alertName: string
  severity: AlertSeverity
  status: AlertStatus
  startsAt: string
  endsAt?: string
  labels: Record<string, string>
  annotations: Record<string, string>
  workloadId?: string
  workloadName?: string
  podName?: string
  podId?: string
  nodeName?: string
  clusterName?: string
  rawData?: any
  enrichedData?: EnrichedData
}

export interface EnrichedData {
  workloadInfo?: {
    name: string
    namespace: string
    kind: string
    gpuRequest: number
  }
  podInfo?: {
    phase: string
    containers: string[]
  }
  nodeInfo?: {
    gpuCount: number
    gpuModel: string
  }
}

export interface AlertCorrelation {
  correlationId: string
  alerts: AlertEvent[]
  correlationType: 'time' | 'entity' | 'causal' | 'cross_source'
  correlationScore?: number
  reason?: string
}

// Alert Statistics Types
export interface SeverityStats {
  count: number
  change: number
}

export interface AlertStatistics {
  critical: SeverityStats
  high: SeverityStats
  warning: SeverityStats
  info: SeverityStats
}

export interface AlertTrendPoint {
  timestamp: string
  critical: number
  high: number
  warning: number
  info: number
}

export interface TopAlertSource {
  alertName: string
  count: number
}

export interface ClusterAlertCount {
  clusterName: string
  count: number
}

// Metric Alert Rule Types
export interface VMRule {
  alert: string
  expr: string
  for?: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
}

export interface VMRuleGroup {
  name: string
  interval?: string
  rules: VMRule[]
}

export type SyncStatus = 'pending' | 'synced' | 'failed'

export interface MetricAlertRule {
  id: number
  name: string
  clusterName: string
  enabled: boolean
  groups: VMRuleGroup[]
  description?: string
  labels?: Record<string, string>
  syncStatus: SyncStatus
  syncMessage?: string
  lastSyncAt?: string
  createdAt: string
  updatedAt: string
}

// Log Alert Rule Types
export type MatchType = 'pattern' | 'threshold' | 'composite'

export interface PatternConfig {
  pattern: string
  flags?: {
    caseInsensitive?: boolean
    multiLine?: boolean
  }
}

export interface ThresholdConfig {
  pattern: string
  threshold: {
    count: number
    window: string // e.g., "10m"
  }
}

export interface CompositeConfig {
  operator: 'and' | 'or'
  conditions: (PatternConfig | ThresholdConfig)[]
}

export type MatchConfig = PatternConfig | ThresholdConfig | CompositeConfig

export interface LabelSelector {
  key: string
  operator: '=' | '!=' | '=~' | '!~' | 'in' | 'notin'
  value: string | string[]
}

export interface AlertTemplate {
  title: string
  summary: string
  description?: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
}

export interface RouteConfig {
  channels: ChannelConfig[]
  continueRouting?: boolean
}

export interface ChannelConfig {
  type: 'webhook' | 'email' | 'dingtalk' | 'slack' | 'wechat' | 'alertmanager'
  config: Record<string, any>
}

export interface LogAlertRule {
  id: number
  name: string
  description?: string
  clusterName: string
  enabled: boolean
  priority: number
  matchType: MatchType
  matchConfig: MatchConfig
  labelSelectors?: LabelSelector[]
  severity: AlertSeverity
  groupBy?: string[]
  groupWait?: number
  repeatInterval?: number
  alertTemplate?: AlertTemplate
  routeConfig?: RouteConfig
  triggerCount?: number
  lastTriggeredAt?: string
  createdAt: string
  updatedAt: string
}

export interface LogAlertRuleVersion {
  id: number
  ruleId: number
  version: number
  config: LogAlertRule
  changeType: 'create' | 'update' | 'delete'
  changedBy?: string
  changeReason?: string
  createdAt: string
}

// Alert Silence Types
export type SilenceType = 'resource' | 'label' | 'alert_name' | 'expression'

export interface ResourceFilter {
  resourceType: 'cluster' | 'node' | 'workload' | 'namespace' | 'pod'
  operator: '=' | '!=' | '=~' | '!~' | 'in' | 'notin'
  value: string | string[]
}

export interface LabelMatcher {
  name: string
  operator: '=' | '!=' | '=~' | '!~'
  value: string
}

export interface TimeWindow {
  daysOfWeek: number[]  // 0-6 (Sun-Sat)
  startTime: string     // "HH:mm"
  endTime: string       // "HH:mm"
  timezone?: string
}

export interface AlertSilence {
  id: string
  name: string
  description?: string
  clusterName?: string
  enabled: boolean
  silenceType: SilenceType
  resourceFilters?: ResourceFilter[]
  labelMatchers?: LabelMatcher[]
  alertNames?: string[]
  matchExpression?: string
  startsAt: string
  endsAt?: string
  timeWindows?: TimeWindow[]
  reason: string
  ticketUrl?: string
  createdBy?: string
  createdAt: string
  silencedCount?: number
}

// Alert Template Types
export interface LogAlertRuleTemplate {
  id: number
  name: string
  description?: string
  category: string
  matchType: MatchType
  matchConfig: MatchConfig
  severity: AlertSeverity
  alertTemplate?: AlertTemplate
  builtIn: boolean
  createdAt: string
}

// Alert Advice Types
export type AdviceStatus = 'pending' | 'accepted' | 'rejected'

export interface AlertRuleAdvice {
  id: number
  workloadId?: string
  workloadName?: string
  category: string
  priority: number
  confidence: number
  reason: string
  suggestedRule: Partial<LogAlertRule>
  status: AdviceStatus
  statusReason?: string
  appliedRuleId?: number
  createdAt: string
  updatedAt: string
}

export interface AdviceSummary {
  pending: number
  accepted: number
  rejected: number
  avgConfidence: number
}

// API Response Types
export interface PaginatedResponse<T> {
  data: T[]
  total: number
  offset: number
  limit: number
}

// Filter/Query Types
export interface ListAlertsParams {
  cluster?: string
  source?: AlertSource
  alertName?: string
  severity?: AlertSeverity
  status?: AlertStatus
  workloadId?: string
  podName?: string
  nodeName?: string
  startsAfter?: string
  startsBefore?: string
  offset?: number
  limit?: number
}

export interface AlertStatisticsParams {
  dateFrom?: string
  dateTo?: string
  clusterName?: string
  groupBy?: 'hour' | 'day'
}

export interface ListMetricRulesParams {
  cluster?: string
  enabled?: boolean
  syncStatus?: SyncStatus
  search?: string
  offset?: number
  limit?: number
}

export interface ListLogRulesParams {
  cluster?: string
  enabled?: boolean
  matchType?: MatchType
  severity?: AlertSeverity
  search?: string
  offset?: number
  limit?: number
}

export interface ListSilencesParams {
  cluster?: string
  enabled?: boolean
  silenceType?: SilenceType
  search?: string
  offset?: number
  limit?: number
}

export interface ListAdvicesParams {
  cluster?: string
  workloadId?: string
  status?: AdviceStatus
  category?: string
  minPriority?: number
  offset?: number
  limit?: number
}
