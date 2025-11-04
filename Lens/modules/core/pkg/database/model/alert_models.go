package model

import (
	"time"
)

// AlertEvent represents a unified alert event from different sources (metric/log/trace)
type AlertEvent struct {
	ID          string    `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"` // fingerprint or UUID
	Source      string    `gorm:"column:source;type:varchar(20);not null;index:idx_source_status,priority:1" json:"source"` // metric/log/trace
	AlertName   string    `gorm:"column:alert_name;type:varchar(255);not null;index:idx_alert_name" json:"alert_name"`
	Severity    string    `gorm:"column:severity;type:varchar(20);not null" json:"severity"` // critical/high/warning/info
	Status      string    `gorm:"column:status;type:varchar(20);not null;default:'firing';index:idx_source_status,priority:2" json:"status"` // firing/resolved/silenced
	
	// Timestamps
	StartsAt  time.Time  `gorm:"column:starts_at;not null;index:idx_starts_at,sort:desc" json:"starts_at"`
	EndsAt    *time.Time `gorm:"column:ends_at" json:"ends_at,omitempty"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	
	// Labels and Annotations (stored as JSONB)
	Labels      ExtType `gorm:"column:labels;type:jsonb;not null" json:"labels"`
	Annotations ExtType `gorm:"column:annotations;type:jsonb" json:"annotations"`
	
	// Context information
	WorkloadID  string `gorm:"column:workload_id;type:varchar(255);index:idx_workload" json:"workload_id,omitempty"`
	PodName     string `gorm:"column:pod_name;type:varchar(255);index:idx_pod" json:"pod_name,omitempty"`
	PodID       string `gorm:"column:pod_id;type:varchar(255)" json:"pod_id,omitempty"`
	NodeName    string `gorm:"column:node_name;type:varchar(255);index:idx_node" json:"node_name,omitempty"`
	ClusterName string `gorm:"column:cluster_name;type:varchar(100)" json:"cluster_name,omitempty"`
	
	// Raw and enriched data
	RawData      ExtType `gorm:"column:raw_data;type:jsonb" json:"raw_data,omitempty"`
	EnrichedData ExtType `gorm:"column:enriched_data;type:jsonb" json:"enriched_data,omitempty"`
	
	// Routing and notification
	RouteKey           string     `gorm:"column:route_key;type:varchar(255)" json:"route_key,omitempty"`
	NotifiedAt         *time.Time `gorm:"column:notified_at" json:"notified_at,omitempty"`
	NotificationStatus string     `gorm:"column:notification_status;type:varchar(50)" json:"notification_status,omitempty"`
}

// TableName returns the table name for AlertEvent
func (*AlertEvent) TableName() string {
	return "alert_events"
}

// AlertCorrelation represents the relationship between multiple alerts
type AlertCorrelation struct {
	ID                   int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	CorrelationID        string    `gorm:"column:correlation_id;type:varchar(64);not null;index:idx_correlation_id" json:"correlation_id"`
	AlertID              string    `gorm:"column:alert_id;type:varchar(64);not null;index:idx_alert_id" json:"alert_id"`
	CorrelationType      string    `gorm:"column:correlation_type;type:varchar(50);not null" json:"correlation_type"` // time/entity/causal/cross_source
	CorrelationScore     float64   `gorm:"column:correlation_score" json:"correlation_score,omitempty"`
	CorrelationReason    string    `gorm:"column:correlation_reason;type:text" json:"correlation_reason,omitempty"`
	CorrelationMetadata  ExtType   `gorm:"column:correlation_metadata;type:jsonb" json:"correlation_metadata,omitempty"`
	CreatedAt            time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName returns the table name for AlertCorrelation
func (*AlertCorrelation) TableName() string {
	return "alert_correlations"
}

// AlertStatistic represents aggregated alert statistics for fast querying
type AlertStatistic struct {
	ID                   int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Date                 time.Time `gorm:"column:date;type:date;not null;uniqueIndex:idx_stat_unique,priority:1;index:idx_date,sort:desc" json:"date"`
	Hour                 int       `gorm:"column:hour;uniqueIndex:idx_stat_unique,priority:2" json:"hour,omitempty"` // 0-23, null for daily stats
	AlertName            string    `gorm:"column:alert_name;type:varchar(255);not null;uniqueIndex:idx_stat_unique,priority:3" json:"alert_name"`
	Source               string    `gorm:"column:source;type:varchar(20);not null;uniqueIndex:idx_stat_unique,priority:4" json:"source"`
	Severity             string    `gorm:"column:severity;type:varchar(20);not null" json:"severity"`
	WorkloadID           string    `gorm:"column:workload_id;type:varchar(255);uniqueIndex:idx_stat_unique,priority:5" json:"workload_id,omitempty"`
	ClusterName          string    `gorm:"column:cluster_name;type:varchar(100);uniqueIndex:idx_stat_unique,priority:6" json:"cluster_name,omitempty"`
	FiringCount          int       `gorm:"column:firing_count;default:0" json:"firing_count"`
	ResolvedCount        int       `gorm:"column:resolved_count;default:0" json:"resolved_count"`
	TotalDurationSeconds int64     `gorm:"column:total_duration_seconds;default:0" json:"total_duration_seconds"`
	AvgDurationSeconds   float64   `gorm:"column:avg_duration_seconds" json:"avg_duration_seconds,omitempty"`
	CreatedAt            time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName returns the table name for AlertStatistic
func (*AlertStatistic) TableName() string {
	return "alert_statistics"
}

// AlertRule represents a unified alert rule configuration
type AlertRule struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Name            string    `gorm:"column:name;type:varchar(255);uniqueIndex;not null" json:"name"`
	Source          string    `gorm:"column:source;type:varchar(20);not null;index:idx_source_enabled,priority:1" json:"source"` // metric/log/trace
	Enabled         bool      `gorm:"column:enabled;default:true;index:idx_source_enabled,priority:2" json:"enabled"`
	RuleType        string    `gorm:"column:rule_type;type:varchar(50);not null" json:"rule_type"` // threshold/pattern/anomaly/absence
	RuleConfig      ExtType   `gorm:"column:rule_config;type:jsonb;not null" json:"rule_config"`
	Severity        string    `gorm:"column:severity;type:varchar(20);default:'warning'" json:"severity"`
	Labels          ExtType   `gorm:"column:labels;type:jsonb" json:"labels,omitempty"`
	Annotations     ExtType   `gorm:"column:annotations;type:jsonb" json:"annotations,omitempty"`
	RouteConfig     ExtType   `gorm:"column:route_config;type:jsonb" json:"route_config,omitempty"`
	InhibitRules    ExtType   `gorm:"column:inhibit_rules;type:jsonb" json:"inhibit_rules,omitempty"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	CreatedBy       string    `gorm:"column:created_by;type:varchar(255)" json:"created_by,omitempty"`
}

