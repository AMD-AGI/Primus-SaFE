// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config

import (
	"fmt"
	"os"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/snapshot"
	"gopkg.in/yaml.v2"
)

type Config struct {
	MultiCluster      bool                `json:"multiCluster" yaml:"multiCluster"`
	LoadK8SClient     bool                `json:"loadK8SClient" yaml:"loadK8SClient"`
	LoadStorageClient bool                `json:"loadStorageClient" yaml:"loadStorageClient"`
	IsControlPlane    bool                `json:"isControlPlane" yaml:"isControlPlane"`
	ControlPlane      *ControlPlaneConfig `json:"controlPlane" yaml:"controlPlane"`
	Controller        ControllerConfig    `yaml:"controller"`
	HttpPort          int                 `json:"httpPort" yaml:"httpPort"`
	NodeExporter      *NodeExporterConfig `json:"nodeExporter" yaml:"nodeExporter"`
	Jobs              *JobsConfig         `json:"jobs" yaml:"jobs"`
	Netflow           *NetFlow            `json:"netflow" yaml:"netflow"`
	Middleware        MiddlewareConfig    `json:"middleware" yaml:"middleware"`
	AIGateway         *AIGatewayConfig    `json:"aiGateway" yaml:"aiGateway"`
	MCP               *MCPConfig          `json:"mcp" yaml:"mcp"`
	SnapshotStore     *SnapshotStoreConfig `json:"snapshotStore" yaml:"snapshotStore"`
}

// MCPConfig contains MCP (Model Context Protocol) server configuration
type MCPConfig struct {
	// Enabled controls whether MCP server is started
	Enabled bool `json:"enabled" yaml:"enabled"`
	// BasePath is the MCP server base path (default: /mcp)
	BasePath string `json:"basePath" yaml:"basePath"`
	// Instructions are server instructions sent to MCP clients
	Instructions string `json:"instructions" yaml:"instructions"`
}

// IsMCPEnabled returns whether MCP server is enabled
func (c *Config) IsMCPEnabled() bool {
	return c.MCP != nil && c.MCP.Enabled
}

// GetMCPBasePath returns the MCP server base path, defaults to "/mcp"
func (c *Config) GetMCPBasePath() string {
	if c.MCP == nil || c.MCP.BasePath == "" {
		return "/mcp"
	}
	return c.MCP.BasePath
}

// GetMCPInstructions returns the MCP server instructions
func (c *Config) GetMCPInstructions() string {
	if c.MCP == nil {
		return ""
	}
	return c.MCP.Instructions
}

type ControllerConfig struct {
	Namespace        string `json:"namespace" yaml:"namespace"`
	LeaderElectionId string `json:"leader_election_id" yaml:"leaderElectionId"`
	MetricsPort      int    `json:"metricsPort" yaml:"metricsPort"`
	HealthzPort      int    `json:"healthzPort" yaml:"healthzPort"`
	PprofPort        int    `json:"pprofPort" yaml:"pprofPort"`
}

func (cfg ControllerConfig) GetMetricsBindAddress() string {
	port := cfg.MetricsPort
	if port == 0 {
		port = 19191
	}
	return fmt.Sprintf(":%d", port)
}

func (cfg ControllerConfig) GetHealthzBindAddress() string {
	port := cfg.HealthzPort
	if port == 0 {
		port = 19192
	}
	return fmt.Sprintf(":%d", port)
}

func (cfg ControllerConfig) GetPprofBindAddress() string {
	port := cfg.PprofPort
	if port == 0 {
		port = 19193
	}
	return fmt.Sprintf(":%d", port)
}

var config *Config

func LoadConfig() (*Config, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}
	configFile, err := os.Open(configPath)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("failed to open config file").
			WithError(err)
	}
	defer configFile.Close()
	decoder := yaml.NewDecoder(configFile)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("failed to parse config file").
			WithError(err)
	}
	return config, nil
}

