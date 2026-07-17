/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestGettersDefaults exercises every getter on an empty viper so the
// default-value branches are covered.
func TestGettersDefaults(t *testing.T) {
	viper.Reset()

	assert.True(t, IsCryptoEnable())
	assert.True(t, IsHealthCheckEnabled())
	assert.Equal(t, 0, GetHealthCheckPort())
	assert.False(t, IsMetricsEnabled())
	assert.Equal(t, 0, GetMetricsPort())
	assert.True(t, IsLeaderElectionEnable())
	assert.Equal(t, 0, GetServerPort())
	assert.False(t, IsSSHEnable())
	assert.Equal(t, "", GetSSHServerIP())
	assert.Equal(t, 0, GetSSHServerPort())
	assert.Equal(t, float64(0), GetMemoryReservePercent())
	assert.Equal(t, float64(0), GetCpuReservePercent())
	assert.Equal(t, float64(0), GetEphemeralStoreReservePercent())
	assert.Equal(t, float64(0), GetMaxEphemeralStorePercent())
	assert.Equal(t, 0, GetWorkloadHangCheckInterval())
	assert.Equal(t, 60, GetWorkloadTTLSecond())
	assert.False(t, IsOpenSearchEnable())
	assert.Equal(t, "", GetOpenSearchEndpoint())
	assert.Equal(t, "", GetOpenSearchIndexPrefix())
	assert.False(t, IsDBEnable())
	assert.Equal(t, "require", GetDBSslMode())
	assert.Equal(t, 100, GetDBMaxOpenConns())
	assert.Equal(t, 10, GetDBMaxIdleConns())
	assert.Equal(t, 600, GetDBMaxLifetimeSecond())
	assert.Equal(t, 60, GetDBMaxIdleTimeSecond())
	assert.Equal(t, 10, GetDBConnectTimeoutSecond())
	assert.Equal(t, 20, GetDBRequestTimeoutSecond())
	assert.Equal(t, 60, GetOpsJobTTLSecond())
	assert.Equal(t, 0, GetOpsJobTimeoutSecond())
	assert.NotEmpty(t, GetDownloadJoImage())
	assert.NotEmpty(t, GetEvalScopeImage())
	assert.Equal(t, 900, GetPrewarmTimeoutSecond())
	assert.Equal(t, 10, GetPrewarmWorkerConcurrent())
	assert.False(t, IsS3Enable())
	assert.Equal(t, int32(0), GetS3ExpireDay())
	assert.Equal(t, "", GetRdmaName())
	assert.Equal(t, "", GetImageSecret())
	assert.Equal(t, -1, GetUserTokenExpire())
	assert.False(t, IsOutboundTLSVerifyEnabled())
	assert.True(t, IsNotificationEnable())
	assert.Equal(t, "", GetSystemHost())
	assert.Equal(t, "", GetSubDomain())
	assert.Equal(t, "", GetIngress())
	assert.False(t, IsSSOEnable())
	assert.False(t, IsCICDEnable())
	assert.Equal(t, "", GetCICDRoleName())
	assert.Equal(t, "", GetCICDControllerName())
	assert.NotEmpty(t, GetModelDownloaderImage())
	assert.NotEmpty(t, GetModelCleanupImage())
	assert.Empty(t, GetComponents())
	assert.True(t, IsCDRequireApproval())
	assert.Equal(t, "", GetTorchFTLightHouse())
	assert.NotEmpty(t, GetCDJobImage())
	assert.False(t, IsTracingEnable())
	assert.Equal(t, "error_only", GetTracingMode())
	assert.Equal(t, 1.0, GetTracingSamplingRatio())
	assert.Equal(t, "", GetTracingOtlpEndpoint())
	assert.False(t, IsA2AScannerEnable())
	assert.Equal(t, 60, GetA2AScannerInterval())
	assert.Equal(t, "a2a.primus.io/enabled=true", GetA2AScannerLabelSelector())
	assert.False(t, IsLLMGatewayEnable())
	assert.False(t, IsMonarchEnable())
	assert.False(t, IsSandboxEnable())
	assert.Equal(t, "", GetSandboxNamespace())
	assert.Equal(t, "", GetSandboxSecret())
	assert.False(t, IsMCPEnable())
	assert.Equal(t, "/api/v1/safe-mcp/mcp", GetMCPBasePath())
	assert.Equal(t, "", GetMCPInstructions())
	assert.Equal(t, "", GetMonarchClientRole())
	assert.False(t, IsModelOptimizationEnable())
	assert.Equal(t, "agent_default", GetModelOptimizationClawAgentID())
	assert.Equal(t, "control-plane-sandbox", GetModelOptimizationDefaultWorkspace())
	assert.Equal(t, 1024, GetModelOptimizationMaxConcurrent())
	assert.Equal(t, 4, GetModelOptimizationClawPluginID())
	assert.Equal(t, "", GetModelOptimizationClawBaseURL())
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

	assert.False(t, IsCryptoEnable())
	assert.False(t, IsHealthCheckEnabled())
	assert.Equal(t, 18080, GetHealthCheckPort())
	assert.True(t, IsMetricsEnabled())
	assert.Equal(t, 19090, GetMetricsPort())
	assert.False(t, IsLeaderElectionEnable())
	assert.Equal(t, 8080, GetServerPort())
	assert.True(t, IsSSHEnable())
	assert.Equal(t, "1.2.3.4", GetSSHServerIP())
	assert.Equal(t, 22, GetSSHServerPort())
	assert.Equal(t, 120, GetWorkloadTTLSecond())
	assert.True(t, IsOpenSearchEnable())
	assert.Equal(t, "http://os:9200", GetOpenSearchEndpoint())
	assert.True(t, IsDBEnable())
	assert.Equal(t, "disable", GetDBSslMode())
	assert.True(t, IsS3Enable())
	assert.Equal(t, int32(7), GetS3ExpireDay())
	assert.Equal(t, "rdma/ib", GetRdmaName())
	assert.Equal(t, "regcred", GetImageSecret())
	assert.Equal(t, "tw325.amd.com", GetSystemHost())
	assert.Equal(t, "higress", GetIngress())
	assert.True(t, IsCICDEnable())
	assert.Equal(t, "cicd", GetCICDRoleName())
	assert.True(t, IsMonarchEnable())
	assert.Equal(t, "monarch", GetMonarchClientRole())
	assert.True(t, IsTracingEnable())
	assert.Equal(t, "all", GetTracingMode())
	assert.Equal(t, 0.5, GetTracingSamplingRatio())
	assert.True(t, IsA2AScannerEnable())
	assert.Equal(t, 60, GetA2AScannerInterval())
	assert.Equal(t, "x=y", GetA2AScannerLabelSelector())
	assert.True(t, IsMCPEnable())
	assert.True(t, IsModelOptimizationEnable())
	assert.Equal(t, "http://claw/v1", GetModelOptimizationClawBaseURL())
	assert.Equal(t, 16, GetModelOptimizationMaxConcurrent())
}

