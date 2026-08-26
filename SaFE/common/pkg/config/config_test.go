/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/viper"
	testifyassert "github.com/stretchr/testify/assert"

	"gotest.tools/assert"
)

func load() error {
	path := "./test.yaml"
	if err := LoadConfig(path); err != nil {
		return err
	}
	return nil
}

func TestConfig(t *testing.T) {
	err := load()
	assert.NilError(t, err)

	assert.Equal(t, getInt("server.port", 0), 8080)
	assert.Equal(t, getString("server.timeout", ""), "30s")
	assert.Equal(t, getBool("server.enable", false), true)
	assert.Equal(t, getFloat("server.ratio", 0), 0.01)

	assert.Equal(t, getString("database.host", ""), "localhost")
	assert.Equal(t, getInt("database.port", 8081), 8081)
	assert.Equal(t, getInt("database.request_timeout_second", 0), 20)
	assert.Equal(t, slices.Equal(getStrings("database.users"), []string{"user1", "user2"}), true)
}

// --- merged from config_getters_test.go ---

// TestGettersDefaults exercises every getter on an empty viper so the
// default-value branches are covered.
func TestGettersDefaults(t *testing.T) {
	viper.Reset()

	testifyassert.True(t, IsCryptoEnable())
	testifyassert.True(t, IsHealthCheckEnabled())
	testifyassert.Equal(t, 0, GetHealthCheckPort())
	testifyassert.False(t, IsMetricsEnabled())
	testifyassert.Equal(t, 0, GetMetricsPort())
	testifyassert.True(t, IsLeaderElectionEnable())
	testifyassert.Equal(t, 0, GetServerPort())
	testifyassert.False(t, IsSSHEnable())
	testifyassert.Equal(t, "", GetSSHServerIP())
	testifyassert.Equal(t, 0, GetSSHServerPort())
	testifyassert.Equal(t, float64(0), GetMemoryReservePercent())
	testifyassert.Equal(t, float64(0), GetCpuReservePercent())
	testifyassert.Equal(t, float64(0), GetEphemeralStoreReservePercent())
	testifyassert.Equal(t, float64(0), GetMaxEphemeralStorePercent())
	testifyassert.Equal(t, 0, GetWorkloadHangCheckInterval())
	testifyassert.Equal(t, 60, GetWorkloadTTLSecond())
	testifyassert.False(t, IsOpenSearchEnable())
	testifyassert.Equal(t, "", GetOpenSearchEndpoint())
	testifyassert.Equal(t, "", GetOpenSearchIndexPrefix())
	testifyassert.False(t, IsDBEnable())
	testifyassert.Equal(t, "require", GetDBSslMode())
	testifyassert.Equal(t, "read-write", GetDBTargetSessionAttrs())
	testifyassert.Equal(t, 100, GetDBMaxOpenConns())
	testifyassert.Equal(t, 10, GetDBMaxIdleConns())
	testifyassert.Equal(t, 600, GetDBMaxLifetimeSecond())
	testifyassert.Equal(t, 60, GetDBMaxIdleTimeSecond())
	testifyassert.Equal(t, 10, GetDBConnectTimeoutSecond())
	testifyassert.Equal(t, 20, GetDBRequestTimeoutSecond())
	testifyassert.Equal(t, 60, GetOpsJobTTLSecond())
	testifyassert.Equal(t, 0, GetOpsJobTimeoutSecond())
	testifyassert.NotEmpty(t, GetDownloadJoImage())
	testifyassert.NotEmpty(t, GetEvalScopeImage())
	testifyassert.Equal(t, 900, GetPrewarmTimeoutSecond())
	testifyassert.Equal(t, 10, GetPrewarmWorkerConcurrent())
	testifyassert.False(t, IsS3Enable())
	testifyassert.Equal(t, int32(0), GetS3ExpireDay())
	testifyassert.Equal(t, "", GetRdmaName())
	testifyassert.Equal(t, "", GetImageSecret())
	testifyassert.Equal(t, -1, GetUserTokenExpire())
	testifyassert.False(t, IsOutboundTLSVerifyEnabled())
	testifyassert.True(t, IsNotificationEnable())
	testifyassert.Equal(t, "", GetSystemHost())
	testifyassert.Equal(t, "", GetSubDomain())
	testifyassert.Equal(t, "", GetIngress())
	testifyassert.False(t, IsSSOEnable())
	testifyassert.False(t, IsCICDEnable())
	testifyassert.Equal(t, "", GetCICDRoleName())
	testifyassert.Equal(t, "", GetCICDControllerName())
	testifyassert.NotEmpty(t, GetModelDownloaderImage())
	testifyassert.NotEmpty(t, GetModelCleanupImage())
	testifyassert.Empty(t, GetComponents())
	testifyassert.True(t, IsCDRequireApproval())
	testifyassert.Equal(t, "", GetTorchFTLightHouse())
	testifyassert.NotEmpty(t, GetCDJobImage())
	testifyassert.False(t, IsTracingEnable())
	testifyassert.Equal(t, "error_only", GetTracingMode())
	testifyassert.Equal(t, 1.0, GetTracingSamplingRatio())
	testifyassert.Equal(t, "", GetTracingOtlpEndpoint())
	testifyassert.False(t, IsA2AScannerEnable())
	testifyassert.Equal(t, 60, GetA2AScannerInterval())
	testifyassert.Equal(t, "a2a.primus.io/enabled=true", GetA2AScannerLabelSelector())
	testifyassert.False(t, IsLLMGatewayEnable())
	testifyassert.False(t, IsMonarchEnable())
	testifyassert.False(t, IsSandboxEnable())
	testifyassert.Equal(t, "", GetSandboxNamespace())
	testifyassert.Equal(t, "", GetSandboxSecret())
	testifyassert.False(t, IsMCPEnable())
	testifyassert.Equal(t, "/api/v1/safe-mcp/mcp", GetMCPBasePath())
	testifyassert.Equal(t, "", GetMCPInstructions())
	testifyassert.Equal(t, "", GetMonarchClientRole())
	testifyassert.False(t, IsModelOptimizationEnable())
	testifyassert.Equal(t, "agent_default", GetModelOptimizationClawAgentID())
	testifyassert.Equal(t, "control-plane-sandbox", GetModelOptimizationDefaultWorkspace())
	testifyassert.Equal(t, 1024, GetModelOptimizationMaxConcurrent())
	testifyassert.Equal(t, 4, GetModelOptimizationClawPluginID())
	testifyassert.Equal(t, "", GetModelOptimizationClawBaseURL())
}

