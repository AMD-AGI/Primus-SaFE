package model

// ResourceFilter defines a filter for silencing alerts based on resource attributes
type ResourceFilter struct {
	ResourceType string `json:"resource_type"` // Type: node/gpu/workload/pod
	ResourceName string `json:"resource_name"` // Resource name or pattern (supports wildcards)
	Namespace    string `json:"namespace,omitempty"` // Kubernetes namespace (for pods/workloads)
}

// LabelMatcher defines a label matcher for flexible alert filtering
type LabelMatcher struct {
	Name     string `json:"name"`            // Label name
	Value    string `json:"value"`           // Label value
	Operator string `json:"operator"`        // Operator: =, !=, =~, !~
	IsRegex  bool   `json:"is_regex,omitempty"` // Whether value is a regex pattern
}

// TimeWindow defines a recurring time window for alert silencing
type TimeWindow struct {
	DaysOfWeek []int  `json:"days_of_week"` // Days: 0=Sunday, 1=Monday, etc.
	StartTime  string `json:"start_time"`   // Time in HH:MM format
	EndTime    string `json:"end_time"`     // Time in HH:MM format
	Timezone   string `json:"timezone,omitempty"` // Timezone (default: UTC)
}

