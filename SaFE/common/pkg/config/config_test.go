/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/spf13/viper"
	testifyassert "github.com/stretchr/testify/assert"
	testifyrequire "github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

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
	testifyassert.Equal(t, "", GetRobustServiceToken())
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
	testifyassert.Equal(t, 7200, GetModelPrewarmTimeoutSecond())
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
	testifyassert.False(t, IsLLMGatewayEnable())
	testifyassert.False(t, IsMonarchEnable())
	testifyassert.False(t, IsSandboxEnable())
	testifyassert.Equal(t, "", GetSandboxNamespace())
	testifyassert.Equal(t, "", GetSandboxSecret())
	testifyassert.Equal(t, "", GetSandboxEnvdInjectorImage())
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
	write("shared_apim_key_hashes", " hash1, ,hash2 \n")
	write("shared_apim_user", " svc-user\n")
	write("claw_base_url", "http://claw")
	write("claw_api_key", "clawkey")
	write(RobustServiceTokenKey, " robust-token\n")

	for _, k := range []string{
		cryptoSecretPath, sshSecretPath, openSearchSecretPath, dbSecretPath,
		s3SecretPath, notificationSecretPath, ssoSecretPath, langfuseProxySecretPath,
		llmGatewaySecretPath, modelOptimizationSecretPath, robustSecretPath,
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
	testifyassert.Equal(t, []string{"hash1", "hash2"}, GetLLMGatewaySharedApimKeyHashes())
	testifyassert.Equal(t, "svc-user", GetLLMGatewaySharedApimUser())
	testifyassert.Equal(t, "clawkey", GetModelOptimizationClawAPIKey())
	testifyassert.Equal(t, "robust-token", GetRobustServiceToken())

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

	viper.Set(cdComponents, "c1,c2")
	testifyassert.Equal(t, []string{"c1", "c2"}, GetComponents())
}