// TableName returns the table name for AlertRule
func (*AlertRule) TableName() string {
	return "alert_rules"
}

// AlertSilence represents a silence configuration for alerts
type AlertSilence struct {
	ID          string    `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"`
	Name        string    `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Description string    `gorm:"column:description;type:text" json:"description,omitempty"`
	ClusterName string    `gorm:"column:cluster_name;type:varchar(100);index:idx_cluster" json:"cluster_name,omitempty"`
	Enabled     bool      `gorm:"column:enabled;default:true;index:idx_enabled" json:"enabled"`
	
	// Silence type: resource/label/alert_name/expression
	SilenceType string `gorm:"column:silence_type;type:varchar(50);not null" json:"silence_type"`
	
	// Resource-based silence
	ResourceFilters ExtType `gorm:"column:resource_filters;type:jsonb" json:"resource_filters,omitempty"`
	
	// Label-based silence
	LabelMatchers ExtType `gorm:"column:label_matchers;type:jsonb" json:"label_matchers,omitempty"`
	
	// Alert name-based silence
	AlertNames ExtType `gorm:"column:alert_names;type:jsonb" json:"alert_names,omitempty"`
	
	// Expression-based silence (advanced)
	MatchExpression string `gorm:"column:match_expression;type:text" json:"match_expression,omitempty"`
	
	// Time configuration
	StartsAt  time.Time  `gorm:"column:starts_at;index:idx_time_range,priority:1" json:"starts_at"`
	EndsAt    *time.Time `gorm:"column:ends_at;index:idx_time_range,priority:2" json:"ends_at,omitempty"` // null = permanent
	TimeWindows ExtType  `gorm:"column:time_windows;type:jsonb" json:"time_windows,omitempty"` // Recurring time windows
	
	// Metadata
	Reason    string `gorm:"column:reason;type:text" json:"reason,omitempty"`
	TicketURL string `gorm:"column:ticket_url;type:varchar(500)" json:"ticket_url,omitempty"`
	
	CreatedBy string    `gorm:"column:created_by;type:varchar(255)" json:"created_by,omitempty"`
	UpdatedBy string    `gorm:"column:updated_by;type:varchar(255)" json:"updated_by,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for AlertSilence
func (*AlertSilence) TableName() string {
	return "alert_silences"
}

// AlertNotification represents a notification record for an alert
type AlertNotification struct {
	ID                  int64      `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	AlertID             string     `gorm:"column:alert_id;type:varchar(64);not null;index:idx_alert_id" json:"alert_id"`
	Channel             string     `gorm:"column:channel;type:varchar(50);not null" json:"channel"` // webhook/email/dingtalk/wechat/slack
	ChannelConfig       ExtType    `gorm:"column:channel_config;type:jsonb" json:"channel_config,omitempty"`
	Status              string     `gorm:"column:status;type:varchar(20);not null;index:idx_status" json:"status"` // pending/sent/failed
	SentAt              *time.Time `gorm:"column:sent_at" json:"sent_at,omitempty"`
	ErrorMessage        string     `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
	RetryCount          int        `gorm:"column:retry_count;default:0" json:"retry_count"`
	NotificationPayload ExtType    `gorm:"column:notification_payload;type:jsonb" json:"notification_payload,omitempty"`
	CreatedAt           time.Time  `gorm:"column:created_at;autoCreateTime;index:idx_created_at,sort:desc" json:"created_at"`
}

// TableName returns the table name for AlertNotification
func (*AlertNotification) TableName() string {
	return "alert_notifications"
}

// ResourceFilter defines filtering rules based on resources
type ResourceFilter struct {
	ResourceType  string            `json:"resource_type"`  // node/gpu/pod/workload
	ResourceIDs   []string          `json:"resource_ids,omitempty"`   // Resource ID list
	ResourceNames []string          `json:"resource_names,omitempty"` // Resource name list
	NamePattern   string            `json:"name_pattern,omitempty"`   // Name pattern (e.g., "gpu-node-*")
	LabelSelector map[string]string `json:"label_selector,omitempty"` // Label selector
}

// LabelMatcher defines label matching rules
type LabelMatcher struct {
	Key      string `json:"key"`
	Operator string `json:"operator"` // =, !=, =~, !~ (regex)
	Value    string `json:"value"`
}

// TimeWindow defines recurring time windows
type TimeWindow struct {
	DayOfWeek []int  `json:"day_of_week,omitempty"` // 0-6, Sunday=0
	StartTime string `json:"start_time"`            // "22:00"
	EndTime   string `json:"end_time"`              // "06:00"
}

// SilencedAlert represents a record of silenced alert (for audit)
type SilencedAlert struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	SilenceID   string    `gorm:"column:silence_id;type:varchar(64);not null;index:idx_silence_id" json:"silence_id"`
	AlertID     string    `gorm:"column:alert_id;type:varchar(64);not null;index:idx_alert_id" json:"alert_id"`
	AlertName   string    `gorm:"column:alert_name;type:varchar(255);not null" json:"alert_name"`
	ClusterName string    `gorm:"column:cluster_name;type:varchar(100)" json:"cluster_name,omitempty"`
	SilencedAt  time.Time `gorm:"column:silenced_at;not null;index:idx_silenced_at,sort:desc" json:"silenced_at"`
	Reason      string    `gorm:"column:reason;type:text" json:"reason,omitempty"`
	AlertData   ExtType   `gorm:"column:alert_data;type:jsonb" json:"alert_data,omitempty"`
}

