package model

import (
	"time"
)

// MetricAlertRule represents a metric-based alert rule that syncs to VMRule
type MetricAlertRule struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Name        string    `gorm:"column:name;type:varchar(255);not null;uniqueIndex:idx_name_cluster" json:"name"`
	ClusterName string    `gorm:"column:cluster_name;type:varchar(100);not null;uniqueIndex:idx_name_cluster;index:idx_cluster" json:"cluster_name"`
	Enabled     bool      `gorm:"column:enabled;default:true" json:"enabled"`
	
	// VMRule specification
	Groups ExtType `gorm:"column:groups;type:jsonb;not null" json:"groups"` // Array of VMRule groups
	
	// Metadata
	Description string  `gorm:"column:description;type:text" json:"description,omitempty"`
	Labels      ExtType `gorm:"column:labels;type:jsonb" json:"labels,omitempty"`
	
	// Resource mapping configuration
	ResourceMapping ExtType `gorm:"column:resource_mapping;type:jsonb" json:"resource_mapping,omitempty"`
	
	// Alert enrichment configuration
	AlertEnrichment ExtType `gorm:"column:alert_enrichment;type:jsonb" json:"alert_enrichment,omitempty"`
	
	// Alert grouping configuration
	AlertGrouping ExtType `gorm:"column:alert_grouping;type:jsonb" json:"alert_grouping,omitempty"`
	
	// Alert routing configuration
	AlertRouting ExtType `gorm:"column:alert_routing;type:jsonb" json:"alert_routing,omitempty"`
	
	// Sync status
	SyncStatus    string     `gorm:"column:sync_status;type:varchar(50);default:'pending'" json:"sync_status"` // pending/synced/failed
	SyncMessage   string     `gorm:"column:sync_message;type:text" json:"sync_message,omitempty"`
	LastSyncAt    *time.Time `gorm:"column:last_sync_at" json:"last_sync_at,omitempty"`
	VMRuleUID     string     `gorm:"column:vmrule_uid;type:varchar(255)" json:"vmrule_uid,omitempty"`
	
	// VMRule status (from Kubernetes)
	VMRuleStatus ExtType `gorm:"column:vmrule_status;type:jsonb" json:"vmrule_status,omitempty"`
	
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	CreatedBy string    `gorm:"column:created_by;type:varchar(255)" json:"created_by,omitempty"`
	UpdatedBy string    `gorm:"column:updated_by;type:varchar(255)" json:"updated_by,omitempty"`
}

// TableName returns the table name for MetricAlertRule
func (*MetricAlertRule) TableName() string {
	return "metric_alert_rules"
}

// VMRuleGroup represents a group of alert rules in VMRule format
type VMRuleGroup struct {
	Name     string        `json:"name"`
	Interval string        `json:"interval,omitempty"`
	Rules    []VMRuleAlert `json:"rules"`
}

// VMRuleAlert represents a single alert rule in VMRule format
type VMRuleAlert struct {
	Alert       string            `json:"alert"`
	Expr        string            `json:"expr"`
	For         string            `json:"for,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// VMRuleSpec represents the complete VMRule specification
type VMRuleSpec struct {
	Groups []VMRuleGroup `json:"groups"`
}

// VMRuleStatus represents the status of a VMRule in Kubernetes
type VMRuleStatus struct {
	Phase          string                 `json:"phase,omitempty"` // Active/Inactive/Failed
	LastUpdateTime string                 `json:"lastUpdateTime,omitempty"`
	Conditions     []VMRuleCondition      `json:"conditions,omitempty"`
	RuleStatus     map[string]interface{} `json:"ruleStatus,omitempty"`
}

// VMRuleCondition represents a condition in VMRule status
type VMRuleCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
}

// ResourceMappingConfig defines how to map Prometheus labels to Primus-Lens resources
type ResourceMappingConfig struct {
	Enabled           bool                `json:"enabled"`
	Mappings          []ResourceMapping   `json:"mappings,omitempty"`
	OnMappingFailure  string              `json:"on_mapping_failure,omitempty"` // ignore/warn/suppress
}

// ResourceMapping defines a single resource mapping rule
type ResourceMapping struct {
	ResourceType   string `json:"resource_type"`         // node/gpu/pod/workload
	LabelKey       string `json:"label_key"`             // Prometheus label key
	IdentifierType string `json:"identifier_type"`       // name/ip/uuid/index
	Required       bool   `json:"required"`              // Whether this mapping is required
	ParentResource string `json:"parent_resource,omitempty"` // Parent resource dependency
	NamespaceLabel string `json:"namespace_label,omitempty"` // For resources that need namespace
}

// AlertEnrichmentConfig defines how to enrich alert with resource information
type AlertEnrichmentConfig struct {
	Enabled            bool              `json:"enabled"`
	AddResourceLinks   bool              `json:"add_resource_links"`   // Add resource detail page links
	AddRelatedResources bool             `json:"add_related_resources"` // Add related resource info
	CustomAnnotations  map[string]string `json:"custom_annotations,omitempty"` // Custom annotations template
}

// AlertGroupingConfig defines how to group alerts
type AlertGroupingConfig struct {
	Enabled        bool     `json:"enabled"`
	GroupBy        []string `json:"group_by,omitempty"`     // Labels to group by
	GroupWait      string   `json:"group_wait,omitempty"`   // Wait time for same group
	GroupInterval  string   `json:"group_interval,omitempty"` // Resend interval
	RepeatInterval string   `json:"repeat_interval,omitempty"` // Repeat notification interval
}

// AlertRoutingConfig defines alert routing rules
type AlertRoutingConfig struct {
	Enabled bool          `json:"enabled"`
	Routes  []RouteRule   `json:"routes,omitempty"`
}

// RouteRule defines a single routing rule
type RouteRule struct {
	Match    map[string]string `json:"match"`    // Label matchers
	Receiver string            `json:"receiver"` // Receiver name
	Continue bool              `json:"continue"` // Continue to next rule
}