// TestSSHReverseForwardGetters pins the `ssh -R` policy defaults and overrides.
func TestSSHReverseForwardGetters(t *testing.T) {
	viper.Reset()
	// Without this the enable flag and the port range leak into every test that
	// runs after this one, which only shows up under some -run orders.
	t.Cleanup(viper.Reset)

	testifyassert.True(t, IsSSHReverseForwardEnable())
	testifyassert.Equal(t, []string{"127.0.0.1"}, GetSSHReverseForwardBindAddresses())
	testifyassert.Equal(t, 1024, GetSSHReverseForwardPortMin())
	testifyassert.Equal(t, 65535, GetSSHReverseForwardPortMax())
	testifyassert.Equal(t, 8, GetSSHReverseForwardMaxPerSession())

	viper.Set(sshReverseForwardEnable, false)
	testifyassert.False(t, IsSSHReverseForwardEnable(), "a deployment must be able to turn it off")
	viper.Set(sshReverseForwardEnable, true)
	viper.Set(sshReverseForwardPortMin, 20000)
	viper.Set(sshReverseForwardPortMax, 20010)
	viper.Set(sshReverseForwardMaxPerSession, 2)
	testifyassert.True(t, IsSSHReverseForwardEnable())
	testifyassert.Equal(t, 20000, GetSSHReverseForwardPortMin())
	testifyassert.Equal(t, 20010, GetSSHReverseForwardPortMax())
	testifyassert.Equal(t, 2, GetSSHReverseForwardMaxPerSession())

	// Both a YAML list and a comma separated scalar are accepted.
	viper.Set(sshReverseForwardBindAddresses, []string{"127.0.0.1", "0.0.0.0"})
	testifyassert.Equal(t, []string{"127.0.0.1", "0.0.0.0"}, GetSSHReverseForwardBindAddresses())
	viper.Set(sshReverseForwardBindAddresses, "127.0.0.1, 10.0.0.1")
	testifyassert.Equal(t, []string{"127.0.0.1", "10.0.0.1"}, GetSSHReverseForwardBindAddresses())

	// Deleting every entry is not the same as never configuring one: it leaves no
	// address a forward may bind, rather than quietly restoring loopback.
	viper.Set(sshReverseForwardBindAddresses, []string{})
	testifyassert.Empty(t, GetSSHReverseForwardBindAddresses())
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
    - name: my-service
      prefix: /my-service
      target: http://my-service:8080
      enabled: true
`,
			expectedCount: 1,
			expectedFirst: &ProxyService{
				Name:    "my-service",
				Prefix:  "/my-service",
				Target:  "http://my-service:8080",
				Enabled: true,
			},
		},
		{
			name: "multiple proxy services",
			configContent: `
proxy:
  services:
    - name: my-service
      prefix: /my-service
      target: http://my-service:8080
      enabled: true
    - name: data-service
      prefix: /api/data
      target: http://data-service:9000
      enabled: false
`,
			expectedCount: 2,
			expectedFirst: &ProxyService{
				Name:    "my-service",
				Prefix:  "/my-service",
				Target:  "http://my-service:8080",
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

func TestAuthKeyActors(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	testifyassert.Empty(t, GetAuthKeyActors(), "no delegated token may call POST /auth/verify unless the operator names its client")
	viper.Set("auth.key_actors", []string{"client-a", " client-b ", "", "  "})
	testifyassert.Equal(t, []string{"client-a", "client-b"}, GetAuthKeyActors(), "blank entries are dropped, ids are trimmed")
	viper.Set("auth.key_actors", "client-a, client-b")
	testifyassert.Equal(t, []string{"client-a", "client-b"}, GetAuthKeyActors(), "a comma separated scalar is accepted too")
	viper.Set("auth.key_actors", []string{})
	testifyassert.Empty(t, GetAuthKeyActors())
}

func TestAuthAPIKeysURL(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	testifyassert.Empty(t, GetAuthAPIKeysURL(), "nothing is routed to the identity provider unless the operator says so")
	for in, want := range map[string]string{
		"/api/v1/apikeys":                 "/api/v1/apikeys",
		"  /api/v1/apikeys/  ":            "/api/v1/apikeys",
		"https://idp.example.com/apikeys": "",
		"//idp.example.com/apikeys":       "",
		"api/v1/apikeys":                  "",
		"/api/v1/api keys":                "",
		"/api/v1/apikeys?x=1":             "",
		"/api/v1/apikeys#x":               "",
		"/api\\v1":                        "",
		"/":                               "",
		"":                                "",
	} {
		viper.Set("auth.api_keys_url", in)
		testifyassert.Equal(t, want, GetAuthAPIKeysURL(), "input %q", in)
	}
}

func TestAuthConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	// The default is the built-in mode, with nothing configured.
	testifyassert.Equal(t, AuthModeBuiltin, GetAuthMode())
	testifyassert.Equal(t, "", GetAuthLoginURL())
	list, err := GetAuthenticators()
	testifyassert.NoError(t, err)
	testifyassert.Empty(t, list)

	// Only the exact word switches the mode; a typo never turns authentication off.
	viper.Set("auth.mode", "externl")
	testifyassert.Equal(t, AuthModeBuiltin, GetAuthMode())
	viper.Set("auth.mode", "external")
	testifyassert.Equal(t, AuthModeExternal, GetAuthMode())

	viper.Set("auth.authenticators", []map[string]any{
		{"type": "oidc", "issuer": "https://idp.example.com", "audience": "a", "jwks_url": "https://idp.example.com/keys", "clock_skew": "1m", "groups_claim": "roles"},
		{"type": "introspection", "url": "https://authn.example.com/oauth/introspect", "client_id": "svc-safe", "client_secret_file": "/etc/secrets/auth/secret", "stale_if_error": "30m", "cache_negative_ttl": "5s", "audience": "safe-apiserver", "allow_missing_audience": true},
	})
	list, err = GetAuthenticators()
	testifyassert.NoError(t, err)
	testifyassert.Len(t, list, 2)
	testifyassert.Equal(t, "https://idp.example.com/keys", list[0].JWKSURL)
	testifyassert.Equal(t, "roles", list[0].GroupsClaim)
	testifyassert.Equal(t, "30m", list[1].StaleIfError)
	testifyassert.Equal(t, "introspection", list[1].Type)
	testifyassert.Equal(t, "svc-safe", list[1].ClientID)
	testifyassert.Equal(t, "/etc/secrets/auth/secret", list[1].ClientSecretFile)
	testifyassert.Equal(t, "safe-apiserver", list[1].Audience)
	testifyassert.True(t, list[1].AllowMissingAudience)
	testifyassert.False(t, list[0].AllowMissingAudience, "an entry that does not say so refuses answers that name no audience")

	viper.Set("auth.group_mappings", []map[string]any{{"group": "admins", "roles": []string{"system-admin"}, "workspaces": []string{"w1"}}})
	maps, err := GetAuthGroupMappings()
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, []AuthGroupMapping{{Group: "admins", Roles: []string{"system-admin"}, Workspaces: []string{"w1"}}}, maps)

	viper.Set("auth.authenticators", "not a list")
	_, err = GetAuthenticators()
	testifyassert.Error(t, err, "an unreadable chain is an error, never an empty one")
}

// users.* and console.access_contact: what a key that is not set means, and what one that is set to
// something the operator might write means.
func TestUsersAndConsoleConfig(t *testing.T) {
	t.Cleanup(viper.Reset)

	viper.Reset()
	testifyassert.Nil(t, GetUserBaseRoles(), "unset is nil: the caller supplies the default")
	testifyassert.True(t, IsUserJoinDefaultWorkspaces(), "unset joins, as the platform always did")
	testifyassert.Equal(t, "", GetConsoleAccessContact())

	for _, tt := range []struct {
		name  string
		value any
		want  []string
	}{
		{"a list", []string{"self-read"}, []string{"self-read"}},
		{"a comma separated scalar", "self-read, viewer", []string{"self-read", "viewer"}},
		{"an empty list is empty, not unset", []string{}, nil},
		{"blank entries are dropped", []string{"self-read", " "}, []string{"self-read"}},
	} {
		viper.Reset()
		viper.Set("users.base_roles", tt.value)
		testifyassert.Equal(t, tt.want, GetUserBaseRoles(), tt.name)
	}

	for _, tt := range []struct {
		value any
		want  bool
	}{{true, true}, {false, false}, {"false", false}, {"true", true}} {
		viper.Reset()
		viper.Set("users.join_default_workspaces", tt.value)
		testifyassert.Equal(t, tt.want, IsUserJoinDefaultWorkspaces(), "%v", tt.value)
	}

	viper.Reset()
	viper.Set("console.access_contact", "  Ask your administrator  ")
	testifyassert.Equal(t, "Ask your administrator", GetConsoleAccessContact(), "surrounding space is not part of the line")
	viper.Set("console.access_contact", "")
	testifyassert.Equal(t, "", GetConsoleAccessContact())

	SetValue("users.join_default_workspaces", "false")
	testifyassert.False(t, IsUserJoinDefaultWorkspaces())
	UnsetValue("users.join_default_workspaces")
	testifyassert.True(t, IsUserJoinDefaultWorkspaces(), "UnsetValue reads as not configured, not as configured empty")
}

// --- merged from sso_bridge_test.go ---

func TestGetSSOBridgeAllowlist(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	// Unset means same origin only.
	testifyassert.Empty(t, GetSSOBridgeAllowlist())

	viper.Set(ssoBridgeAllowlist, []string{"https://a.example.com/x", " ", "https://b.example.com"})
	testifyassert.Equal(t, []string{"https://a.example.com/x", "https://b.example.com"}, GetSSOBridgeAllowlist())

	viper.Set(ssoBridgeAllowlist, "https://a.example.com, https://b.example.com")
	testifyassert.Equal(t, []string{"https://a.example.com", "https://b.example.com"}, GetSSOBridgeAllowlist())
}

// --- merged from save_image_test.go ---

func TestGetSaveImageCluster(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	file := filepath.Join(t.TempDir(), "config.yaml")
	testifyrequire.NoError(t, os.WriteFile(file, []byte(`save_image:
  clusters:
  - cluster: edge
    registry: edge.example.com
    target_project: custom
    staging_project: save-staging
    ca_secret: harbor/edge-ca
    staging_push_secret: primus-safe/edge-staging
    host_aliases:
    - ip: 10.0.0.5
      hostnames: [edge.example.com]
  - cluster: Central
    registry: central.example.com
`), 0o600))
	testifyrequire.NoError(t, LoadConfig(file))

	c, ok, err := GetSaveImageCluster("edge")
	testifyrequire.NoError(t, err)
	testifyrequire.True(t, ok)
	testifyassert.Equal(t, SaveImageCluster{
		Cluster: "edge", Registry: "edge.example.com", TargetProject: "custom",
		StagingProject: "save-staging", CASecret: "harbor/edge-ca", StagingPushSecret: "primus-safe/edge-staging",
		HostAliases: []HostAlias{{IP: "10.0.0.5", Hostnames: []string{"edge.example.com"}}},
	}, c)

	// Cluster IDs are matched exactly; they are values, not keys viper folds to lower case.
	_, ok, err = GetSaveImageCluster("Central")
	testifyrequire.NoError(t, err)
	testifyassert.True(t, ok)
	_, ok, err = GetSaveImageCluster("central")
	testifyrequire.NoError(t, err)
	testifyassert.False(t, ok)

	viper.Set(saveImageClusters, "not a list")
	_, _, err = GetSaveImageCluster("edge")
	testifyassert.Error(t, err, "settings that cannot be read are not an absence")
}

// The chart renders the operator's entries into both services that read them: the
// resource manager (where to stage and publish) and the job manager (host aliases).
func TestChartRendersSaveImageClusters(t *testing.T) {
	values := filepath.Join(t.TempDir(), "values.yaml")
	testifyrequire.NoError(t, os.WriteFile(values, []byte(`save_image:
  clusters:
  - cluster: edge
    registry: edge.example.com
    staging_project: stage
    host_aliases:
    - ip: 10.0.0.5
      hostnames: [edge.example.com]
`), 0o600))
	for _, component := range []string{"resource-manager", "job-manager"} {
		loadRendered(t, renderConfigMapData(t, component, "config.yaml", "-f", values))
		c, ok, err := GetSaveImageCluster("edge")
		testifyrequire.NoError(t, err, component)
		testifyrequire.True(t, ok, component)
		testifyassert.Equal(t, "edge.example.com", c.Registry, component)
		testifyassert.Equal(t, "stage", c.StagingProject, component)
		testifyassert.Equal(t, []HostAlias{{IP: "10.0.0.5", Hostnames: []string{"edge.example.com"}}}, c.HostAliases, component)
	}
	loadRendered(t, renderConfigMapData(t, "job-manager", "config.yaml"))
	_, ok, err := GetSaveImageCluster("edge")
	testifyrequire.NoError(t, err, "a stock install renders an empty list")
	testifyassert.False(t, ok)
}

// --- merged from chart_external_user_dir_test.go ---

// The job-manager reads external_execution.user_dir_mounts from the config the chart renders: off
// unless the operator turns it on, so external Pods keep mounting a hostpath volume whole.
func TestChartRendersExternalUserDirMounts(t *testing.T) {
	render := func(values ...string) string {
		return renderConfigMapData(t, "job-manager", "config.yaml", values...)
	}

	loadRendered(t, render())
	testifyassert.False(t, IsExternalUserDirMountsEnable())

	loadRendered(t, render("--set", "external_execution.user_dir_mounts=true"))
	testifyassert.True(t, IsExternalUserDirMountsEnable())
	// The other external execution settings are rendered next to it and are not disturbed.
	testifyassert.False(t, IsExternalExecutionEnable())

	loadRendered(t, render("--set", "external_execution.enabled=true", "--set", "external_execution.user_dir_mounts=false"))
	testifyassert.False(t, IsExternalUserDirMountsEnable())
	testifyassert.True(t, IsExternalExecutionEnable())
}

// --- merged from chart_robust_test.go ---

// robustTokenCallers are the components that call the Robust API through
// robustclient and therefore need its service token.
var robustTokenCallers = []string{"apiserver", "job-manager", "resource-manager"}

type renderedPodSecrets struct {
	mounts  map[string]string // volume name -> mount path, over all containers
	secrets map[string]struct {
		name     string
		optional bool
	}
}

// renderDeploymentSecrets returns the secret volumes and their mounts of each
// rendered Deployment, keyed by Deployment name.
func renderDeploymentSecrets(t *testing.T, values ...string) map[string]renderedPodSecrets {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	cmd := exec.Command("helm", append([]string{"template", "rel", chartPath}, values...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	testifyrequire.NoErrorf(t, err, "helm template failed:\n%s", stderr.String())

	result := map[string]renderedPodSecrets{}
	decoder := yaml.NewDecoder(bytes.NewReader(out))
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							VolumeMounts []struct {
								Name      string `yaml:"name"`
								MountPath string `yaml:"mountPath"`
							} `yaml:"volumeMounts"`
						} `yaml:"containers"`
						Volumes []struct {
							Name   string `yaml:"name"`
							Secret *struct {
								SecretName string `yaml:"secretName"`
								Optional   bool   `yaml:"optional"`
							} `yaml:"secret"`
						} `yaml:"volumes"`
					} `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"spec"`
		}
		if err := decoder.Decode(&doc); err != nil {
			testifyrequire.ErrorIsf(t, err, io.EOF, "rendered chart is not valid YAML: %v", err)
			break
		}
		if doc.Kind != "Deployment" {
			continue
		}
		pod := renderedPodSecrets{mounts: map[string]string{}, secrets: map[string]struct {
			name     string
			optional bool
		}{}}
		for _, c := range doc.Spec.Template.Spec.Containers {
			for _, m := range c.VolumeMounts {
				pod.mounts[m.Name] = m.MountPath
			}
		}
		for _, v := range doc.Spec.Template.Spec.Volumes {
			if v.Secret != nil {
				pod.secrets[v.Name] = struct {
					name     string
					optional bool
				}{v.Secret.SecretName, v.Secret.Optional}
			}
		}
		result[doc.Metadata.Name] = pod
	}
	return result
}

// TestChartGivesEveryRobustCallerTheServiceToken pins the whole path for each
// caller: the Secret is mounted (optional, so a missing Secret cannot stop the
// pod), the rendered config points robust.secret_path at the mount, and the
// getter reads the token from a file laid out the way the kubelet writes it.
func TestChartGivesEveryRobustCallerTheServiceToken(t *testing.T) {
	deployments := renderDeploymentSecrets(t)
	for _, component := range robustTokenCallers {
		t.Run(component, func(t *testing.T) {
			pod, ok := deployments["rel-"+component]
			testifyrequire.True(t, ok, "no rel-%s Deployment rendered", component)
			vol, ok := pod.secrets["robust-service-token"]
			testifyrequire.True(t, ok, "rel-%s has no robust-service-token secret volume", component)
			testifyassert.Equal(t, "robust-service-token", vol.name)
			testifyassert.True(t, vol.optional, "the Secret must be optional")
			mountPath := pod.mounts["robust-service-token"]
			testifyassert.Equal(t, "/etc/secrets/robust", mountPath)

			rendered := renderConfigMapData(t, "release-name-"+component, "config.yaml")
			loadRendered(t, rendered)
			testifyassert.Equal(t, mountPath, viper.GetString(robustSecretPath),
				"the config must read the token where the Deployment mounts it")

			// Re-point the configured directory at a temp copy of the mount.
			dir := t.TempDir()
			testifyrequire.NoError(t, os.WriteFile(filepath.Join(dir, RobustServiceTokenKey), []byte("tok\n"), 0o600))
			viper.Set(robustSecretPath, dir)
			testifyassert.Equal(t, "tok", GetRobustServiceToken())
		})
	}
}

func TestChartCanOmitTheRobustServiceToken(t *testing.T) {
	deployments := renderDeploymentSecrets(t, "--set", "robust.service_token_secret=")
	for _, component := range robustTokenCallers {
		pod := deployments["rel-"+component]
		_, ok := pod.secrets["robust-service-token"]
		testifyassert.False(t, ok, component)
		rendered := renderConfigMapData(t, "release-name-"+component, "config.yaml", "--set", "robust.service_token_secret=")
		testifyassert.False(t, strings.Contains(rendered, "robust:"), component)
	}
}

func TestChartRendersACustomRobustSecret(t *testing.T) {
	values := []string{"--set", "robust.service_token_secret=other", "--set", "robust.secret_path=/var/run/robust"}
	deployments := renderDeploymentSecrets(t, values...)
	for _, component := range robustTokenCallers {
		pod := deployments["rel-"+component]
		testifyassert.Equal(t, "other", pod.secrets["robust-service-token"].name, component)
		testifyassert.Equal(t, "/var/run/robust", pod.mounts["robust-service-token"], component)
		loadRendered(t, renderConfigMapData(t, "release-name-"+component, "config.yaml", values...))
		testifyassert.Equal(t, "/var/run/robust", viper.GetString(robustSecretPath), component)
	}
}

// TestRobustServiceTokenNeedsAConfiguredDirectory: with robust.secret_path
// unset the getter must not fall back to a file in the working directory.
func TestRobustServiceTokenNeedsAConfiguredDirectory(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	dir := t.TempDir()
	testifyrequire.NoError(t, os.WriteFile(filepath.Join(dir, RobustServiceTokenKey), []byte("stray"), 0o600))
	t.Chdir(dir)
	testifyassert.Equal(t, "", GetRobustServiceToken())
}

// --- merged from chart_render_test.go ---

// chartPath is the chart that renders the configuration consumed by SaFE services.
const chartPath = "../../../charts/primus-safe"

func renderConfigMapData(t *testing.T, name, key string, values ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	if _, err := os.Stat(chartPath); err != nil {
		t.Skipf("chart not found at %s", chartPath)
	}

	args := append([]string{"template", chartPath}, values...)
	cmd := exec.Command("helm", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	// helm puts the reason on stderr, and nothing below can run without a render.
	testifyrequire.NoErrorf(t, err, "helm template failed:\n%s", stderr.String())

	decoder := yaml.NewDecoder(strings.NewReader(string(out)))
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Data map[string]string `yaml:"data"`
		}
		if err := decoder.Decode(&doc); err != nil {
			// End of stream is the loop's exit; anything else means the chart
			// rendered something that is not YAML, which is worth saying out loud.
			testifyrequire.ErrorIsf(t, err, io.EOF, "rendered chart is not valid YAML: %v\n%s", err, out)
			break
		}
		if doc.Kind == "ConfigMap" && strings.Contains(doc.Metadata.Name, name) {
			if cfg, ok := doc.Data[key]; ok {
				return cfg
			}
		}
	}
	t.Fatalf("no %s key in rendered ConfigMap %s", key, name)
	return ""
}

func renderApiserverConfig(t *testing.T, values ...string) string {
	t.Helper()
	return renderConfigMapData(t, "apiserver", "config.yaml", values...)
}

// loadRendered writes the rendered config where LoadConfig can read it, so the
// getters are driven by the same file the apiserver reads in the cluster.
func loadRendered(t *testing.T, rendered string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	path := filepath.Join(t.TempDir(), "config.yaml")
	testifyassert.NoError(t, os.WriteFile(path, []byte(rendered), 0o600))
	testifyassert.NoError(t, LoadConfig(path))
}

// TestChartRendersAuthKeyActors pins that auth.key_actors reaches the getter, and that it is empty (nobody
// may fetch keys with a delegated token) unless the operator sets it.
func TestChartRendersAuthKeyActors(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t))
	testifyassert.Empty(t, GetAuthKeyActors())

	loadRendered(t, renderApiserverConfig(t, "--set", "auth.key_actors={client-a,client-b}"))
	testifyassert.Equal(t, []string{"client-a", "client-b"}, GetAuthKeyActors())

	// The other auth settings are rendered next to it and are not disturbed.
	loadRendered(t, renderApiserverConfig(t, "--set", "auth.mode=external", "--set", "auth.key_actors={client-a}"))
	testifyassert.Equal(t, AuthModeExternal, GetAuthMode())
	testifyassert.Equal(t, []string{"client-a"}, GetAuthKeyActors())
}

// renderWebEnv returns the environment of the console container (the "-web" Deployment) of the rendered chart.
func renderWebEnv(t *testing.T, values ...string) map[string]string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	cmd := exec.Command("helm", append([]string{"template", "rel", chartPath}, values...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	testifyrequire.NoErrorf(t, err, "helm template failed:\n%s", stderr.String())

	decoder := yaml.NewDecoder(strings.NewReader(string(out)))
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							Name string `yaml:"name"`
							Env  []struct {
								Name  string `yaml:"name"`
								Value string `yaml:"value"`
							} `yaml:"env"`
						} `yaml:"containers"`
					} `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"spec"`
		}
		if err := decoder.Decode(&doc); err != nil {
			testifyrequire.ErrorIsf(t, err, io.EOF, "rendered chart is not valid YAML: %v", err)
			break
		}
		if doc.Kind == "Deployment" && doc.Metadata.Name == "rel-web" {
			env := map[string]string{}
			for _, c := range doc.Spec.Template.Spec.Containers {
				for _, e := range c.Env {
					env[e.Name] = e.Value
				}
			}
			return env
		}
	}
	t.Fatal("no rel-web Deployment in the rendered chart")
	return nil
}

// TestChartRoutesAPIKeysToTheIdentityProvider pins the two halves of the console's key routing: the path the
// apiserver reports to the console (auth.api_keys_url, from GET /envs), and the route the console's nginx
// gets (API_KEYS_UPSTREAM). The one value api_keys_upstream gives both; api_keys_url alone is for a deployment
// whose own gateway routes the path; and nothing is routed unless the mode is external, so a built-in
// deployment's /api/v1/apikeys still reaches the apiserver.
func TestChartRoutesAPIKeysToTheIdentityProvider(t *testing.T) {
	const upstream = "identity.idns.svc.cluster.local:8080"
	for _, tt := range []struct {
		name         string
		values       []string
		wantURL      string
		wantUpstream string // "" means the console container has no API_KEYS_UPSTREAM
	}{
		{"stock install: nothing routed", nil, "", ""},
		{"external, nothing set", []string{"auth.mode=external"}, "", ""},
		{"external, upstream: the console's nginx routes it and the path is reported",
			[]string{"auth.mode=external", "auth.api_keys_upstream=" + upstream}, "/api/v1/apikeys", upstream},
		{"external, url only: the deployment's own gateway routes it",
			[]string{"auth.mode=external", "auth.api_keys_url=/api/v1/apikeys"}, "/api/v1/apikeys", ""},
		{"external, a url and an upstream: the path reported is the one the console's nginx routes",
			[]string{"auth.mode=external", "auth.api_keys_url=/idp/apikeys", "auth.api_keys_upstream=" + upstream}, "/api/v1/apikeys", upstream},
		{"builtin, upstream set: the console's nginx must not take the path",
			[]string{"auth.mode=builtin", "auth.api_keys_upstream=" + upstream}, "/api/v1/apikeys", ""},
		{"no mode, upstream set: the console's nginx must not take the path",
			[]string{"auth.api_keys_upstream=" + upstream}, "/api/v1/apikeys", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var args []string
			for _, v := range tt.values {
				args = append(args, "--set", v)
			}
			loadRendered(t, renderApiserverConfig(t, args...))
			testifyassert.Equal(t, tt.wantURL, GetAuthAPIKeysURL())

			env := renderWebEnv(t, args...)
			if tt.wantUpstream == "" {
				testifyassert.NotContains(t, env, "API_KEYS_UPSTREAM")
			} else {
				testifyassert.Equal(t, tt.wantUpstream, env["API_KEYS_UPSTREAM"])
			}
		})
	}
}

// TestChartRendersReverseForwardDefaults pins what a stock install actually gets:
// remote forwarding on, bound to loopback inside the pod, and limited.
func TestChartRendersReverseForwardDefaults(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t))

	testifyassert.True(t, IsSSHReverseForwardEnable())
	testifyassert.Equal(t, []string{"127.0.0.1"}, GetSSHReverseForwardBindAddresses())
	testifyassert.Equal(t, 1024, GetSSHReverseForwardPortMin())
	testifyassert.Equal(t, 65535, GetSSHReverseForwardPortMax())
	testifyassert.Equal(t, 8, GetSSHReverseForwardMaxPerSession())
}

// TestChartRendersReverseForwardOverrides pins that values written by an operator
// reach the getters, including turning the feature off - the case Helm's `default`
// used to swallow.
func TestChartRendersReverseForwardOverrides(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t,
		"--set", "ssh.reverse_forward.enable=false",
		"--set", "ssh.reverse_forward.port_min=20000",
		"--set", "ssh.reverse_forward.port_max=20010",
		"--set", "ssh.reverse_forward.max_forwards_per_session=3",
		"--set", "ssh.reverse_forward.bind_addresses={127.0.0.1,0.0.0.0}",
	))

	// Turning it off has to reach the getters too. Helm's `default` treats false as
	// absent, so writing enable: false once rendered back to on.
	testifyassert.False(t, IsSSHReverseForwardEnable())
	testifyassert.Equal(t, []string{"127.0.0.1", "0.0.0.0"}, GetSSHReverseForwardBindAddresses())
	testifyassert.Equal(t, 20000, GetSSHReverseForwardPortMin())
	testifyassert.Equal(t, 20010, GetSSHReverseForwardPortMax())
	testifyassert.Equal(t, 3, GetSSHReverseForwardMaxPerSession())
}

// TestChartRendersReverseForwardEnabledExplicitly covers the other direction: an
// operator who writes the default out in full gets what they wrote, rather than a
// render that only works while the value is absent.
func TestChartRendersReverseForwardEnabledExplicitly(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t, "--set", "ssh.reverse_forward.enable=true"))
	testifyassert.True(t, IsSSHReverseForwardEnable())
}

// TestChartRendersAnEmptyBindListAsEmpty pins the other half of the same trap: an
// operator who removes every bind address gets none, not the default put back.
func TestChartRendersAnEmptyBindListAsEmpty(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t, "--set", "ssh.reverse_forward.bind_addresses={}"))
	testifyassert.Empty(t, GetSSHReverseForwardBindAddresses())
}

func TestChartRendersCICDProxyRelayAsNativeSidecar(t *testing.T) {
	type container struct {
		Name          string `yaml:"name"`
		RestartPolicy string `yaml:"restartPolicy"`
		Resources     struct {
			Limits   map[string]string `yaml:"limits"`
			Requests map[string]string `yaml:"requests"`
		} `yaml:"resources"`
	}
	var runner struct {
		Spec struct {
			Spec struct {
				Containers     []container `yaml:"containers"`
				InitContainers []container `yaml:"initContainers"`
			} `yaml:"spec"`
		} `yaml:"spec"`
	}
	rendered := renderConfigMapData(t, "github-runner-template", "template",
		"--show-only", "templates/configmap/github_runner_template.yaml",
		"--set", "cicd.proxy_relay_image=example/proxy-relay:latest")
	testifyrequire.NoError(t, yaml.Unmarshal([]byte(rendered), &runner))

	for _, current := range runner.Spec.Spec.Containers {
		testifyassert.NotEqual(t, "proxy-relay", current.Name)
	}
	for _, current := range runner.Spec.Spec.InitContainers {
		if current.Name != "proxy-relay" {
			continue
		}
		testifyassert.Equal(t, "Always", current.RestartPolicy)
		testifyassert.Equal(t, map[string]string{"cpu": "500m", "memory": "256Mi"}, current.Resources.Limits)
		testifyassert.Empty(t, current.Resources.Requests)
		testifyassert.NotContains(t, current.Resources.Limits, "amd.com/gpu")
		return
	}
	t.Fatal("proxy-relay init container not found")
}

func TestChartRendersCICDProxyRelayUpstreamDomains(t *testing.T) {
	// The relay exists to tunnel what GitHub's IP allow list gates. Defaulting the list
	// to empty would instead force every destination upstream with no direct fallback,
	// so package mirrors and internal registries would fail as opaque tunnel errors.
	domains := func(t *testing.T, values ...string) string {
		t.Helper()
		rendered := renderConfigMapData(t, "github-runner-template", "template",
			append([]string{"--show-only", "templates/configmap/github_runner_template.yaml",
				"--set", "cicd.proxy_relay_image=example/proxy-relay:latest"}, values...)...)
		var runner struct {
			Spec struct {
				Spec struct {
					InitContainers []struct {
						Name string `yaml:"name"`
						Env  []struct {
							Name  string `yaml:"name"`
							Value string `yaml:"value"`
						} `yaml:"env"`
					} `yaml:"initContainers"`
				} `yaml:"spec"`
			} `yaml:"spec"`
		}
		testifyrequire.NoError(t, yaml.Unmarshal([]byte(rendered), &runner))
		for _, current := range runner.Spec.Spec.InitContainers {
			if current.Name != "proxy-relay" {
				continue
			}
			for _, env := range current.Env {
				if env.Name == "PROXY_UPSTREAM_DOMAINS" {
					return env.Value
				}
			}
			t.Fatal("PROXY_UPSTREAM_DOMAINS not found on the relay")
		}
		t.Fatal("proxy-relay init container not found")
		return ""
	}

	testifyassert.Equal(t, ".github.com .githubusercontent.com .ghcr.io", domains(t))
	testifyassert.Equal(t, ".corp.example",
		domains(t, "--set", "cicd.proxy_relay_upstream_domains=.corp.example"))
	// An explicit empty value stays the all-upstream escape hatch.
	testifyassert.Equal(t, "", domains(t, "--set", "cicd.proxy_relay_upstream_domains="))
}

func TestChartRendersCICDProxyRelayCredentialEncoding(t *testing.T) {
	type container struct {
		Name string   `yaml:"name"`
		Args []string `yaml:"args"`
	}
	var runner struct {
		Spec struct {
			Spec struct {
				InitContainers []container `yaml:"initContainers"`
			} `yaml:"spec"`
		} `yaml:"spec"`
	}
	rendered := renderConfigMapData(t, "github-runner-template", "template",
		"--show-only", "templates/configmap/github_runner_template.yaml",
		"--set", "cicd.proxy_relay_image=example/proxy-relay:latest")
	testifyrequire.NoError(t, yaml.Unmarshal([]byte(rendered), &runner))

	var script string
	for _, current := range runner.Spec.Spec.InitContainers {
		if current.Name == "proxy-relay" {
			testifyrequire.Len(t, current.Args, 1)
			script = current.Args[0]
			break
		}
	}
	testifyrequire.NotEmpty(t, script)
	testifyassert.Contains(t, script, "username=$(percent_encode </etc/secrets/proxy/username)")
	testifyassert.Contains(t, script, "pw=$(percent_encode </etc/secrets/proxy/password)")
	testifyassert.Contains(t, script, `login=" login=${username}:${pw}"`)

	start := strings.Index(script, "percent_encode() {")
	testifyrequire.NotEqual(t, -1, start)
	end := strings.Index(script[start:], "\n}")
	testifyrequire.NotEqual(t, -1, end)
	encoder := script[start : start+end+2]
	encode := func(t *testing.T, value string) string {
		t.Helper()
		cmd := exec.Command("/bin/sh", "-c", encoder+"\npercent_encode")
		cmd.Stdin = strings.NewReader(value)
		out, err := cmd.CombinedOutput()
		testifyrequire.NoErrorf(t, err, "relay credential encoder failed: %s", out)
		return string(out)
	}

	encodedUsername := encode(t, "proxy user%#")
	testifyassert.Regexp(t, `^(%[0-9a-f]{2})+$`, encodedUsername)
	for _, password := range []string{"p@ss w0rd", "p%40ss", "päss🔒"} {
		t.Run(password, func(t *testing.T) {
			encodedPassword := encode(t, password)
			testifyassert.Regexp(t, `^(%[0-9a-f]{2})+$`, encodedPassword)
			cachePeer := "cache_peer proxy.example parent 3128 0 no-query default login=" + encodedUsername + ":" + encodedPassword
			fields := strings.Fields(cachePeer)
			testifyrequire.Len(t, fields, 8)
			testifyassert.Equal(t, "login="+encodedUsername+":"+encodedPassword, fields[7])
			testifyassert.Equal(t, -1, strings.IndexFunc(fields[7], unicode.IsSpace))

			credentials := strings.SplitN(strings.TrimPrefix(fields[7], "login="), ":", 2)
			testifyrequire.Len(t, credentials, 2)
			username, err := url.PathUnescape(credentials[0])
			testifyrequire.NoError(t, err)
			decodedPassword, err := url.PathUnescape(credentials[1])
			testifyrequire.NoError(t, err)
			testifyassert.Equal(t, "proxy user%#", username)
			testifyassert.Equal(t, password, decodedPassword)
		})
	}
}

func TestChartRendersHostedRunnerRuntime(t *testing.T) {
	type container struct {
		Name            string   `yaml:"name"`
		Image           string   `yaml:"image"`
		RestartPolicy   string   `yaml:"restartPolicy"`
		Args            []string `yaml:"args"`
		SecurityContext struct {
			Privileged bool `yaml:"privileged"`
			RunAsUser  int  `yaml:"runAsUser"`
		} `yaml:"securityContext"`
	}
	var runner struct {
		Spec struct {
			Template struct {
				Spec struct {
					DNSPolicy                     string      `yaml:"dnsPolicy"`
					TerminationGracePeriodSeconds int         `yaml:"terminationGracePeriodSeconds"`
					Containers                    []container `yaml:"containers"`
					InitContainers                []container `yaml:"initContainers"`
				} `yaml:"spec"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}
	rendered := renderConfigMapData(t, "github-hosted-runner-template", "template",
		"--show-only", "templates/configmap/github_hosted_runner_template.yaml")
	testifyrequire.NoError(t, yaml.Unmarshal([]byte(rendered), &runner))

	testifyassert.Equal(t, "ClusterFirst", runner.Spec.Template.Spec.DNSPolicy)
	testifyassert.Equal(t, 180, runner.Spec.Template.Spec.TerminationGracePeriodSeconds)
	testifyrequire.Len(t, runner.Spec.Template.Spec.Containers, 1)
	testifyassert.Equal(t, "ghcr.io/actions/actions-runner:2.328.0",
		runner.Spec.Template.Spec.Containers[0].Image)
	testifyassert.False(t, runner.Spec.Template.Spec.Containers[0].SecurityContext.Privileged)
	testifyassert.Equal(t, 1001, runner.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser)

	names := map[string]container{}
	for _, current := range runner.Spec.Template.Spec.InitContainers {
		names[current.Name] = current
	}
	testifyassert.Equal(t, "ghcr.io/actions/actions-runner:2.328.0", names["init-dind-externals"].Image)
	testifyassert.Equal(t, "docker:28.3.3-dind", names["dind"].Image)
	testifyassert.Equal(t, "Always", names["dind"].RestartPolicy)
	testifyassert.True(t, names["dind"].SecurityContext.Privileged)
	testifyassert.Equal(t, []string{
		"dockerd",
		"--host=unix:///var/run/docker.sock",
		"--group=123",
	}, names["dind"].Args)
}