// TestGettersWithValues sets every scalar key and verifies the value branch.
func TestGettersWithValues(t *testing.T) {
	viper.Reset()
	SetValue(cryptoEnable, "false")
	viper.Set(healthCheckEnable, false)
	viper.Set(healthCheckPort, 18080)
	viper.Set(metricsEnable, true)
	viper.Set(metricsPort, 19090)
	viper.Set(leaderElectionEnable, false)
	viper.Set(serverPort, 8080)
	viper.Set(sshEnable, true)
	viper.Set(sshServerIP, "1.2.3.4")
	viper.Set(sshServerPort, 22)
	viper.Set(memoryReservePercent, 0.1)
	viper.Set(serverPort, 8080)
	viper.Set(workloadTTLSecond, 120)
	viper.Set(openSearchEnable, true)
	viper.Set(openSearchEndpoint, "http://os:9200")
	viper.Set(dbEnable, true)
	viper.Set(dbSslMode, "disable")
	viper.Set(s3Enable, true)
	viper.Set(s3ExpireDay, 7)
	viper.Set(rdmaName, "rdma/ib")
	viper.Set(imageSecret, "regcred")
	viper.Set(domain, "amd.com")
	viper.Set(subDomain, "tw325")
	viper.Set(ingress, "higress")
	viper.Set(cicdEnable, true)
	viper.Set(cicdRoleName, "cicd")
	viper.Set(monarchEnable, true)
	viper.Set(monarchClientRole, "monarch")
	viper.Set(tracingEnable, true)
	viper.Set(tracingMode, "all")
	viper.Set(tracingSamplingRatio, 0.5)
	viper.Set(a2aScannerEnable, true)
	viper.Set(a2aScannerInterval, -5) // negative -> falls back to 60
	viper.Set(a2aScannerLabel, "x=y")
	viper.Set(mcpEnable, true)
	viper.Set(modelOptimizationEnable, true)
	viper.Set(modelOptimizationClawBaseURL, "http://claw/v1")
	viper.Set(modelOptimizationConcurrency, 16)

	testifyassert.False(t, IsCryptoEnable())
	testifyassert.False(t, IsHealthCheckEnabled())
	testifyassert.Equal(t, 18080, GetHealthCheckPort())
	testifyassert.True(t, IsMetricsEnabled())
	testifyassert.Equal(t, 19090, GetMetricsPort())
	testifyassert.False(t, IsLeaderElectionEnable())
	testifyassert.Equal(t, 8080, GetServerPort())
	testifyassert.True(t, IsSSHEnable())
	testifyassert.Equal(t, "1.2.3.4", GetSSHServerIP())
	testifyassert.Equal(t, 22, GetSSHServerPort())
	testifyassert.Equal(t, 120, GetWorkloadTTLSecond())
	testifyassert.True(t, IsOpenSearchEnable())
	testifyassert.Equal(t, "http://os:9200", GetOpenSearchEndpoint())
	testifyassert.True(t, IsDBEnable())
	testifyassert.Equal(t, "disable", GetDBSslMode())
	testifyassert.True(t, IsS3Enable())
	testifyassert.Equal(t, int32(7), GetS3ExpireDay())
	testifyassert.Equal(t, "rdma/ib", GetRdmaName())
	testifyassert.Equal(t, "regcred", GetImageSecret())
	testifyassert.Equal(t, "tw325.amd.com", GetSystemHost())
	testifyassert.Equal(t, "higress", GetIngress())
	testifyassert.True(t, IsCICDEnable())
	testifyassert.Equal(t, "cicd", GetCICDRoleName())
	testifyassert.True(t, IsMonarchEnable())
	testifyassert.Equal(t, "monarch", GetMonarchClientRole())
	testifyassert.True(t, IsTracingEnable())
	testifyassert.Equal(t, "all", GetTracingMode())
	testifyassert.Equal(t, 0.5, GetTracingSamplingRatio())
	testifyassert.True(t, IsA2AScannerEnable())
	testifyassert.Equal(t, 60, GetA2AScannerInterval())
	testifyassert.Equal(t, "x=y", GetA2AScannerLabelSelector())
	testifyassert.True(t, IsMCPEnable())
	testifyassert.True(t, IsModelOptimizationEnable())
	testifyassert.Equal(t, "http://claw/v1", GetModelOptimizationClawBaseURL())
	testifyassert.Equal(t, 16, GetModelOptimizationMaxConcurrent())
}

