package model

// VMRuleGroup represents a group of alert rules in a VMRule CRD
type VMRuleGroup struct {
	Name     string   `json:"name" binding:"required"`
	Interval string   `json:"interval,omitempty"` // Evaluation interval, e.g., "30s", "1m"
	Rules    []VMRule `json:"rules" binding:"required"`
}

// VMRule represents a single alert rule within a VMRule group
type VMRule struct {
	Alert           string            `json:"alert" binding:"required"`  // Alert name
	Expr            string            `json:"expr" binding:"required"`   // PromQL/MetricsQL expression
	For             string            `json:"for,omitempty"`             // Duration before firing, e.g., "5m"
	Labels          map[string]string `json:"labels,omitempty"`          // Additional labels
	Annotations     map[string]string `json:"annotations,omitempty"`     // Annotations for alert description
	Keep_firing_for string            `json:"keep_firing_for,omitempty"` // Keep firing duration
	Debug           bool              `json:"debug,omitempty"`           // Enable debug mode
}

// VMRuleStatus represents the status of a VMRule in Kubernetes
// This structure matches the status field returned by VictoriaMetrics Operator
type VMRuleStatus struct {
	Phase              string            `json:"phase,omitempty"`              // Current phase: "Ready", "Pending", "Failed"
	Reason             string            `json:"reason,omitempty"`             // Reason for current phase
	Message            string            `json:"message,omitempty"`            // Human-readable message
	LastUpdateTime     string            `json:"lastUpdateTime,omitempty"`     // Last time status was updated
	Conditions         []VMRuleCondition `json:"conditions,omitempty"`         // Detailed conditions
	ObservedGeneration int64             `json:"observedGeneration,omitempty"` // Last observed generation
}

// VMRuleCondition represents a condition in VMRule status
type VMRuleCondition struct {
	Type               string `json:"type"`                         // Type of condition
	Status             string `json:"status"`                       // Status: "True", "False", "Unknown"
	LastTransitionTime string `json:"lastTransitionTime,omitempty"` // Last time condition transitioned
	Reason             string `json:"reason,omitempty"`             // Machine-readable reason
	Message            string `json:"message,omitempty"`            // Human-readable message
}

// ResourceMappingConfig defines configuration for mapping Prometheus labels to Primus-Lens resources
type ResourceMappingConfig struct {
	Enabled       bool              `json:"enabled"`                  // Enable resource mapping
	LabelMappings []LabelMapping    `json:"label_mappings,omitempty"` // Label mapping rules
	DefaultLabels map[string]string `json:"default_labels,omitempty"` // Default labels to add
}

// LabelMapping defines a mapping rule from Prometheus labels to resource information
type LabelMapping struct {
	PrometheusLabel string `json:"prometheus_label"`    // Source Prometheus label name
	ResourceType    string `json:"resource_type"`       // Target resource type: pod/node/workload/gpu
	ResourceField   string `json:"resource_field"`      // Target resource field
	Transform       string `json:"transform,omitempty"` // Optional transformation: lowercase/uppercase/regex
	Pattern         string `json:"pattern,omitempty"`   // Optional regex pattern for extraction
}

// AlertEnrichmentConfig defines configuration for enriching alerts with resource information
type AlertEnrichmentConfig struct {
	Enabled             bool                    `json:"enabled"`                         // Enable alert enrichment
	IncludeNodeInfo     bool                    `json:"include_node_info,omitempty"`     // Include node information
	IncludeGPUInfo      bool                    `json:"include_gpu_info,omitempty"`      // Include GPU information
	IncludePodInfo      bool                    `json:"include_pod_info,omitempty"`      // Include pod information
	IncludeWorkloadInfo bool                    `json:"include_workload_info,omitempty"` // Include workload information
	CustomFields        []CustomEnrichmentField `json:"custom_fields,omitempty"`         // Custom enrichment fields
}

// CustomEnrichmentField defines a custom field to add to alerts
type CustomEnrichmentField struct {
	Name         string `json:"name"`                    // Field name to add
	Source       string `json:"source"`                  // Source: label/annotation/metric/query
	SourceKey    string `json:"source_key,omitempty"`    // Source key (for label/annotation)
	Query        string `json:"query,omitempty"`         // Query (for metric/query source)
	DefaultValue string `json:"default_value,omitempty"` // Default value if source not found
}

// AlertGroupingConfig defines configuration for grouping related alerts
type AlertGroupingConfig struct {
	Enabled        bool     `json:"enabled"`                   // Enable alert grouping
	GroupByLabels  []string `json:"group_by_labels,omitempty"` // Labels to group by
	GroupInterval  string   `json:"group_interval,omitempty"`  // Grouping interval, e.g., "5m"
	GroupWait      string   `json:"group_wait,omitempty"`      // Wait time before sending group, e.g., "30s"
	RepeatInterval string   `json:"repeat_interval,omitempty"` // Repeat interval for grouped alerts
}