// sandboxPod is the part of the rendered sandbox template the extra CA touches.
type sandboxPod struct {
	Spec struct {
		PodTemplate struct {
			Spec struct {
				Containers []struct {
					Name    string   `yaml:"name"`
					Command []string `yaml:"command"`
					Args    []string `yaml:"args"`
					Env     []struct {
						Name      string                 `yaml:"name"`
						Value     string                 `yaml:"value"`
						ValueFrom map[string]interface{} `yaml:"valueFrom"`
					} `yaml:"env"`
					EnvFrom      []map[string]interface{} `yaml:"envFrom"`
					VolumeMounts []struct {
						Name      string `yaml:"name"`
						MountPath string `yaml:"mountPath"`
						ReadOnly  bool   `yaml:"readOnly"`
					} `yaml:"volumeMounts"`
				} `yaml:"containers"`
				InitContainers []struct {
					Name  string `yaml:"name"`
					Image string `yaml:"image"`
				} `yaml:"initContainers"`
				Volumes []struct {
					Name   string `yaml:"name"`
					Secret *struct {
						SecretName string `yaml:"secretName"`
					} `yaml:"secret"`
				} `yaml:"volumes"`
			} `yaml:"spec"`
		} `yaml:"podTemplate"`
	} `yaml:"spec"`
}