// TestSecretFileGetters covers every getFromFile-backed getter by pointing all
// secret-path keys at a temp directory populated with the expected item files.
func TestSecretFileGetters(t *testing.T) {
	viper.Reset()
	dir := t.TempDir()
	write := func(name, content string) {
		testifyassert.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0600))
	}
	write("key", "cryptokey")
	write("id_rsa", "priv")
	write("id_rsa.pub", "pub")
	write("username", "osuser")
	write("password", "ospass")
	write("host", "db-host")
	write("port", "5432")
	write("dbname", "safe")
	write("user", "dbuser")
	write("access_key", "ak")
	write("secret_key", "sk")
	write("bucket", "b1")
	write("endpoint", "http://s3")
	write("config", "notif-config")
	write("id", "ssoid")
	write("secret", "ssosecret")
	write("redirect_uri", "http://cb")
	write("public_key", "pk")
	write("litellm_endpoint", "http://litellm")
	write("litellm_admin_key", "adminkey")
	write("litellm_team_id", "team1")
	write("claw_base_url", "http://claw")
	write("claw_api_key", "clawkey")

	for _, k := range []string{
		cryptoSecretPath, sshSecretPath, openSearchSecretPath, dbSecretPath,
		s3SecretPath, notificationSecretPath, ssoSecretPath, langfuseProxySecretPath,
		llmGatewaySecretPath, modelOptimizationSecretPath,
	} {
		viper.Set(k, dir)
	}

	testifyassert.Equal(t, "cryptokey", GetCryptoKey())
	testifyassert.Equal(t, "pub", GetSSHRsaPublic())
	testifyassert.Equal(t, "priv", GetSSHRsaPrivate())
	testifyassert.Equal(t, "osuser", GetOpenSearchUser())
	testifyassert.Equal(t, "ospass", GetOpenSearchPasswd())
	testifyassert.Equal(t, "db-host", GetDBHost())
	testifyassert.Equal(t, 5432, GetDBPort())
	testifyassert.Equal(t, "safe", GetDBName())
	testifyassert.Equal(t, "dbuser", GetDBUser())
	testifyassert.Equal(t, "ospass", GetDBPassword())
	testifyassert.Equal(t, "ak", GetS3AccessKey())
	testifyassert.Equal(t, "sk", GetS3SecretKey())
	testifyassert.Equal(t, "b1", GetS3Bucket())
	testifyassert.Equal(t, "http://s3", GetS3Endpoint())
	testifyassert.Equal(t, "notif-config", GetNotificationConfig())
	testifyassert.Equal(t, "ssoid", GetSSOClientId())
	testifyassert.Equal(t, "ssosecret", GetSSOClientSecret())
	testifyassert.Equal(t, "http://s3", GetSSOEndpoint())
	testifyassert.Equal(t, "http://cb", GetSSORedirectURI())
	testifyassert.Equal(t, "pk", GetLangfuseProxyPublicKey())
	testifyassert.Equal(t, "sk", GetLangfuseProxySecretKey())
	testifyassert.Equal(t, "http://litellm", GetLLMGatewayEndpoint())
	testifyassert.Equal(t, "adminkey", GetLLMGatewayAdminKey())
	testifyassert.Equal(t, "team1", GetLLMGatewayTeamID())
	testifyassert.Equal(t, "clawkey", GetModelOptimizationClawAPIKey())

	// GetDBPort with non-numeric content -> 0
	write("port", "not-a-number")
	testifyassert.Equal(t, 0, GetDBPort())
}