type NodeExporterConfig struct {
	ContainerdSocketPath  string        `yaml:"containerd_socket_path" json:"containerd_socket_path"`
	GrpcServer            string        `yaml:"grpc_server" json:"grpc_server"` // Deprecated: use TelemetryProcessorURL
	TelemetryProcessorURL string        `yaml:"telemetry_processor_url" json:"telemetry_processor_url"`
	PySpy                 *PySpyConfig  `yaml:"pyspy" json:"pyspy"`
}

// PySpyConfig contains py-spy profiler configuration
type PySpyConfig struct {
	Enabled           bool   `yaml:"enabled" json:"enabled"`
	StoragePath       string `yaml:"storage_path" json:"storage_path"`
	BinaryPath        string `yaml:"binary_path" json:"binary_path"`
	MaxStorageSizeMB  int64  `yaml:"max_storage_size_mb" json:"max_storage_size_mb"`
	FileRetentionDays int    `yaml:"file_retention_days" json:"file_retention_days"`
	MaxConcurrentJobs int    `yaml:"max_concurrent_jobs" json:"max_concurrent_jobs"`
	DefaultDuration   int    `yaml:"default_duration" json:"default_duration"`
	DefaultRate       int    `yaml:"default_rate" json:"default_rate"`
}

// GetPySpyConfig returns PySpyConfig with defaults
func (c *NodeExporterConfig) GetPySpyConfig() *PySpyConfig {
	if c == nil || c.PySpy == nil {
		return &PySpyConfig{
			Enabled:           true,
			StoragePath:       "/var/lib/lens/pyspy",
			BinaryPath:        "/usr/local/bin/py-spy",
			MaxStorageSizeMB:  10240,
			FileRetentionDays: 7,
			MaxConcurrentJobs: 5,
			DefaultDuration:   30,
			DefaultRate:       100,
		}
	}
	cfg := c.PySpy
	if cfg.StoragePath == "" {
		cfg.StoragePath = "/var/lib/lens/pyspy"
	}
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = "/usr/local/bin/py-spy"
	}
	if cfg.MaxStorageSizeMB == 0 {
		cfg.MaxStorageSizeMB = 10240
	}
	if cfg.FileRetentionDays == 0 {
		cfg.FileRetentionDays = 7
	}
	if cfg.MaxConcurrentJobs == 0 {
		cfg.MaxConcurrentJobs = 5
	}
	if cfg.DefaultDuration == 0 {
		cfg.DefaultDuration = 30
	}
	if cfg.DefaultRate == 0 {
		cfg.DefaultRate = 100
	}
	return cfg
}

type JobsConfig struct {
	GrpcPort             int                         `yaml:"grpc_port" json:"grpc_port"`
	Mode                 string                      `yaml:"mode" json:"mode"` // data, management, or standalone
	WeeklyReport         *WeeklyReportConfig         `yaml:"weekly_report" json:"weekly_report"`
	WeeklyReportBackfill *WeeklyReportBackfillConfig `yaml:"weekly_report_backfill" json:"weekly_report_backfill"`
	AIAgent              *AIAgentConfig              `yaml:"ai_agent" json:"ai_agent"`
}

// AIAgentConfig contains configuration for AI agent used by jobs
type AIAgentConfig struct {
	Name     string        `yaml:"name" json:"name"`         // Agent name, default: "lens-agent-api"
	Endpoint string        `yaml:"endpoint" json:"endpoint"` // Agent endpoint URL, e.g., "http://lens-agent-api:8000"
	Timeout  time.Duration `yaml:"timeout" json:"timeout"`   // Request timeout, default: 120s
	Retry    int           `yaml:"retry" json:"retry"`       // Retry count, default: 2
}

// WeeklyReportBackfillConfig contains configuration for GPU usage weekly report backfill job
type WeeklyReportBackfillConfig struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	Cron               string `yaml:"cron" json:"cron"`                               // Default: "0 3 * * *" (daily at 3:00 AM)
	MaxWeeksToBackfill int    `yaml:"max_weeks_to_backfill" json:"max_weeks_to_backfill"` // 0 = no limit
}