// TableName returns the table name for SilencedAlert
func (*SilencedAlert) TableName() string {
	return "silenced_alerts"
}

// AssociatedResource represents a resource associated with an alert
type AssociatedResource struct {
	ResourceType   string                 `json:"resource_type"`   // node/gpu/pod/workload
	ResourceID     string                 `json:"resource_id"`
	ResourceName   string                 `json:"resource_name"`
	ClusterName    string                 `json:"cluster_name,omitempty"`
	Namespace      string                 `json:"namespace,omitempty"`
	ParentResourceID string               `json:"parent_resource_id,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Link           string                 `json:"link,omitempty"` // URL to resource detail page
}

// EnrichedAlert represents an alert with enriched resource information
type EnrichedAlert struct {
	AlertName           string               `json:"alert_name"`
	Status              string               `json:"status"`
	Severity            string               `json:"severity"`
	Value               interface{}          `json:"value,omitempty"`
	StartsAt            time.Time            `json:"starts_at"`
	Labels              map[string]string    `json:"labels"`
	Annotations         map[string]string    `json:"annotations"`
	AssociatedResources []AssociatedResource `json:"associated_resources,omitempty"`
	EnrichedAnnotations map[string]string    `json:"enriched_annotations,omitempty"`
	RecoverySuggestions []RecoverySuggestion `json:"recovery_suggestions,omitempty"`
}

// RecoverySuggestion represents a suggested action to recover from alert
type RecoverySuggestion struct {
	Action              string `json:"action"`
	Description         string `json:"description"`
	Command             string `json:"command,omitempty"`
	AutomationAvailable bool   `json:"automation_available"`
	AutomationEndpoint  string `json:"automation_endpoint,omitempty"`
}

