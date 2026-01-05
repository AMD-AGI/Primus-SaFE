package aitopics

// ========== scan.identify-component ==========

// IdentifyComponentInput is the input payload for component identification
type IdentifyComponentInput struct {
	Workload WorkloadInfo     `json:"workload"`
	Options  *IdentifyOptions `json:"options,omitempty"`
}

// IdentifyOptions contains optional settings for identification
type IdentifyOptions struct {
	IncludeConfidence bool `json:"include_confidence,omitempty"`
	IncludeRationale  bool `json:"include_rationale,omitempty"`
}

// IdentifyComponentOutput is the output payload for component identification
type IdentifyComponentOutput struct {
	ComponentType  string   `json:"component_type"`
	Category       string   `json:"category"`
	Confidence     float64  `json:"confidence"`
	Rationale      string   `json:"rationale,omitempty"`
	AlternateTypes []string `json:"alternate_types,omitempty"`
}

// ========== scan.suggest-grouping ==========

// SuggestGroupingInput is the input payload for grouping suggestions
type SuggestGroupingInput struct {
	Workloads      []WorkloadInfo   `json:"workloads"`
	ExistingGroups []ComponentGroup `json:"existing_groups,omitempty"`
	Options        *GroupingOptions `json:"options,omitempty"`
}

// GroupingOptions contains optional settings for grouping suggestions
type GroupingOptions struct {
	MaxSuggestions int     `json:"max_suggestions,omitempty"`
	MinConfidence  float64 `json:"min_confidence,omitempty"`
}

// SuggestGroupingOutput is the output payload for grouping suggestions
type SuggestGroupingOutput struct {
	Suggestions []GroupingSuggestion `json:"suggestions"`
}

// GroupingSuggestion represents a suggested grouping
type GroupingSuggestion struct {
	SuggestionID  string   `json:"suggestion_id"`
	GroupName     string   `json:"group_name"`
	ComponentType string   `json:"component_type"`
	Category      string   `json:"category"`
	Members       []string `json:"members"` // Workload UIDs
	Rationale     string   `json:"rationale,omitempty"`
	Confidence    float64  `json:"confidence"`
}