// WeeklyReportConfig contains configuration for GPU usage weekly reports
type WeeklyReportConfig struct {
	Enabled              bool                `yaml:"enabled" json:"enabled"`
	Cron                 string              `yaml:"cron" json:"cron"`
	Timezone             string              `yaml:"timezone" json:"timezone"`
	TimeRangeDays        int                 `yaml:"time_range_days" json:"time_range_days"`
	UtilizationThreshold int                 `yaml:"utilization_threshold" json:"utilization_threshold"`
	MinGpuCount          int                 `yaml:"min_gpu_count" json:"min_gpu_count"`
	TopN                 int                 `yaml:"top_n" json:"top_n"`
	Conductor            ConductorConfig     `yaml:"conductor" json:"conductor"`
	Brand                BrandConfig         `yaml:"brand" json:"brand"`
	OutputFormats        []string            `yaml:"output_formats" json:"output_formats"`
	Email                EmailConfig         `yaml:"email" json:"email"`
	Storage              ReportStorageConfig `yaml:"storage" json:"storage"`
}

// ConductorConfig contains Conductor API configuration
type ConductorConfig struct {
	BaseURL string        `yaml:"base_url" json:"base_url"`
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
	Retry   int           `yaml:"retry" json:"retry"`
}

// BrandConfig contains branding configuration for reports
type BrandConfig struct {
	LogoURL      string `yaml:"logo_url" json:"logo_url"`
	CompanyName  string `yaml:"company_name" json:"company_name"`
	PrimaryColor string `yaml:"primary_color" json:"primary_color"`
}

// EmailConfig contains email configuration for report distribution
type EmailConfig struct {
	Enabled         bool             `yaml:"enabled" json:"enabled"`
	SMTP            SMTPConfig       `yaml:"smtp" json:"smtp"`
	Recipients      RecipientsConfig `yaml:"recipients" json:"recipients"`
	SubjectTemplate string           `yaml:"subject_template" json:"subject_template"`
	AttachPDF       bool             `yaml:"attach_pdf" json:"attach_pdf"`
}

// SMTPConfig contains SMTP server configuration
type SMTPConfig struct {
	Host        string `yaml:"host" json:"host"`
	Port        int    `yaml:"port" json:"port"`
	Username    string `yaml:"username" json:"username"`
	PasswordEnv string `yaml:"password_env" json:"password_env"`
}

// RecipientsConfig contains email recipients
type RecipientsConfig struct {
	To []string `yaml:"to" json:"to"`
	CC []string `yaml:"cc" json:"cc"`
}

// ReportStorageConfig contains report storage configuration
type ReportStorageConfig struct {
	RetentionDays int `yaml:"retention_days" json:"retention_days"`
}

// AIGatewayConfig contains AI Gateway configuration
type AIGatewayConfig struct {
	// RegistryMode controls the agent registry backend: memory, db, or config
	RegistryMode string `json:"registryMode" yaml:"registryMode"`
	// HealthCheckInterval is the interval for agent health checks
	HealthCheckInterval int `json:"healthCheckInterval" yaml:"healthCheckInterval"`
	// UnhealthyThreshold is the number of failed health checks before marking agent unhealthy
	UnhealthyThreshold int `json:"unhealthyThreshold" yaml:"unhealthyThreshold"`
}

// ControlPlaneConfig contains control plane configuration
type ControlPlaneConfig struct {
	// SecretName is the name of the secret containing control plane DB config
	// Default: "lens-controlplane-db" or env CONTROLPLANE_DB_SECRET_NAME
	SecretName string `json:"secretName" yaml:"secretName"`
	// SecretNamespace is the namespace of the secret
	// Default: current namespace or env CONTROLPLANE_DB_SECRET_NAMESPACE
	SecretNamespace string `json:"secretNamespace" yaml:"secretNamespace"`
	// PrimusSafeSync configuration for syncing clusters from primus-safe
	PrimusSafeSync *PrimusSafeSyncConfig `json:"primusSafeSync" yaml:"primusSafeSync"`
}

