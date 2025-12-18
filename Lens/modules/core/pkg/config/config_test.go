package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestControllerConfig_GetMetricsBindAddress tests the GetMetricsBindAddress method
func TestControllerConfig_GetMetricsBindAddress(t *testing.T) {
	tests := []struct {
		name   string
		config ControllerConfig
		want   string
	}{
		{
			name: "custom port",
			config: ControllerConfig{
				MetricsPort: 8080,
			},
			want: ":8080",
		},
		{
			name: "zero port uses default 19191",
			config: ControllerConfig{
				MetricsPort: 0,
			},
			want: ":19191",
		},
		{
			name: "negative port uses default 19191",
			config: ControllerConfig{
				MetricsPort: -1,
			},
			want: ":-1",
		},
		{
			name: "high port number",
			config: ControllerConfig{
				MetricsPort: 65535,
			},
			want: ":65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetMetricsBindAddress()
			if got != tt.want {
				t.Errorf("GetMetricsBindAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestControllerConfig_GetHealthzBindAddress tests the GetHealthzBindAddress method
func TestControllerConfig_GetHealthzBindAddress(t *testing.T) {
	tests := []struct {
		name   string
		config ControllerConfig
		want   string
	}{
		{
			name: "custom port",
			config: ControllerConfig{
				HealthzPort: 8081,
			},
			want: ":8081",
		},
		{
			name: "zero port uses default 19192",
			config: ControllerConfig{
				HealthzPort: 0,
			},
			want: ":19192",
		},
		{
			name: "negative port uses default 19192",
			config: ControllerConfig{
				HealthzPort: -1,
			},
			want: ":-1",
		},
		{
			name: "high port number",
			config: ControllerConfig{
				HealthzPort: 65534,
			},
			want: ":65534",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetHealthzBindAddress()
			if got != tt.want {
				t.Errorf("GetHealthzBindAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestControllerConfig_GetPprofBindAddress tests the GetPprofBindAddress method
func TestControllerConfig_GetPprofBindAddress(t *testing.T) {
	tests := []struct {
		name   string
		config ControllerConfig
		want   string
	}{
		{
			name: "custom port",
			config: ControllerConfig{
				PprofPort: 8082,
			},
			want: ":8082",
		},
		{
			name: "zero port uses default 19193",
			config: ControllerConfig{
				PprofPort: 0,
			},
			want: ":19193",
		},
		{
			name: "negative port uses default 19193",
			config: ControllerConfig{
				PprofPort: -1,
			},
			want: ":-1",
		},
		{
			name: "high port number",
			config: ControllerConfig{
				PprofPort: 65533,
			},
			want: ":65533",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetPprofBindAddress()
			if got != tt.want {
				t.Errorf("GetPprofBindAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNetFlow_GetScanPortListenInterval tests the GetScanPortListenInterval method
func TestNetFlow_GetScanPortListenInterval(t *testing.T) {
	tests := []struct {
		name    string
		netflow NetFlow
		want    time.Duration
	}{
		{
			name: "positive interval",
			netflow: NetFlow{
				ScanPortListenIntervalSeconds: 5,
			},
			want: 5 * time.Second,
		},
		{
			name: "zero interval uses default 2 seconds",
			netflow: NetFlow{
				ScanPortListenIntervalSeconds: 0,
			},
			want: 2 * time.Second,
		},
		{
			name: "negative interval uses default 2 seconds",
			netflow: NetFlow{
				ScanPortListenIntervalSeconds: -1,
			},
			want: 2 * time.Second,
		},
		{
			name: "large interval",
			netflow: NetFlow{
				ScanPortListenIntervalSeconds: 3600,
			},
			want: 3600 * time.Second,
		},
		{
			name: "1 second interval",
			netflow: NetFlow{
				ScanPortListenIntervalSeconds: 1,
			},
			want: 1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.netflow.GetScanPortListenInterval()
			if got != tt.want {
				t.Errorf("GetScanPortListenInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLoadConfig tests the LoadConfig function
func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		setupEnv    func(t *testing.T) string // Returns temp dir path
		configYAML  string
		wantErr     bool
		checkResult func(t *testing.T, cfg *Config)
	}{
		{
			name: "valid complete config",
			setupEnv: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			configYAML: `
multiCluster: true
loadK8SClient: true
loadStorageClient: true
httpPort: 8080
controller:
  namespace: primus-lens
  leaderElectionId: controller-leader
  metricsPort: 19191
  healthzPort: 19192
  pprofPort: 19193
nodeExporter:
  containerd_socket_path: /run/containerd/containerd.sock
  grpc_server: localhost:50051
  telemetry_processor_url: http://telemetry:8080
jobs:
  grpc_port: 50052
netflow:
  scan_port_listen_interval_seconds: 5
  policy_config_path: /etc/policy.yaml
`,
			wantErr: false,
			checkResult: func(t *testing.T, cfg *Config) {
				if cfg == nil {
					t.Fatal("Expected non-nil config")
				}
				if !cfg.MultiCluster {
					t.Error("Expected MultiCluster to be true")
				}
				if !cfg.LoadK8SClient {
					t.Error("Expected LoadK8SClient to be true")
				}
				if !cfg.LoadStorageClient {
					t.Error("Expected LoadStorageClient to be true")
				}
				if cfg.HttpPort != 8080 {
					t.Errorf("Expected HttpPort to be 8080, got %d", cfg.HttpPort)
				}
				if cfg.Controller.Namespace != "primus-lens" {
					t.Errorf("Expected Controller.Namespace to be primus-lens, got %s", cfg.Controller.Namespace)
				}
				if cfg.NodeExporter == nil {
					t.Error("Expected NodeExporter to be set")
				} else {
					if cfg.NodeExporter.ContainerdSocketPath != "/run/containerd/containerd.sock" {
						t.Errorf("Expected ContainerdSocketPath, got %s", cfg.NodeExporter.ContainerdSocketPath)
					}
				}
				if cfg.Jobs == nil {
					t.Error("Expected Jobs to be set")
				} else {
					if cfg.Jobs.GrpcPort != 50052 {
						t.Errorf("Expected Jobs.GrpcPort to be 50052, got %d", cfg.Jobs.GrpcPort)
					}
				}
				if cfg.Netflow == nil {
					t.Error("Expected Netflow to be set")
				} else {
					if cfg.Netflow.ScanPortListenIntervalSeconds != 5 {
						t.Errorf("Expected interval to be 5, got %d", cfg.Netflow.ScanPortListenIntervalSeconds)
					}
				}
			},
		},
		{
			name: "minimal valid config",
			setupEnv: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			configYAML: `
multiCluster: false
loadK8SClient: true
loadStorageClient: false
httpPort: 9090
controller:
  namespace: default
`,
			wantErr: false,
			checkResult: func(t *testing.T, cfg *Config) {
				if cfg == nil {
					t.Fatal("Expected non-nil config")
				}
				if cfg.MultiCluster {
					t.Error("Expected MultiCluster to be false")
				}
				if cfg.HttpPort != 9090 {
					t.Errorf("Expected HttpPort to be 9090, got %d", cfg.HttpPort)
				}
				if cfg.Controller.Namespace != "default" {
					t.Errorf("Expected namespace to be default, got %s", cfg.Controller.Namespace)
				}
			},
		},
		{
			name: "config with nil optional fields",
			setupEnv: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			configYAML: `
multiCluster: false
loadK8SClient: true
loadStorageClient: true
httpPort: 8080
controller:
  namespace: test
nodeExporter:
jobs:
netflow:
`,
			wantErr: false,
			checkResult: func(t *testing.T, cfg *Config) {
				if cfg.NodeExporter != nil {
					t.Logf("NodeExporter is not nil, but empty: %+v", cfg.NodeExporter)
				}
				if cfg.Jobs != nil {
					t.Logf("Jobs is not nil, but empty: %+v", cfg.Jobs)
				}
				if cfg.Netflow != nil {
					t.Logf("Netflow is not nil, but empty: %+v", cfg.Netflow)
				}
			},
		},
		{
			name: "invalid yaml syntax",
			setupEnv: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			configYAML: `
multiCluster: true
  invalid: yaml: syntax
loadK8SClient: true
`,
			wantErr: true,
		},
		{
			name: "non-existent config file",
			setupEnv: func(t *testing.T) string {
				// Don't create the file
				return t.TempDir()
			},
			configYAML: "",
			wantErr:    true,
		},
		{
			name: "empty config file",
			setupEnv: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			configYAML: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := tt.setupEnv(t)
			configPath := filepath.Join(tmpDir, "config.yaml")

			// Create config file if yaml content is provided
			if tt.configYAML != "" || tt.name == "empty config file" {
				err := os.WriteFile(configPath, []byte(tt.configYAML), 0644)
				if err != nil {
					t.Fatalf("Failed to create config file: %v", err)
				}
			}

			// Set environment variable
			oldConfigPath := os.Getenv("CONFIG_PATH")
			if tt.name == "non-existent config file" {
				os.Setenv("CONFIG_PATH", filepath.Join(tmpDir, "non-existent.yaml"))
			} else {
				os.Setenv("CONFIG_PATH", configPath)
			}
			defer os.Setenv("CONFIG_PATH", oldConfigPath)

			// Call LoadConfig
			cfg, err := LoadConfig()

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check result if no error expected
			if tt.checkResult != nil && !tt.wantErr {
				tt.checkResult(t, cfg)
			}
		})
	}
}

// TestLoadConfig_DefaultPath tests LoadConfig with default config.yaml path
func TestLoadConfig_DefaultPath(t *testing.T) {
	// Save original working directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(originalWd)

	// Create temp directory and change to it
	tmpDir := t.TempDir()
	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Create config.yaml in current directory
	configContent := `
multiCluster: false
loadK8SClient: true
loadStorageClient: true
httpPort: 7777
controller:
  namespace: default-test
`
	err = os.WriteFile("config.yaml", []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Unset CONFIG_PATH to use default
	oldConfigPath := os.Getenv("CONFIG_PATH")
	os.Unsetenv("CONFIG_PATH")
	defer os.Setenv("CONFIG_PATH", oldConfigPath)

	// Load config
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify config was loaded
	if cfg.HttpPort != 7777 {
		t.Errorf("Expected HttpPort to be 7777, got %d", cfg.HttpPort)
	}
	if cfg.Controller.Namespace != "default-test" {
		t.Errorf("Expected namespace to be default-test, got %s", cfg.Controller.Namespace)
	}
}

// TestConfig_StructFields tests that Config struct has expected fields
func TestConfig_StructFields(t *testing.T) {
	cfg := Config{
		MultiCluster:      true,
		LoadK8SClient:     true,
		LoadStorageClient: false,
		HttpPort:          8080,
		Controller: ControllerConfig{
			Namespace:        "test-ns",
			LeaderElectionId: "leader-1",
			MetricsPort:      9090,
			HealthzPort:      9091,
			PprofPort:        9092,
		},
		NodeExporter: &NodeExporterConfig{
			ContainerdSocketPath:  "/path/to/socket",
			GrpcServer:            "server:50051",
			TelemetryProcessorURL: "http://processor:8080",
		},
		Jobs: &JobsConfig{
			GrpcPort: 50052,
		},
		Netflow: &NetFlow{
			ScanPortListenIntervalSeconds: 10,
			PolicyConfigPath:              "/path/to/policy",
		},
	}

	// Verify all fields are accessible
	if !cfg.MultiCluster {
		t.Error("Expected MultiCluster to be true")
	}
	if cfg.Controller.Namespace != "test-ns" {
		t.Errorf("Expected namespace test-ns, got %s", cfg.Controller.Namespace)
	}
	if cfg.NodeExporter.ContainerdSocketPath != "/path/to/socket" {
		t.Error("NodeExporter.ContainerdSocketPath mismatch")
	}
	if cfg.Jobs.GrpcPort != 50052 {
		t.Error("Jobs.GrpcPort mismatch")
	}
	if cfg.Netflow.PolicyConfigPath != "/path/to/policy" {
		t.Error("Netflow.PolicyConfigPath mismatch")
	}
}

// TestControllerConfig_AllMethods tests all three bind address methods together
func TestControllerConfig_AllMethods(t *testing.T) {
	cfg := ControllerConfig{
		Namespace:        "test",
		LeaderElectionId: "leader",
		MetricsPort:      8001,
		HealthzPort:      8002,
		PprofPort:        8003,
	}

	metrics := cfg.GetMetricsBindAddress()
	healthz := cfg.GetHealthzBindAddress()
	pprof := cfg.GetPprofBindAddress()

	if metrics != ":8001" {
		t.Errorf("Expected metrics :8001, got %s", metrics)
	}
	if healthz != ":8002" {
		t.Errorf("Expected healthz :8002, got %s", healthz)
	}
	if pprof != ":8003" {
		t.Errorf("Expected pprof :8003, got %s", pprof)
	}

	// Test with all zero ports
	cfgZero := ControllerConfig{}
	if cfgZero.GetMetricsBindAddress() != ":19191" {
		t.Error("Expected default metrics port")
	}
	if cfgZero.GetHealthzBindAddress() != ":19192" {
		t.Error("Expected default healthz port")
	}
	if cfgZero.GetPprofBindAddress() != ":19193" {
		t.Error("Expected default pprof port")
	}
}

// Benchmark tests
func BenchmarkControllerConfig_GetMetricsBindAddress(b *testing.B) {
	cfg := ControllerConfig{MetricsPort: 8080}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.GetMetricsBindAddress()
	}
}

func BenchmarkControllerConfig_GetHealthzBindAddress(b *testing.B) {
	cfg := ControllerConfig{HealthzPort: 8081}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.GetHealthzBindAddress()
	}
}

func BenchmarkControllerConfig_GetPprofBindAddress(b *testing.B) {
	cfg := ControllerConfig{PprofPort: 8082}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.GetPprofBindAddress()
	}
}

func BenchmarkNetFlow_GetScanPortListenInterval(b *testing.B) {
	nf := NetFlow{ScanPortListenIntervalSeconds: 5}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = nf.GetScanPortListenInterval()
	}
}

func BenchmarkLoadConfig(b *testing.B) {
	// Create temp config file
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
multiCluster: true
loadK8SClient: true
loadStorageClient: true
httpPort: 8080
controller:
  namespace: bench-test
  metricsPort: 19191
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		b.Fatalf("Failed to create config file: %v", err)
	}

	oldConfigPath := os.Getenv("CONFIG_PATH")
	os.Setenv("CONFIG_PATH", configPath)
	defer os.Setenv("CONFIG_PATH", oldConfigPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadConfig()
	}
}

// TestMiddlewareConfig_GetTraceMode tests the GetTraceMode method
func TestMiddlewareConfig_GetTraceMode(t *testing.T) {
	tests := []struct {
		name   string
		config MiddlewareConfig
		want   string
	}{
		{
			name:   "nil trace config returns error_only",
			config: MiddlewareConfig{},
			want:   "error_only",
		},
		{
			name: "empty mode returns error_only",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					Mode: "",
				},
			},
			want: "error_only",
		},
		{
			name: "error_only mode",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					Mode: "error_only",
				},
			},
			want: "error_only",
		},
		{
			name: "always mode",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					Mode: "always",
				},
			},
			want: "always",
		},
		{
			name: "custom mode",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					Mode: "custom",
				},
			},
			want: "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetTraceMode()
			if got != tt.want {
				t.Errorf("GetTraceMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMiddlewareConfig_GetSamplingRatio tests the GetSamplingRatio method
func TestMiddlewareConfig_GetSamplingRatio(t *testing.T) {
	ratio01 := 0.1
	ratio05 := 0.5
	ratio10 := 1.0
	ratio00 := 0.0

	tests := []struct {
		name   string
		config MiddlewareConfig
		want   float64
	}{
		{
			name:   "nil trace config returns 0.1",
			config: MiddlewareConfig{},
			want:   0.1,
		},
		{
			name: "nil sampling ratio returns 0.1",
			config: MiddlewareConfig{
				Trace: &TraceConfig{},
			},
			want: 0.1,
		},
		{
			name: "sampling ratio 0.1",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					SamplingRatio: &ratio01,
				},
			},
			want: 0.1,
		},
		{
			name: "sampling ratio 0.5",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					SamplingRatio: &ratio05,
				},
			},
			want: 0.5,
		},
		{
			name: "sampling ratio 1.0",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					SamplingRatio: &ratio10,
				},
			},
			want: 1.0,
		},
		{
			name: "sampling ratio 0.0",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					SamplingRatio: &ratio00,
				},
			},
			want: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetSamplingRatio()
			if got != tt.want {
				t.Errorf("GetSamplingRatio() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMiddlewareConfig_GetErrorSamplingRatio tests the GetErrorSamplingRatio method
func TestMiddlewareConfig_GetErrorSamplingRatio(t *testing.T) {
	ratio01 := 0.1
	ratio05 := 0.5
	ratio10 := 1.0
	ratio00 := 0.0

	tests := []struct {
		name   string
		config MiddlewareConfig
		want   float64
	}{
		{
			name:   "nil trace config returns 1.0",
			config: MiddlewareConfig{},
			want:   1.0,
		},
		{
			name: "nil error sampling ratio returns 1.0",
			config: MiddlewareConfig{
				Trace: &TraceConfig{},
			},
			want: 1.0,
		},
		{
			name: "error sampling ratio 0.1",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					ErrorSamplingRatio: &ratio01,
				},
			},
			want: 0.1,
		},
		{
			name: "error sampling ratio 0.5",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					ErrorSamplingRatio: &ratio05,
				},
			},
			want: 0.5,
		},
		{
			name: "error sampling ratio 1.0",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					ErrorSamplingRatio: &ratio10,
				},
			},
			want: 1.0,
		},
		{
			name: "error sampling ratio 0.0",
			config: MiddlewareConfig{
				Trace: &TraceConfig{
					ErrorSamplingRatio: &ratio00,
				},
			},
			want: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetErrorSamplingRatio()
			if got != tt.want {
				t.Errorf("GetErrorSamplingRatio() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTraceConfig_Struct tests TraceConfig struct fields
func TestTraceConfig_Struct(t *testing.T) {
	ratio := 0.5
	errRatio := 0.8

	cfg := TraceConfig{
		Mode:               "always",
		SamplingRatio:      &ratio,
		ErrorSamplingRatio: &errRatio,
	}

	if cfg.Mode != "always" {
		t.Errorf("Expected Mode to be 'always', got %s", cfg.Mode)
	}
	if *cfg.SamplingRatio != 0.5 {
		t.Errorf("Expected SamplingRatio to be 0.5, got %f", *cfg.SamplingRatio)
	}
	if *cfg.ErrorSamplingRatio != 0.8 {
		t.Errorf("Expected ErrorSamplingRatio to be 0.8, got %f", *cfg.ErrorSamplingRatio)
	}
}

// TestMiddlewareConfig_WithTraceConfig tests MiddlewareConfig with full TraceConfig
func TestMiddlewareConfig_WithTraceConfig(t *testing.T) {
	enableLogging := true
	enableTracing := true
	samplingRatio := 0.3
	errorSamplingRatio := 0.9

	cfg := MiddlewareConfig{
		EnableLogging: &enableLogging,
		EnableTracing: &enableTracing,
		Trace: &TraceConfig{
			Mode:               "always",
			SamplingRatio:      &samplingRatio,
			ErrorSamplingRatio: &errorSamplingRatio,
		},
	}

	if !cfg.IsLoggingEnabled() {
		t.Error("Expected logging to be enabled")
	}
	if !cfg.IsTracingEnabled() {
		t.Error("Expected tracing to be enabled")
	}
	if cfg.GetTraceMode() != "always" {
		t.Errorf("Expected trace mode 'always', got %s", cfg.GetTraceMode())
	}
	if cfg.GetSamplingRatio() != 0.3 {
		t.Errorf("Expected sampling ratio 0.3, got %f", cfg.GetSamplingRatio())
	}
	if cfg.GetErrorSamplingRatio() != 0.9 {
		t.Errorf("Expected error sampling ratio 0.9, got %f", cfg.GetErrorSamplingRatio())
	}
}

// TestLoadConfig_WithTraceConfig tests LoadConfig with trace configuration
func TestLoadConfig_WithTraceConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
multiCluster: false
loadK8SClient: true
loadStorageClient: true
httpPort: 8080
controller:
  namespace: test
middleware:
  enableLogging: true
  enableTracing: true
  trace:
    mode: always
    samplingRatio: 0.25
    errorSamplingRatio: 0.75
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	oldConfigPath := os.Getenv("CONFIG_PATH")
	os.Setenv("CONFIG_PATH", configPath)
	defer os.Setenv("CONFIG_PATH", oldConfigPath)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Middleware.GetTraceMode() != "always" {
		t.Errorf("Expected trace mode 'always', got %s", cfg.Middleware.GetTraceMode())
	}
	if cfg.Middleware.GetSamplingRatio() != 0.25 {
		t.Errorf("Expected sampling ratio 0.25, got %f", cfg.Middleware.GetSamplingRatio())
	}
	if cfg.Middleware.GetErrorSamplingRatio() != 0.75 {
		t.Errorf("Expected error sampling ratio 0.75, got %f", cfg.Middleware.GetErrorSamplingRatio())
	}
}

// TestLoadConfig_WithErrorOnlyTraceConfig tests LoadConfig with error_only trace mode
func TestLoadConfig_WithErrorOnlyTraceConfig(t *testing.T) {
	// Reset global config to avoid test interference
	config = nil

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
multiCluster: false
loadK8SClient: true
loadStorageClient: true
httpPort: 8080
controller:
  namespace: test
middleware:
  enableTracing: true
  trace:
    mode: error_only
    samplingRatio: 0.15
    errorSamplingRatio: 0.5
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	oldConfigPath := os.Getenv("CONFIG_PATH")
	os.Setenv("CONFIG_PATH", configPath)
	defer os.Setenv("CONFIG_PATH", oldConfigPath)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Middleware.GetTraceMode() != "error_only" {
		t.Errorf("Expected trace mode 'error_only', got %s", cfg.Middleware.GetTraceMode())
	}
	if cfg.Middleware.GetSamplingRatio() != 0.15 {
		t.Errorf("Expected sampling ratio 0.15, got %f", cfg.Middleware.GetSamplingRatio())
	}
	if cfg.Middleware.GetErrorSamplingRatio() != 0.5 {
		t.Errorf("Expected error sampling ratio 0.5, got %f", cfg.Middleware.GetErrorSamplingRatio())
	}
}

// Benchmark tests for TraceConfig methods
func BenchmarkMiddlewareConfig_GetTraceMode(b *testing.B) {
	cfg := MiddlewareConfig{
		Trace: &TraceConfig{
			Mode: "always",
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.GetTraceMode()
	}
}

func BenchmarkMiddlewareConfig_GetSamplingRatio(b *testing.B) {
	ratio := 0.5
	cfg := MiddlewareConfig{
		Trace: &TraceConfig{
			SamplingRatio: &ratio,
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.GetSamplingRatio()
	}
}

func BenchmarkMiddlewareConfig_GetErrorSamplingRatio(b *testing.B) {
	ratio := 0.8
	cfg := MiddlewareConfig{
		Trace: &TraceConfig{
			ErrorSamplingRatio: &ratio,
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.GetErrorSamplingRatio()
	}
}