// TestStringHelpers covers getStrings/removeBlank, GetAddons and slice getters.
func TestStringHelpers(t *testing.T) {
	viper.Reset()
	testifyassert.Empty(t, removeBlank([]string{"", "  ", "\t"}))
	testifyassert.Equal(t, []string{"a", "b"}, removeBlank([]string{" a ", "", "b"}))

	viper.Set(addonDefault, "nginx, redis ,")
	testifyassert.Equal(t, []string{"nginx", "redis"}, GetAddons(nil))
	v := "1.0"
	viper.Set(addonPrefix+"-1.0", "custom")
	testifyassert.Equal(t, []string{"custom"}, GetAddons(&v))

	viper.Set(mcpAllowedOrigins, "http://a, http://b")
	testifyassert.Equal(t, []string{"http://a", "http://b"}, GetMCPAllowedOrigins())

	viper.Set(a2aScannerNamespaces, []string{"ns1", "ns2"})
	testifyassert.Equal(t, []string{"ns1", "ns2"}, GetA2AScannerNamespaces())

	viper.Set(cdComponents, "c1,c2")
	testifyassert.Equal(t, []string{"c1", "c2"}, GetComponents())
}

// TestModelOptimizationClawBaseURLDerivation covers the domain-derived fallback.
func TestModelOptimizationClawBaseURLDerivation(t *testing.T) {
	viper.Reset()
	viper.Set(domain, "amd.com")
	testifyassert.Equal(t, "https://amd.com/claw-api/v1", GetModelOptimizationClawBaseURL())
	viper.Set(subDomain, "tw325")
	testifyassert.Equal(t, "https://tw325.amd.com/claw-api/v1", GetModelOptimizationClawBaseURL())
}