// AlertRoutingConfig defines configuration for routing alerts to different receivers
type AlertRoutingConfig struct {
	Enabled         bool            `json:"enabled"`                    // Enable alert routing
	DefaultReceiver string          `json:"default_receiver,omitempty"` // Default receiver name
	Routes          []AlertRoute    `json:"routes,omitempty"`           // Routing rules
	Receivers       []AlertReceiver `json:"receivers,omitempty"`        // Receiver configurations
}

// AlertRoute defines a routing rule for alerts
type AlertRoute struct {
	Match          map[string]string `json:"match,omitempty"`           // Label matchers (exact)
	MatchRegex     map[string]string `json:"match_regex,omitempty"`     // Label matchers (regex)
	Receiver       string            `json:"receiver"`                  // Receiver name
	Continue       bool              `json:"continue,omitempty"`        // Continue to next route
	GroupBy        []string          `json:"group_by,omitempty"`        // Group by labels
	GroupWait      string            `json:"group_wait,omitempty"`      // Group wait duration
	GroupInterval  string            `json:"group_interval,omitempty"`  // Group interval
	RepeatInterval string            `json:"repeat_interval,omitempty"` // Repeat interval
}

// AlertReceiver defines a receiver for alerts
type AlertReceiver struct {
	Name            string           `json:"name"`                       // Receiver name
	WebhookConfigs  []WebhookConfig  `json:"webhook_configs,omitempty"`  // Webhook configurations
	EmailConfigs    []EmailConfig    `json:"email_configs,omitempty"`    // Email configurations
	SlackConfigs    []SlackConfig    `json:"slack_configs,omitempty"`    // Slack configurations
	DingTalkConfigs []DingTalkConfig `json:"dingtalk_configs,omitempty"` // DingTalk configurations
}

// WebhookConfig defines a webhook receiver configuration
type WebhookConfig struct {
	URL          string            `json:"url"`                     // Webhook URL
	SendResolved bool              `json:"send_resolved,omitempty"` // Send resolved alerts
	HTTPConfig   *HTTPClientConfig `json:"http_config,omitempty"`   // HTTP client configuration
}

// EmailConfig defines an email receiver configuration
type EmailConfig struct {
	To           []string          `json:"to"`                      // Recipient email addresses
	From         string            `json:"from,omitempty"`          // Sender email address
	Smarthost    string            `json:"smarthost,omitempty"`     // SMTP server
	AuthUsername string            `json:"auth_username,omitempty"` // SMTP auth username
	AuthPassword string            `json:"auth_password,omitempty"` // SMTP auth password
	RequireTLS   *bool             `json:"require_tls,omitempty"`   // Require TLS
	SendResolved bool              `json:"send_resolved,omitempty"` // Send resolved alerts
	Headers      map[string]string `json:"headers,omitempty"`       // Custom headers
}

// SlackConfig defines a Slack receiver configuration
type SlackConfig struct {
	APIUrl       string            `json:"api_url,omitempty"`       // Slack API URL
	Channel      string            `json:"channel,omitempty"`       // Slack channel
	Username     string            `json:"username,omitempty"`      // Bot username
	Title        string            `json:"title,omitempty"`         // Message title
	Text         string            `json:"text,omitempty"`          // Message text
	SendResolved bool              `json:"send_resolved,omitempty"` // Send resolved alerts
	HTTPConfig   *HTTPClientConfig `json:"http_config,omitempty"`   // HTTP client configuration
}

// DingTalkConfig defines a DingTalk receiver configuration
type DingTalkConfig struct {
	URL          string `json:"url"`                     // DingTalk webhook URL
	Secret       string `json:"secret,omitempty"`        // DingTalk webhook secret
	Message      string `json:"message,omitempty"`       // Message template
	SendResolved bool   `json:"send_resolved,omitempty"` // Send resolved alerts
}

// HTTPClientConfig defines HTTP client configuration for webhook requests
type HTTPClientConfig struct {
	BearerToken     string            `json:"bearer_token,omitempty"`      // Bearer token
	BearerTokenFile string            `json:"bearer_token_file,omitempty"` // Bearer token file path
	ProxyURL        string            `json:"proxy_url,omitempty"`         // Proxy URL
	TLSConfig       *TLSConfig        `json:"tls_config,omitempty"`        // TLS configuration
	BasicAuth       *BasicAuth        `json:"basic_auth,omitempty"`        // Basic authentication
	Headers         map[string]string `json:"headers,omitempty"`           // Custom headers
}

// TLSConfig defines TLS configuration
type TLSConfig struct {
	CAFile             string `json:"ca_file,omitempty"`              // CA certificate file
	CertFile           string `json:"cert_file,omitempty"`            // Client certificate file
	KeyFile            string `json:"key_file,omitempty"`             // Client key file
	ServerName         string `json:"server_name,omitempty"`          // Server name for verification
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty"` // Skip TLS verification
}

// BasicAuth defines basic authentication configuration
type BasicAuth struct {
	Username     string `json:"username,omitempty"`      // Username
	Password     string `json:"password,omitempty"`      // Password
	PasswordFile string `json:"password_file,omitempty"` // Password file path
}
