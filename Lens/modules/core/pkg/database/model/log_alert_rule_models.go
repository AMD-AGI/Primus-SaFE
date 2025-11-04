package model

import (
	"time"
)

// LogAlertRule represents a log-based alert rule configuration
type LogAlertRule struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Name            string    `gorm:"column:name;type:varchar(255);not null;uniqueIndex:idx_name_cluster" json:"name"`
	Description     string    `gorm:"column:description;type:text" json:"description,omitempty"`
	ClusterName     string    `gorm:"column:cluster_name;type:varchar(100);not null;uniqueIndex:idx_name_cluster;index:idx_cluster_enabled,priority:1" json:"cluster_name"`
	Enabled         bool      `gorm:"column:enabled;default:true;index:idx_cluster_enabled,priority:2" json:"enabled"`
	Priority        int       `gorm:"column:priority;default:5" json:"priority"` // 1-10, higher number = higher priority
	
	// Label Selectors - for matching target objects (workload/namespace/pod/node)
	LabelSelectors  ExtType   `gorm:"column:label_selectors;type:jsonb;not null" json:"label_selectors"`
	
	// Match Configuration
	MatchType       string    `gorm:"column:match_type;type:varchar(50);not null" json:"match_type"` // pattern/threshold/anomaly/composite
	MatchConfig     ExtType   `gorm:"column:match_config;type:jsonb;not null" json:"match_config"`
	
	// Alert Configuration
	Severity        string    `gorm:"column:severity;type:varchar(20);default:'warning'" json:"severity"` // critical/warning/info
	AlertTemplate   ExtType   `gorm:"column:alert_template;type:jsonb" json:"alert_template"`
	
	// Grouping and Suppression
	GroupBy         []string  `gorm:"column:group_by;type:text[];serializer:json" json:"group_by,omitempty"`
	GroupWait       int64     `gorm:"column:group_wait;default:30" json:"group_wait"` // seconds
	RepeatInterval  int64     `gorm:"column:repeat_interval;default:3600" json:"repeat_interval"` // seconds
	
	// Routing Configuration
	RouteConfig     ExtType   `gorm:"column:route_config;type:jsonb" json:"route_config,omitempty"`
	
	// Metadata
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	CreatedBy       string    `gorm:"column:created_by;type:varchar(255)" json:"created_by,omitempty"`
	UpdatedBy       string    `gorm:"column:updated_by;type:varchar(255)" json:"updated_by,omitempty"`
	
	// Status tracking
	LastTriggeredAt *time.Time `gorm:"column:last_triggered_at" json:"last_triggered_at,omitempty"`
	TriggerCount    int64      `gorm:"column:trigger_count;default:0" json:"trigger_count"`
}

// TableName returns the table name for LogAlertRule
func (*LogAlertRule) TableName() string {
	return "log_alert_rules"
}

// LogAlertRuleVersion represents a version history of a rule
type LogAlertRuleVersion struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	RuleID      int64     `gorm:"column:rule_id;not null;index:idx_rule_id" json:"rule_id"`
	Version     int       `gorm:"column:version;not null" json:"version"`
	Config      ExtType   `gorm:"column:config;type:jsonb;not null" json:"config"` // Full rule configuration snapshot
	Status      string    `gorm:"column:status;type:varchar(20);default:'draft'" json:"status"` // draft/active/deprecated
	DeployedAt  *time.Time `gorm:"column:deployed_at" json:"deployed_at,omitempty"`
	CreatedBy   string    `gorm:"column:created_by;type:varchar(255)" json:"created_by,omitempty"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	ChangeLog   string    `gorm:"column:change_log;type:text" json:"change_log,omitempty"`
}

// TableName returns the table name for LogAlertRuleVersion
func (*LogAlertRuleVersion) TableName() string {
	return "log_alert_rule_versions"
}

// LogAlertRuleStatistic represents execution statistics for log alert rules
type LogAlertRuleStatistic struct {
	ID                int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	RuleID            int64     `gorm:"column:rule_id;not null;uniqueIndex:idx_rule_stat_unique,priority:1" json:"rule_id"`
	Date              time.Time `gorm:"column:date;type:date;not null;uniqueIndex:idx_rule_stat_unique,priority:2;index:idx_date,sort:desc" json:"date"`
	Hour              int       `gorm:"column:hour;uniqueIndex:idx_rule_stat_unique,priority:3" json:"hour"` // 0-23, null for daily stats
	ClusterName       string    `gorm:"column:cluster_name;type:varchar(100);not null;uniqueIndex:idx_rule_stat_unique,priority:4" json:"cluster_name"`
	EvaluatedCount    int64     `gorm:"column:evaluated_count;default:0" json:"evaluated_count"`
	MatchedCount      int64     `gorm:"column:matched_count;default:0" json:"matched_count"`
	FiredCount        int64     `gorm:"column:fired_count;default:0" json:"fired_count"`
	AvgEvalTimeMs     float64   `gorm:"column:avg_eval_time_ms" json:"avg_eval_time_ms,omitempty"`
	MaxEvalTimeMs     float64   `gorm:"column:max_eval_time_ms" json:"max_eval_time_ms,omitempty"`
	ErrorCount        int64     `gorm:"column:error_count;default:0" json:"error_count"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for LogAlertRuleStatistic
func (*LogAlertRuleStatistic) TableName() string {
	return "log_alert_rule_statistics"
}

// LogAlertRuleTemplate represents a predefined rule template
type LogAlertRuleTemplate struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Name            string    `gorm:"column:name;type:varchar(255);uniqueIndex;not null" json:"name"`
	Category        string    `gorm:"column:category;type:varchar(50);not null;index:idx_category" json:"category"` // basic/gpu/training/network/performance
	Description     string    `gorm:"column:description;type:text" json:"description,omitempty"`
	TemplateConfig  ExtType   `gorm:"column:template_config;type:jsonb;not null" json:"template_config"` // Template configuration
	Tags            []string  `gorm:"column:tags;type:text[];serializer:json" json:"tags,omitempty"`
	IsBuiltin       bool      `gorm:"column:is_builtin;default:false" json:"is_builtin"` // Whether it's a built-in template
	UsageCount      int64     `gorm:"column:usage_count;default:0" json:"usage_count"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	CreatedBy       string    `gorm:"column:created_by;type:varchar(255)" json:"created_by,omitempty"`
}

// TableName returns the table name for LogAlertRuleTemplate
func (*LogAlertRuleTemplate) TableName() string {
	return "log_alert_rule_templates"
}