// --- merged from proxy_test.go ---

func TestGetProxyServices(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		expectedCount int
		expectedFirst *ProxyService
	}{
		{
			name: "empty proxy services",
			configContent: `
proxy:
  services: []
`,
			expectedCount: 0,
			expectedFirst: nil,
		},
		{
			name: "single proxy service",
			configContent: `
proxy:
  services:
    - name: qa-agent
      prefix: /agent/qa
      target: http://qa-agent-service:8080
      enabled: true
`,
			expectedCount: 1,
			expectedFirst: &ProxyService{
				Name:    "qa-agent",
				Prefix:  "/agent/qa",
				Target:  "http://qa-agent-service:8080",
				Enabled: true,
			},
		},
		{
			name: "multiple proxy services",
			configContent: `
proxy:
  services:
    - name: qa-agent
      prefix: /agent/qa
      target: http://qa-agent-service:8080
      enabled: true
    - name: data-service
      prefix: /api/data
      target: http://data-service:9000
      enabled: false
`,
			expectedCount: 2,
			expectedFirst: &ProxyService{
				Name:    "qa-agent",
				Prefix:  "/agent/qa",
				Target:  "http://qa-agent-service:8080",
				Enabled: true,
			},
		},
		{
			name: "no proxy config",
			configContent: `
server:
  port: 8088
`,
			expectedCount: 0,
			expectedFirst: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.configContent), 0644)
			testifyassert.NoError(t, err)

			// Reset viper for each test
			viper.Reset()

			// Load the config
			err = LoadConfig(configPath)
			testifyassert.NoError(t, err)

			// Get proxy services
			services := GetProxyServices()

			// Verify count
			assert.Equal(t, tt.expectedCount, len(services))

			// Verify first service if exists
			if tt.expectedFirst != nil && len(services) > 0 {
				assert.Equal(t, tt.expectedFirst.Name, services[0].Name)
				assert.Equal(t, tt.expectedFirst.Prefix, services[0].Prefix)
				assert.Equal(t, tt.expectedFirst.Target, services[0].Target)
				assert.Equal(t, tt.expectedFirst.Enabled, services[0].Enabled)
			}
		})
	}
}

func TestProxyServiceStruct(t *testing.T) {
	tests := []struct {
		name    string
		service ProxyService
		valid   bool
	}{
		{
			name: "valid service",
			service: ProxyService{
				Name:    "test-service",
				Prefix:  "/api/test",
				Target:  "http://test-service:8080",
				Enabled: true,
			},
			valid: true,
		},
		{
			name: "disabled service",
			service: ProxyService{
				Name:    "disabled-service",
				Prefix:  "/api/disabled",
				Target:  "http://disabled-service:8080",
				Enabled: false,
			},
			valid: true,
		},
		{
			name: "empty name",
			service: ProxyService{
				Name:    "",
				Prefix:  "/api/test",
				Target:  "http://test-service:8080",
				Enabled: true,
			},
			valid: false,
		},
		{
			name: "empty prefix",
			service: ProxyService{
				Name:    "test-service",
				Prefix:  "",
				Target:  "http://test-service:8080",
				Enabled: true,
			},
			valid: false,
		},
		{
			name: "empty target",
			service: ProxyService{
				Name:    "test-service",
				Prefix:  "/api/test",
				Target:  "",
				Enabled: true,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.service.Name != "" &&
				tt.service.Prefix != "" &&
				tt.service.Target != ""
			assert.Equal(t, tt.valid, isValid)
		})
	}
}