// GetSecretName returns the secret name for control plane DB
func (c *ControlPlaneConfig) GetSecretName() string {
	if c != nil && c.SecretName != "" {
		return c.SecretName
	}
	if envName := os.Getenv("CONTROLPLANE_DB_SECRET_NAME"); envName != "" {
		return envName
	}
	// Default: PostgresCluster operator auto-created secret
	return "primus-lens-control-plane-pguser-primus-lens-control-plane"
}

// GetSecretNamespace returns the secret namespace for control plane DB
func (c *ControlPlaneConfig) GetSecretNamespace() string {
	if c != nil && c.SecretNamespace != "" {
		return c.SecretNamespace
	}
	if envNs := os.Getenv("CONTROLPLANE_DB_SECRET_NAMESPACE"); envNs != "" {
		return envNs
	}
	// Default to current namespace from downward API
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	return "primus-lens"
}

// PrimusSafeSyncConfig contains configuration for syncing clusters from primus-safe
type PrimusSafeSyncConfig struct {
	// Enabled controls whether primus-safe sync is enabled
	Enabled bool `json:"enabled" yaml:"enabled"`
	// SyncInterval is the interval for syncing clusters (default: 60s)
	SyncIntervalSeconds int `json:"syncIntervalSeconds" yaml:"syncIntervalSeconds"`
	// AutoInstall controls whether to auto-install dataplane for new clusters
	AutoInstall bool `json:"autoInstall" yaml:"autoInstall"`
	// DefaultProfile is the default install profile (minimal, standard, full)
	DefaultProfile string `json:"defaultProfile" yaml:"defaultProfile"`
}

// GetSyncInterval returns the sync interval duration
func (c *PrimusSafeSyncConfig) GetSyncInterval() time.Duration {
	if c.SyncIntervalSeconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(c.SyncIntervalSeconds) * time.Second
}

// GetDefaultProfile returns the default install profile
func (c *PrimusSafeSyncConfig) GetDefaultProfile() string {
	if c.DefaultProfile == "" {
		return "minimal"
	}
	return c.DefaultProfile
}

type NetFlow struct {
	ScanPortListenIntervalSeconds int    `json:"scan_port_listen_interval_seconds" yaml:"scan_port_listen_interval_seconds"`
	PolicyConfigPath              string `json:"policy_config_path" yaml:"policy_config_path"`
}

func (n NetFlow) GetScanPortListenInterval() time.Duration {
	if n.ScanPortListenIntervalSeconds <= 0 {
		return 2 * time.Second
	}
	return time.Duration(n.ScanPortListenIntervalSeconds) * time.Second
}

// MiddlewareConfig middleware configuration
type MiddlewareConfig struct {
	EnableLogging *bool        `json:"enableLogging" yaml:"enableLogging"` // Whether to enable request logging middleware
	EnableTracing *bool        `json:"enableTracing" yaml:"enableTracing"` // Whether to enable distributed tracing middleware
	Trace         *TraceConfig `json:"trace" yaml:"trace"`                 // Trace configuration
	Auth          *AuthConfig  `json:"auth" yaml:"auth"`                   // Auth configuration
}