// TestSecretFileGetters covers every getFromFile-backed getter by pointing all
// secret-path keys at a temp directory populated with the expected item files.
func TestSecretFileGetters(t *testing.T) {
	viper.Reset()
	dir := t.TempDir()
	write := func(name, content string) {
		assert.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0600))
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

	assert.Equal(t, "cryptokey", GetCryptoKey())
	assert.Equal(t, "pub", GetSSHRsaPublic())
	assert.Equal(t, "priv", GetSSHRsaPrivate())
	assert.Equal(t, "osuser", GetOpenSearchUser())
	assert.Equal(t, "ospass", GetOpenSearchPasswd())
	assert.Equal(t, "db-host", GetDBHost())
	assert.Equal(t, 5432, GetDBPort())
	assert.Equal(t, "safe", GetDBName())
	assert.Equal(t, "dbuser", GetDBUser())
	assert.Equal(t, "ospass", GetDBPassword())
	assert.Equal(t, "ak", GetS3AccessKey())
	assert.Equal(t, "sk", GetS3SecretKey())
	assert.Equal(t, "b1", GetS3Bucket())
	assert.Equal(t, "http://s3", GetS3Endpoint())
	assert.Equal(t, "notif-config", GetNotificationConfig())
	assert.Equal(t, "ssoid", GetSSOClientId())
	assert.Equal(t, "ssosecret", GetSSOClientSecret())
	assert.Equal(t, "http://s3", GetSSOEndpoint())
	assert.Equal(t, "http://cb", GetSSORedirectURI())
	assert.Equal(t, "pk", GetLangfuseProxyPublicKey())
	assert.Equal(t, "sk", GetLangfuseProxySecretKey())
	assert.Equal(t, "http://litellm", GetLLMGatewayEndpoint())
	assert.Equal(t, "adminkey", GetLLMGatewayAdminKey())
	assert.Equal(t, "team1", GetLLMGatewayTeamID())
	assert.Equal(t, "clawkey", GetModelOptimizationClawAPIKey())

	// GetDBPort with non-numeric content -> 0
	write("port", "not-a-number")
	assert.Equal(t, 0, GetDBPort())
}

// TestStringHelpers covers getStrings/removeBlank, GetAddons and slice getters.
func TestStringHelpers(t *testing.T) {
	viper.Reset()
	assert.Empty(t, removeBlank([]string{"", "  ", "\t"}))
	assert.Equal(t, []string{"a", "b"}, removeBlank([]string{" a ", "", "b"}))

	viper.Set(addonDefault, "nginx, redis ,")
	assert.Equal(t, []string{"nginx", "redis"}, GetAddons(nil))
	v := "1.0"
	viper.Set(addonPrefix+"-1.0", "custom")
	assert.Equal(t, []string{"custom"}, GetAddons(&v))

	viper.Set(mcpAllowedOrigins, "http://a, http://b")
	assert.Equal(t, []string{"http://a", "http://b"}, GetMCPAllowedOrigins())

	viper.Set(a2aScannerNamespaces, []string{"ns1", "ns2"})
	assert.Equal(t, []string{"ns1", "ns2"}, GetA2AScannerNamespaces())

	viper.Set(cdComponents, "c1,c2")
	assert.Equal(t, []string{"c1", "c2"}, GetComponents())
}

// TestModelOptimizationClawBaseURLDerivation covers the domain-derived fallback.
func TestModelOptimizationClawBaseURLDerivation(t *testing.T) {
	viper.Reset()
	viper.Set(domain, "amd.com")
	assert.Equal(t, "https://amd.com/claw-api/v1", GetModelOptimizationClawBaseURL())
	viper.Set(subDomain, "tw325")
	assert.Equal(t, "https://tw325.amd.com/claw-api/v1", GetModelOptimizationClawBaseURL())
}