func renderSandboxPod(t *testing.T, values ...string) sandboxPod {
	t.Helper()
	args := append([]string{"--show-only", "templates/configmap/sandbox_template.yaml"}, values...)
	rendered := renderConfigMapData(t, "sandbox-template", "template", args...)
	var pod sandboxPod
	testifyrequire.NoError(t, yaml.Unmarshal([]byte(rendered), &pod))
	testifyrequire.Len(t, pod.Spec.PodTemplate.Spec.Containers, 1)
	return pod
}

var sandboxCAVariables = []string{"SSL_CERT_FILE", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE", "NODE_EXTRA_CA_CERTS"}

func TestChartRendersTheSandboxWithoutAnExtraCAByDefault(t *testing.T) {
	// The injector's setup script refuses to start a sandbox whose CA variables point
	// at the merged bundle when no certificate was found. Pointing them there with no
	// CA configured would stop every sandbox on a deployment that needs none.
	pod := renderSandboxPod(t)
	main := pod.Spec.PodTemplate.Spec.Containers[0]
	for _, current := range main.Env {
		testifyassert.NotContains(t, sandboxCAVariables, current.Name)
	}
	for _, current := range main.VolumeMounts {
		testifyassert.NotEqual(t, "extra-ca", current.Name)
	}
	for _, current := range pod.Spec.PodTemplate.Spec.Volumes {
		testifyassert.NotEqual(t, "extra-ca", current.Name)
	}
}

func TestChartMountsTheSandboxExtraCASecret(t *testing.T) {
	pod := renderSandboxPod(t, "--set", "sandbox.extra_ca_secret=sandbox-extra-ca")
	main := pod.Spec.PodTemplate.Spec.Containers[0]

	env := map[string]string{}
	for _, current := range main.Env {
		env[current.Name] = current.Value
	}
	for _, name := range sandboxCAVariables {
		testifyassert.Equal(t, "/shared/bin/ca-bundle.pem", env[name], name)
	}

	mounted := false
	for _, current := range main.VolumeMounts {
		if current.Name == "extra-ca" {
			mounted = true
			testifyassert.Equal(t, "/etc/claw/extra-ca", current.MountPath)
			testifyassert.True(t, current.ReadOnly)
		}
	}
	testifyassert.True(t, mounted, "extra-ca is not mounted in the main container")

	for _, current := range pod.Spec.PodTemplate.Spec.Volumes {
		if current.Name == "extra-ca" {
			testifyrequire.NotNil(t, current.Secret)
			testifyassert.Equal(t, "sandbox-extra-ca", current.Secret.SecretName)
			return
		}
	}
	t.Fatal("extra-ca volume not found")
}

func TestChartRendersNoSandboxEnvSecretByDefault(t *testing.T) {
	pod := renderSandboxPod(t)
	main := pod.Spec.PodTemplate.Spec.Containers[0]
	testifyassert.Empty(t, main.EnvFrom)
	for _, current := range main.Env {
		if current.Name == "NODE_NAME" {
			fieldRef, _ := current.ValueFrom["fieldRef"].(map[string]interface{})
			testifyassert.Equal(t, "spec.nodeName", fieldRef["fieldPath"])
			continue
		}
		testifyassert.Nil(t, current.ValueFrom, "%s reads from a reference", current.Name)
	}
}

func TestChartInjectsTheSandboxEnvSecret(t *testing.T) {
	plain := renderSandboxPod(t, "--set", "sandbox.extra_ca_secret=sandbox-extra-ca")
	pod := renderSandboxPod(t, "--set", "sandbox.env_secret=sandbox-env",
		"--set", "sandbox.extra_ca_secret=sandbox-extra-ca")
	main := pod.Spec.PodTemplate.Spec.Containers[0]
	// The whole Secret, by reference, optional; SecretEnvSource has no namespace, so it is the
	// Secret in the Pod's own (the workspace's) namespace.
	testifyassert.Equal(t, []map[string]interface{}{
		{"secretRef": map[string]interface{}{"name": "sandbox-env", "optional": true}},
	}, main.EnvFrom)
	// The explicit env is unchanged, and explicit env wins over envFrom in Kubernetes, so the
	// Secret cannot replace the CA variables.
	testifyassert.Equal(t, plain.Spec.PodTemplate.Spec.Containers[0].Env, main.Env)
}

// Only sandbox-env, the name the apiserver and resource-manager reserve whether or not the
// feature is on, is accepted: any other name would be guarded only once the apiserver had
// restarted with it, after the template already read it. A value helm parses as a number or a
// boolean (0, false) is refused too, not read as unset.
func TestChartRefusesAnySandboxEnvSecretNameButTheReservedOne(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	for _, name := range []string{"other", "sandbox-env2", "0", "false", "Not_A_Name", strings.Repeat("a", 254),
		"a\n- name: x"} {
		cmd := exec.Command("helm", "template", chartPath, "--show-only",
			"templates/configmap/sandbox_template.yaml", "--set", "sandbox.env_secret="+name)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		testifyrequire.Error(t, err, name)
		testifyassert.Contains(t, stderr.String(), "set it to sandbox-env or leave it empty", name)
		testifyassert.NotContains(t, string(out), "envFrom")
	}
}

// The apiserver needs no configuration for the guard, so the chart gives it none and turning the
// feature on does not restart it.
func TestChartLeavesTheApiserverAloneForTheSandboxEnvSecret(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	render := func(values ...string) string {
		args := append([]string{"template", chartPath, "--show-only", "templates/apiserver/deployment.yaml",
			"--show-only", "templates/apiserver/config.yaml"}, values...)
		out, err := exec.Command("helm", args...).Output()
		testifyrequire.NoError(t, err)
		return string(out)
	}
	testifyassert.Equal(t, render(), render("--set", "sandbox.env_secret=sandbox-env"))
}

// An image may set HSA_ENABLE_IPC_MODE_LEGACY=1 (ROCm vLLM images do). In an unprivileged Pod
// without host IPC the legacy amdkfd IPC import is refused, so every GPU-to-GPU IPC handle fails
// to open. The sandbox pins the default IPC path in the Pod spec, which overrides the image ENV
// for every process in the container, PID 1 included.
func TestChartPinsTheSandboxToTheNonLegacyHIPIPCPath(t *testing.T) {
	for _, values := range [][]string{nil, {"--set", "sandbox.extra_ca_secret=sandbox-extra-ca",
		"--set", "sandbox.env_secret=sandbox-env"}} {
		pod := renderSandboxPod(t, values...)
		main := pod.Spec.PodTemplate.Spec.Containers[0]
		found := 0
		for _, current := range main.Env {
			if current.Name == "HSA_ENABLE_IPC_MODE_LEGACY" {
				found++
				testifyassert.Equal(t, "0", current.Value, "values %v", values)
				testifyassert.Nil(t, current.ValueFrom, "values %v", values)
			}
		}
		testifyassert.Equal(t, 1, found, "HSA_ENABLE_IPC_MODE_LEGACY entries with values %v", values)
	}
}

// sandboxPathSetupScript is the PATH setup the startup command opens with, byte for byte. It is
// what pathSetupScript() in the PrimusClaw workload manager (sandbox/pkg/workloadmanager) returns
// for the sandboxes it builds itself, so a SaFE sandbox and a PrimusClaw one start with the same
// PATH from the same image. Change both together.
//
// It keeps the PATH the container starts with (the image's) first and in its order, drops entries
// that are empty, relative or contain a ".." segment, and appends each runtime directory it lacks
// once: the injected tools in /shared/bin, the workspace user's `pip install --user` scripts, and
// the system directories the startup command runs its own commands from.
const sandboxPathSetupScript = `__sandbox_path_rest="${PATH-}:"'/shared/bin:/home/sandbox/.local/bin:/usr/local/bin:/usr/bin:/bin:/sbin:'; __sandbox_path=; while [ -n "$__sandbox_path_rest" ]; do __sandbox_path_dir="${__sandbox_path_rest%%:*}"; __sandbox_path_rest="${__sandbox_path_rest#*:}"; case "$__sandbox_path_dir" in */../*|*/..) ;; /*) case ":$__sandbox_path:" in *":$__sandbox_path_dir:"*) ;; *) __sandbox_path="${__sandbox_path:+$__sandbox_path:}$__sandbox_path_dir" ;; esac ;; esac; done; PATH="$__sandbox_path"; export PATH; unset __sandbox_path_rest __sandbox_path_dir __sandbox_path; `

// sandboxStartupTail is what the startup command runs once PATH is set up.
const sandboxStartupTail = "mkdir -p /app /app/work && /shared/bin/setup-amd-ca.sh && if [ -n \"${HYPERLOOM_SIGNING_KEY_PEM-}\" ]; then keyfile=\"${HYPERLOOM_SIGNING_KEY_FILE:-/dev/shm/hyperloom-signing-key.pem}\"; case \"$keyfile\" in /app|/app/*) echo \"signing key path is under /app\" >&2; exit 1;; esac; mkdir -p \"$(dirname \"$keyfile\")\"; umask 077; printf '%s\\n' \"$HYPERLOOM_SIGNING_KEY_PEM\" > \"$keyfile\"; chmod 0400 \"$keyfile\"; unset HYPERLOOM_SIGNING_KEY_PEM; export HYPERLOOM_SIGNING_KEY_FILE=\"$keyfile\"; fi && if [ ! -f \"${HYPERLOOM_SIGNING_KEY_FILE-}\" ]; then unset HYPERLOOM_SIGNING_KEY_FILE; fi && umask 002 && if [ -n \"${SANDBOX_STARTUP-}\" ]; then eval \"$SANDBOX_STARTUP\"; fi && exec /shared/bin/envd --port=8080 --workspace=/app"

// sandboxRuntimePath is PATH when the container starts with none.
const sandboxRuntimePath = "/shared/bin:/home/sandbox/.local/bin:/usr/local/bin:/usr/bin:/bin:/sbin"

// A PATH env var on the container replaces the image's ENV PATH, so the template must set none,
// whatever else is turned on.
func TestChartSetsNoPathOnTheSandbox(t *testing.T) {
	for _, values := range [][]string{
		nil,
		{"--set", "sandbox.extra_ca_secret=sandbox-extra-ca", "--set", "sandbox.env_secret=sandbox-env"},
	} {
		pod := renderSandboxPod(t, values...)
		for _, current := range pod.Spec.PodTemplate.Spec.Containers[0].Env {
			testifyassert.NotEqual(t, "PATH", current.Name, "values %v", values)
		}
	}
}

// The rendered startup command is /bin/sh -c with exactly the PATH setup followed by the unchanged
// startup tail, whatever else is turned on: the chart's YAML quoting must not alter one byte of it.
func TestChartRendersTheSandboxPathSetupExactly(t *testing.T) {
	for _, values := range [][]string{
		nil,
		{"--set", "sandbox.extra_ca_secret=sandbox-extra-ca", "--set", "sandbox.env_secret=sandbox-env"},
	} {
		main := renderSandboxPod(t, values...).Spec.PodTemplate.Spec.Containers[0]
		testifyrequire.Equal(t, []string{"/bin/sh", "-c"}, main.Command, "values %v", values)
		testifyrequire.Equal(t, []string{sandboxPathSetupScript + sandboxStartupTail}, main.Args, "values %v", values)
		foundNode := false
		foundKey := false
		for _, current := range main.Env {
			if current.Name == "NODE_NAME" {
				foundNode = true
				fieldRef, _ := current.ValueFrom["fieldRef"].(map[string]interface{})
				testifyrequire.NotNil(t, fieldRef, "values %v", values)
				testifyassert.Equal(t, "spec.nodeName", fieldRef["fieldPath"], "values %v", values)
			}
			if current.Name == "HYPERLOOM_SIGNING_KEY_FILE" {
				foundKey = true
				testifyassert.Equal(t, "/run/signing-key/signing-key.pem", current.Value, "values %v", values)
				testifyassert.Nil(t, current.ValueFrom, "values %v", values)
			}
		}
		testifyrequire.True(t, foundNode, "values %v", values)
		testifyrequire.True(t, foundKey, "values %v", values)
	}
}

// The PATH setup, as rendered, runs under every POSIX shell on the host, since /bin/sh in a
// sandbox image may be any of them.
func TestChartSandboxStartupKeepsTheImagePath(t *testing.T) {
	args := renderSandboxPod(t).Spec.PodTemplate.Spec.Containers[0].Args
	testifyrequire.Len(t, args, 1)
	setup, found := strings.CutSuffix(args[0], sandboxStartupTail)
	testifyrequire.True(t, found, "startup command does not end in the startup tail: %s", args[0])
	var shells []string
	for _, name := range []string{"sh", "dash", "bash", "busybox"} {
		if shell, err := exec.LookPath(name); err == nil {
			shells = append(shells, shell)
		}
	}
	testifyrequire.NotEmpty(t, shells, "no POSIX shell on the host")
	cases := []struct {
		name, start, want string
	}{
		{"image path first, missing entries appended",
			"/opt/venv/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"/opt/venv/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/shared/bin:/home/sandbox/.local/bin"},
		{"nothing duplicated when every entry is present", sandboxRuntimePath, sandboxRuntimePath},
		{"a directory only named as a prefix still counts as missing", "/shared/bin2:/bin/x",
			"/shared/bin2:/bin/x:" + sandboxRuntimePath},
		{"an empty path becomes exactly the runtime entries", "", sandboxRuntimePath},
		{"a duplicate in the image path is kept once", "/opt/a:/opt/b:/opt/a", "/opt/a:/opt/b:" + sandboxRuntimePath},
		{"relative and empty entries are dropped", "bin:/opt/a::.:~/bin:/opt/b:",
			"/opt/a:/opt/b:" + sandboxRuntimePath},
		{"entries with a .. segment are dropped", "..:/opt/../etc:/opt/a:/opt/b/..:/opt/..x:/..",
			"/opt/a:/opt/..x:" + sandboxRuntimePath},
		{"a directory with a space or a glob is kept literally", "/opt/my tools/bin:/opt/*:/opt/[a]",
			"/opt/my tools/bin:/opt/*:/opt/[a]:" + sandboxRuntimePath},
	}
	for _, shell := range shells {
		for _, current := range cases {
			command := []string{shell}
			switch filepath.Base(shell) {
			case "busybox":
				command = append(command, "sh")
			case "bash":
				// No startup files: a host's bashrc may change PATH before the script runs.
				command = append(command, "--norc", "--noprofile", "--posix")
			}
			// The setup unsets its own variables; exit 7 marks one it left behind.
			command = append(command, "-c", setup+`for v in __sandbox_path_rest __sandbox_path_dir __sandbox_path; do eval "[ -z \"\${$v+x}\" ]" || exit 7; done; printf '%s' "$PATH"`)
			cmd := exec.Command(command[0], command[1:]...)
			cmd.Env = []string{"PATH=" + current.start}
			out, err := cmd.Output()
			testifyrequire.NoError(t, err, "%s: %s", shell, current.name)
			testifyassert.Equal(t, current.want, string(out), "%s: %s", shell, current.name)
		}
	}
}

func TestChartPullsTheEnvdInjectorThroughTheSiteProxy(t *testing.T) {
	pod := renderSandboxPod(t, "--set", "global.sub_domain=site", "--set", "global.domain=example.com")
	testifyrequire.Len(t, pod.Spec.PodTemplate.Spec.InitContainers, 1)
	testifyassert.True(t, strings.HasPrefix(pod.Spec.PodTemplate.Spec.InitContainers[0].Image,
		"harbor.site.example.com/proxy/primussafe/agent-sandbox-envd-injector@sha256:"))
}

// TestChartRendersModelMaxConcurrentDownloads pins what an operator's value means: unset
// is 4, and 0 (which Helm's `default` used to replace with 4) as well as a negative
// value reach the getter as "no limit".
func TestChartRendersModelMaxConcurrentDownloads(t *testing.T) {
	render := func(values ...string) {
		loadRendered(t, renderConfigMapData(t, "resource-manager", "config.yaml", values...))
	}
	render()
	testifyassert.Equal(t, 4, GetModelMaxConcurrentDownloads())
	render("--set", "model.max_concurrent_downloads=2")
	testifyassert.Equal(t, 2, GetModelMaxConcurrentDownloads())
	render("--set", "model.max_concurrent_downloads=0")
	testifyassert.Equal(t, 0, GetModelMaxConcurrentDownloads())
	render("--set", "model.max_concurrent_downloads=-1")
	testifyassert.Equal(t, -1, GetModelMaxConcurrentDownloads())
}

// TestChartRendersTheUsersAndConsoleDefaults pins what a stock install gets: every user holds the default role
// and joins the default workspaces, as the platform always did, and the console's no-access page says whom to
// ask. The webhook and the apiserver read the same values, so they are rendered from one definition and
// compared here.
func TestChartRendersTheUsersAndConsoleDefaults(t *testing.T) {
	for _, name := range []string{"apiserver", "webhooks"} {
		loadRendered(t, renderConfigMapData(t, name, "config.yaml"))
		testifyassert.Equal(t, []string{"default"}, GetUserBaseRoles(), name)
		testifyassert.True(t, IsUserJoinDefaultWorkspaces(), name)
	}
	loadRendered(t, renderApiserverConfig(t))
	testifyassert.Equal(t, "请联系管理员开通", GetConsoleAccessContact())
}

// TestChartRendersTheMinimalUsersConfiguration pins that the values an operator writes for a deployment that
// grants nothing until an administrator does reach both components, including the one Helm's `default`
// swallows: false.
func TestChartRendersTheMinimalUsersConfiguration(t *testing.T) {
	values := []string{"--set", "users.base_roles={self-read}", "--set", "users.join_default_workspaces=false"}
	for _, name := range []string{"apiserver", "webhooks"} {
		loadRendered(t, renderConfigMapData(t, name, "config.yaml", values...))
		testifyassert.Equal(t, []string{"self-read"}, GetUserBaseRoles(), name)
		testifyassert.False(t, IsUserJoinDefaultWorkspaces(), name)
	}

	// More than one, and an empty list (which is the default, not "no roles").
	loadRendered(t, renderConfigMapData(t, "webhooks", "config.yaml", "--set", "users.base_roles={self-read,viewer}"))
	testifyassert.Equal(t, []string{"self-read", "viewer"}, GetUserBaseRoles())
	loadRendered(t, renderConfigMapData(t, "webhooks", "config.yaml", "--set", "users.base_roles=null"))
	testifyassert.Equal(t, []string{"default"}, GetUserBaseRoles())
}

// TestChartRendersTheAccessContact pins that the line is carried as text (a value with a colon or a quote
// stays one string) and that an operator who sets it to nothing gets no line, not the default put back.
func TestChartRendersTheAccessContact(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"set", []string{"--set-string", "console.access_contact=Ask the platform team: #help"}, "Ask the platform team: #help"},
		{"a quote", []string{"--set-string", `console.access_contact=say "hello"`}, `say "hello"`},
		{"empty is no line", []string{"--set-string", "console.access_contact="}, ""},
	} {
		loadRendered(t, renderApiserverConfig(t, tt.args...))
		testifyassert.Equal(t, tt.want, GetConsoleAccessContact(), tt.name)
	}
}

// TestChartRendersTheConsoleReturnPrefixes pins that a stock install leaves console.return_prefixes out, so the
// apiserver's default list applies, and that a list an operator sets reaches the apiserver as that list.
func TestChartRendersTheConsoleReturnPrefixes(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t))
	testifyassert.Equal(t, DefaultConsoleReturnPrefixes, GetConsoleReturnPrefixes())

	loadRendered(t, renderApiserverConfig(t, "--set", "console.return_prefixes={/dispatron,/claw-api}"))
	testifyassert.Equal(t, []string{"/dispatron", "/claw-api"}, GetConsoleReturnPrefixes())

	loadRendered(t, renderApiserverConfig(t, "--set", "console.return_prefixes=null"))
	testifyassert.Equal(t, DefaultConsoleReturnPrefixes, GetConsoleReturnPrefixes())

	// An empty list is none, not the default put back.
	loadRendered(t, renderApiserverConfig(t, "--set-json", "console.return_prefixes=[]"))
	testifyassert.Equal(t, []string{}, GetConsoleReturnPrefixes())
}