// AuthConfig contains authentication configuration
type AuthConfig struct {
	// Enabled controls whether authentication middleware is enabled
	Enabled bool `json:"enabled" yaml:"enabled"`
	// SafeAPIURL is the SaFE API server URL (e.g., "http://primus-safe-apiserver:8080")
	SafeAPIURL string `json:"safeApiUrl" yaml:"safeApiUrl"`
	// InternalToken is the X-Internal-Token for calling SaFE verify endpoint
	InternalToken string `json:"internalToken" yaml:"internalToken"`
	// InternalTokenEnv is the environment variable name for internal token (alternative to InternalToken)
	InternalTokenEnv string `json:"internalTokenEnv" yaml:"internalTokenEnv"`
	// Timeout is the HTTP request timeout for auth verification (default: 5s)
	Timeout int `json:"timeout" yaml:"timeout"`
	// ExcludePaths are paths that skip authentication (e.g., health check endpoints)
	ExcludePaths []string `json:"excludePaths" yaml:"excludePaths"`
}

// TraceConfig contains trace-specific configuration
type TraceConfig struct {
	// Mode controls when traces are exported:
	// - "error_only": Only export traces when an error occurs (default)
	// - "always": Always export traces (subject to sampling ratio)
	Mode string `json:"mode" yaml:"mode"`

	// SamplingRatio controls the sampling ratio when mode is "always" (0.0 to 1.0)
	// Default: 0.1 (10%)
	SamplingRatio *float64 `json:"samplingRatio" yaml:"samplingRatio"`

	// ErrorSamplingRatio controls the sampling ratio for error traces in "error_only" mode (0.0 to 1.0)
	// Default: 1.0 (100% of errors are sampled)
	ErrorSamplingRatio *float64 `json:"errorSamplingRatio" yaml:"errorSamplingRatio"`
}

// GetTraceMode returns the trace mode, default is "error_only"
func (m MiddlewareConfig) GetTraceMode() string {
	if m.Trace == nil || m.Trace.Mode == "" {
		return "error_only"
	}
	return m.Trace.Mode
}

// GetSamplingRatio returns the sampling ratio for "always" mode, default is 0.1
func (m MiddlewareConfig) GetSamplingRatio() float64 {
	if m.Trace == nil || m.Trace.SamplingRatio == nil {
		return 0.1
	}
	return *m.Trace.SamplingRatio
}

// GetErrorSamplingRatio returns the error sampling ratio for "error_only" mode, default is 1.0
func (m MiddlewareConfig) GetErrorSamplingRatio() float64 {
	if m.Trace == nil || m.Trace.ErrorSamplingRatio == nil {
		return 1.0
	}
	return *m.Trace.ErrorSamplingRatio
}

// IsLoggingEnabled returns whether logging middleware is enabled, default enabled
func (m MiddlewareConfig) IsLoggingEnabled() bool {
	// If not explicitly set in config file (nil), return true by default (backward compatible)
	if m.EnableLogging == nil {
		return true
	}
	return *m.EnableLogging
}

// IsTracingEnabled returns whether tracing middleware is enabled, default enabled
func (m MiddlewareConfig) IsTracingEnabled() bool {
	// If not explicitly set in config file (nil), return true by default (backward compatible)
	if m.EnableTracing == nil {
		return true
	}
	return *m.EnableTracing
}

// IsAuthEnabled returns whether auth middleware is enabled, default disabled
func (m MiddlewareConfig) IsAuthEnabled() bool {
	if m.Auth == nil {
		return false
	}
	return m.Auth.Enabled
}

// GetAuthConfig returns the auth configuration
func (m MiddlewareConfig) GetAuthConfig() *AuthConfig {
	return m.Auth
}

// GetInternalToken returns the internal token from config or environment variable
func (a *AuthConfig) GetInternalToken() string {
	if a.InternalToken != "" {
		return a.InternalToken
	}
	if a.InternalTokenEnv != "" {
		return os.Getenv(a.InternalTokenEnv)
	}
	return ""
}

// GetTimeout returns the timeout duration, default 5 seconds
func (a *AuthConfig) GetTimeout() time.Duration {
	if a.Timeout <= 0 {
		return 5 * time.Second
	}
	return time.Duration(a.Timeout) * time.Second
}