func TestGetProxyServicesWithInvalidYAML(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		shouldLoad    bool
	}{
		{
			name: "invalid yaml structure",
			configContent: `
proxy:
  services: "not-an-array"
`,
			shouldLoad: true, // This will load but return empty array
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.configContent), 0644)
			testifyassert.NoError(t, err)

			viper.Reset()

			// This might fail to load or return empty services
			err = LoadConfig(configPath)
			if tt.shouldLoad {
				testifyassert.NoError(t, err)
			}

			services := GetProxyServices()

			// Should return empty array on error or invalid structure
			testifyassert.NotNil(t, services)
		})
	}
}

func TestGetProxyServicesEnabledFiltering(t *testing.T) {
	configContent := `
proxy:
  services:
    - name: enabled-service
      prefix: /api/enabled
      target: http://enabled:8080
      enabled: true
    - name: disabled-service
      prefix: /api/disabled
      target: http://disabled:8080
      enabled: false
    - name: another-enabled
      prefix: /api/another
      target: http://another:8080
      enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	testifyassert.NoError(t, err)

	viper.Reset()
	err = LoadConfig(configPath)
	testifyassert.NoError(t, err)

	services := GetProxyServices()
	assert.Equal(t, 3, len(services))

	// Count enabled services
	enabledCount := 0
	for _, svc := range services {
		if svc.Enabled {
			enabledCount++
		}
	}
	assert.Equal(t, 2, enabledCount)
}

func TestProxyServiceDefaults(t *testing.T) {
	// Test that GetProxyServices returns empty array when no config is set
	viper.Reset()

	services := GetProxyServices()
	// GetProxyServices returns []ProxyService{} which is not nil
	assert.Equal(t, 0, len(services))
}

// --- merged from tls_config_test.go ---

// TestOutboundTLSVerifyFromConfig locks the config-key wiring: the value the
// Helm chart writes (tls.verify_outbound) must drive IsOutboundTLSVerifyEnabled.
// This guards against the getter and the chart drifting apart.
func TestOutboundTLSVerifyFromConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "enabled", content: "tls:\n  verify_outbound: true\n", want: true},
		{name: "disabled", content: "tls:\n  verify_outbound: false\n", want: false},
		{name: "absent defaults to false", content: "server:\n  port: 8088\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			viper.Reset()
			if err := LoadConfig(path); err != nil {
				t.Fatalf("load config: %v", err)
			}
			assert.Equal(t, tt.want, IsOutboundTLSVerifyEnabled())
		})
	}
}

// TestDBTargetSessionAttrsHonoursAnExplicitEmpty covers the Go half of the way
// out of this setting.
//
// Asking for a writable session is right for every component here, so it is on
// by default -- but a deployment whose db host resolves to a replica has to be
// able to clear it, or every connection it makes is refused outright. Clearing
// it means an explicit empty value, and an empty value is exactly what a getter
// is most likely to quietly replace with its default. getString tests
// viper.IsSet rather than the value, which is what makes the escape hatch
// reachable; nothing else states that, and reading the default back here would
// mean a deployment could not turn this off at all.
func TestDBTargetSessionAttrsHonoursAnExplicitEmpty(t *testing.T) {
	viper.Reset()
	testifyassert.Equal(t, "read-write", GetDBTargetSessionAttrs(), "unset must ask for a writable session")

	viper.Set(dbTargetSessionAttrs, "")
	testifyassert.Equal(t, "", GetDBTargetSessionAttrs(), "an explicit empty must survive, or the setting cannot be turned off")

	viper.Set(dbTargetSessionAttrs, "any")
	testifyassert.Equal(t, "any", GetDBTargetSessionAttrs())
	viper.Reset()
}
