package model

import (
	"time"
)

// AlertRuleAdvice represents a recommended alert rule from cluster inspection
// It can be either a log alert rule or metric alert rule recommendation
type AlertRuleAdvice struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	RuleType    string    `gorm:"column:rule_type;type:varchar(20);not null;index:idx_rule_type" json:"rule_type"` // log/metric
	Name        string    `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Title       string    `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Description string    `gorm:"column:description;type:text" json:"description"`
	Category    string    `gorm:"column:category;type:varchar(50);not null;index:idx_category" json:"category"` // performance/error/resource/security/availability
	
	// Target information
	ClusterName     string    `gorm:"column:cluster_name;type:varchar(100);not null;index:idx_cluster_name" json:"cluster_name"`
	TargetResource  string    `gorm:"column:target_resource;type:varchar(50)" json:"target_resource,omitempty"` // workload/node/namespace/pod
	TargetName      string    `gorm:"column:target_name;type:varchar(255)" json:"target_name,omitempty"`
	
	// Advice configuration
	// For log rules: contains label_selectors, match_type, match_config, etc.
	// For metric rules: contains groups with alert rules
	RuleConfig      ExtType   `gorm:"column:rule_config;type:jsonb;not null" json:"rule_config"`
	
	// Severity and priority
	Severity        string    `gorm:"column:severity;type:varchar(20);default:'warning'" json:"severity"` // critical/warning/info
	Priority        int       `gorm:"column:priority;default:5" json:"priority"` // 1-10, higher number = higher priority
	
	// Reasoning and evidence
	Reason          string    `gorm:"column:reason;type:text" json:"reason"` // Why this rule is recommended
	Evidence        ExtType   `gorm:"column:evidence;type:jsonb" json:"evidence,omitempty"` // Supporting data, logs, metrics
	
	// Status tracking
	Status          string    `gorm:"column:status;type:varchar(20);default:'pending';index:idx_status" json:"status"` // pending/reviewed/accepted/rejected/applied
	ReviewedBy      string    `gorm:"column:reviewed_by;type:varchar(255)" json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time `gorm:"column:reviewed_at" json:"reviewed_at,omitempty"`
	ReviewNotes     string    `gorm:"column:review_notes;type:text" json:"review_notes,omitempty"`
	
	// Applied rule tracking
	AppliedRuleID   *int64    `gorm:"column:applied_rule_id" json:"applied_rule_id,omitempty"` // ID of the created rule if applied
	AppliedAt       *time.Time `gorm:"column:applied_at" json:"applied_at,omitempty"`
	
	// Inspection metadata
	InspectionID    string    `gorm:"column:inspection_id;type:varchar(100);index:idx_inspection_id" json:"inspection_id,omitempty"` // ID of the inspection that generated this advice
	InspectionTime  time.Time `gorm:"column:inspection_time;not null" json:"inspection_time"`
	
	// Tags for categorization
	Tags            []string  `gorm:"column:tags;type:text[];serializer:json" json:"tags,omitempty"`
	
	// Confidence score (0.0-1.0)
	ConfidenceScore float64   `gorm:"column:confidence_score;default:0.5" json:"confidence_score"`
	
	// Expiry - advice may become irrelevant after some time
	ExpiresAt       *time.Time `gorm:"column:expires_at;index:idx_expires_at" json:"expires_at,omitempty"`
	
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime;index:idx_created_at" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	CreatedBy       string    `gorm:"column:created_by;type:varchar(255)" json:"created_by,omitempty"`
}

// TableName returns the table name for AlertRuleAdvice
func (*AlertRuleAdvice) TableName() string {
	return "alert_rule_advices"
}

// AlertRuleAdviceStatistics represents aggregated statistics for rule advices
type AlertRuleAdviceStatistics struct {
	ID                  int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ClusterName         string    `gorm:"column:cluster_name;type:varchar(100);not null;uniqueIndex:idx_cluster_date,priority:1" json:"cluster_name"`
	Date                time.Time `gorm:"column:date;type:date;not null;uniqueIndex:idx_cluster_date,priority:2;index:idx_date" json:"date"`
	
	// Counts by rule type
	LogRuleCount        int64     `gorm:"column:log_rule_count;default:0" json:"log_rule_count"`
	MetricRuleCount     int64     `gorm:"column:metric_rule_count;default:0" json:"metric_rule_count"`
	
	// Counts by category
	PerformanceCount    int64     `gorm:"column:performance_count;default:0" json:"performance_count"`
	ErrorCount          int64     `gorm:"column:error_count;default:0" json:"error_count"`
	ResourceCount       int64     `gorm:"column:resource_count;default:0" json:"resource_count"`
	SecurityCount       int64     `gorm:"column:security_count;default:0" json:"security_count"`
	AvailabilityCount   int64     `gorm:"column:availability_count;default:0" json:"availability_count"`
	
	// Counts by status
	PendingCount        int64     `gorm:"column:pending_count;default:0" json:"pending_count"`
	ReviewedCount       int64     `gorm:"column:reviewed_count;default:0" json:"reviewed_count"`
	AcceptedCount       int64     `gorm:"column:accepted_count;default:0" json:"accepted_count"`
	RejectedCount       int64     `gorm:"column:rejected_count;default:0" json:"rejected_count"`
	AppliedCount        int64     `gorm:"column:applied_count;default:0" json:"applied_count"`
	
	// Average scores
	AvgConfidenceScore  float64   `gorm:"column:avg_confidence_score" json:"avg_confidence_score"`
	AvgPriority         float64   `gorm:"column:avg_priority" json:"avg_priority"`
	
	CreatedAt           time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for AlertRuleAdviceStatistics
func (*AlertRuleAdviceStatistics) TableName() string {
	return "alert_rule_advice_statistics"
}