// IsPathExcluded checks if the given path should skip authentication
func (a *AuthConfig) IsPathExcluded(path string) bool {
	for _, excludePath := range a.ExcludePaths {
		if path == excludePath || (len(excludePath) > 0 && excludePath[len(excludePath)-1] == '*' && len(path) >= len(excludePath)-1 && path[:len(excludePath)-1] == excludePath[:len(excludePath)-1]) {
			return true
		}
	}
	return false
}

// SnapshotStoreConfig configures external storage for code snapshots.
// When Enabled is false (default), snapshots are stored inline in the database.
type SnapshotStoreConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Type    string `json:"type" yaml:"type"` // "s3" or "local"

	// S3 backend settings
	S3Endpoint   string `json:"s3Endpoint" yaml:"s3Endpoint"`
	S3Bucket     string `json:"s3Bucket" yaml:"s3Bucket"`
	S3AccessKey  string `json:"s3AccessKey" yaml:"s3AccessKey"`
	S3SecretKey  string `json:"s3SecretKey" yaml:"s3SecretKey"`
	S3Secure     bool   `json:"s3Secure" yaml:"s3Secure"`
	S3PathPrefix string `json:"s3PathPrefix" yaml:"s3PathPrefix"`

	// Environment variable names for S3 credentials (override file-based values)
	S3AccessKeyEnv string `json:"s3AccessKeyEnv" yaml:"s3AccessKeyEnv"`
	S3SecretKeyEnv string `json:"s3SecretKeyEnv" yaml:"s3SecretKeyEnv"`

	// Secret file path (Kubernetes secret mount) for S3 credentials
	S3SecretPath string `json:"s3SecretPath" yaml:"s3SecretPath"`

	// Local backend settings
	LocalRootDir string `json:"localRootDir" yaml:"localRootDir"`
}

// ToSnapshotConfig converts SnapshotStoreConfig to snapshot.Config.
// It resolves credentials from env vars / secret files when specified.
func (c *SnapshotStoreConfig) ToSnapshotConfig() snapshot.Config {
	if c == nil || !c.Enabled {
		return snapshot.Config{}
	}

	cfg := snapshot.Config{
		Type: snapshot.StoreType(c.Type),
	}

	switch cfg.Type {
	case snapshot.StoreTypeS3:
		accessKey := c.S3AccessKey
		secretKey := c.S3SecretKey
		// Override from env vars
		if c.S3AccessKeyEnv != "" {
			if v := os.Getenv(c.S3AccessKeyEnv); v != "" {
				accessKey = v
			}
		}
		if c.S3SecretKeyEnv != "" {
			if v := os.Getenv(c.S3SecretKeyEnv); v != "" {
				secretKey = v
			}
		}
		// Override from secret file (Kubernetes secret mount)
		if c.S3SecretPath != "" {
			if data, err := os.ReadFile(c.S3SecretPath + "/access_key"); err == nil {
				accessKey = string(data)
			}
			if data, err := os.ReadFile(c.S3SecretPath + "/secret_key"); err == nil {
				secretKey = string(data)
			}
			// Also try endpoint/bucket from secret
			if data, err := os.ReadFile(c.S3SecretPath + "/endpoint"); err == nil && c.S3Endpoint == "" {
				c.S3Endpoint = string(data)
			}
			if data, err := os.ReadFile(c.S3SecretPath + "/bucket"); err == nil && c.S3Bucket == "" {
				c.S3Bucket = string(data)
			}
		}
		cfg.S3 = snapshot.S3Config{
			Endpoint:   c.S3Endpoint,
			Bucket:     c.S3Bucket,
			AccessKey:  accessKey,
			SecretKey:  secretKey,
			Secure:     c.S3Secure,
			PathPrefix: c.S3PathPrefix,
		}
	case snapshot.StoreTypeLocal:
		rootDir := c.LocalRootDir
		if rootDir == "" {
			rootDir = "/var/lib/lens/code-snapshots"
		}
		cfg.Local = snapshot.LocalConfig{
			RootDir: rootDir,
		}
	}

	return cfg
}
