/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ProxyService represents a proxy service configuration
type ProxyService struct {
	Name       string `json:"name" yaml:"name" mapstructure:"name"`                      // Service name
	Prefix     string `json:"prefix" yaml:"prefix" mapstructure:"prefix"`                // URL prefix for the proxy route
	Target     string `json:"target" yaml:"target" mapstructure:"target"`                // Target service URL
	Enabled    bool   `json:"enabled" yaml:"enabled" mapstructure:"enabled"`             // Whether the proxy is enabled
	AuthHeader string `json:"auth_header" yaml:"auth_header" mapstructure:"auth_header"` // Optional: replace Authorization with Basic auth (format: "user:pass")
}

// SetValue sets a configuration value for the specified key.
func SetValue(key, value string) {
	viper.Set(key, value)
}

// LoadConfig loads configuration from the specified file path.
func LoadConfig(path string) error {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")
	return viper.ReadInConfig()
}

func getString(key, defaultValue string) string {
	if !viper.IsSet(key) {
		return defaultValue
	}
	return viper.GetString(key)
}

func getBool(key string, defaultValue bool) bool {
	if !viper.IsSet(key) {
		return defaultValue
	}
	return viper.GetBool(key)
}

func getInt(key string, defaultValue int) int {
	if !viper.IsSet(key) {
		return defaultValue
	}
	return viper.GetInt(key)
}

func getFloat(key string, defaultValue float64) float64 {
	if !viper.IsSet(key) {
		return defaultValue
	}
	return viper.GetFloat64(key)
}

// getStringSlice reads a YAML list, falling back to a comma separated scalar so
// both `["a", "b"]` and `"a,b"` are accepted.
func getStringSlice(key string, defaultValue []string) []string {
	if !viper.IsSet(key) {
		return defaultValue
	}
	// viper splits a scalar on whitespace, which would keep the commas; decide on
	// the raw value first, rather than doing the list work and throwing it away -
	// this is re-read for every SSH connection.
	if _, isScalar := viper.Get(key).(string); isScalar {
		return getStrings(key)
	}
	values := removeBlank(viper.GetStringSlice(key))
	// An empty result here means the key was set to an empty list, which is not the
	// same as leaving it out: the operator removed every entry, and answering with
	// the default would hand back a value they deleted.
	return values
}

func getStrings(key string) []string {
	val := viper.GetString(key)
	return removeBlank(strings.Split(val, ","))
}

func removeBlank(slice []string) []string {
	var result []string
	for _, val := range slice {
		if trim := strings.TrimSpace(val); trim != "" {
			result = append(result, trim)
		}
	}
	return result
}

// IsCryptoEnable returns whether encryption is enabled.
func IsCryptoEnable() bool {
	return getBool(cryptoEnable, true)
}

// GetCryptoKey returns the encryption key.
func GetCryptoKey() string {
	return getFromFile(cryptoSecretPath, "key")
}

// IsHealthCheckEnabled returns whether health checks are enabled.
func IsHealthCheckEnabled() bool {
	return getBool(healthCheckEnable, true)
}

// GetHealthCheckPort returns the port for health check endpoint.
func GetHealthCheckPort() int {
	return getInt(healthCheckPort, 0)
}

// IsMetricsEnabled returns whether the Prometheus metrics endpoint is enabled.
func IsMetricsEnabled() bool {
	return getBool(metricsEnable, false)
}

// GetMetricsPort returns the port for the Prometheus metrics endpoint.
func GetMetricsPort() int {
	return getInt(metricsPort, 0)
}

// IsLeaderElectionEnable returns whether leader election is enabled.
func IsLeaderElectionEnable() bool {
	return getBool(leaderElectionEnable, true)
}

// GetServerPort returns the API server port.
func GetServerPort() int {
	return getInt(serverPort, 0)
}

// IsSSHEnable returns whether SSH access is enabled.
func IsSSHEnable() bool {
	return getBool(sshEnable, false)
}

// GetSSHServerIP returns the SSH server IP address.
func GetSSHServerIP() string {
	return getString(sshServerIP, "")
}

// GetSSHServerPort returns the SSH server port.
func GetSSHServerPort() int {
	return getInt(sshServerPort, 0)
}

// GetSSHRsaPublic returns the SSH RSA public key path.
func GetSSHRsaPublic() string {
	return getFromFile(sshSecretPath, "id_rsa.pub")
}

// GetSSHRsaPrivate returns the SSH RSA private key path.
func GetSSHRsaPrivate() string {
	return getFromFile(sshSecretPath, "id_rsa")
}

// IsSSHReverseForwardEnable returns whether SSH remote port forwarding (`ssh -R`)
// is enabled. On by default: it is what an SSH client already expects of a dev box,
// and what bounds it is the rest of the policy - loopback only, a port range, and a
// per-session limit - not the absence of the feature. A deployment that does not
// want pod traffic leaving through a developer's network turns it off here.
func IsSSHReverseForwardEnable() bool {
	return getBool(sshReverseForwardEnable, true)
}

// GetSSHReverseForwardBindAddresses returns the addresses a remote forward may
// listen on inside the target Pod.
func GetSSHReverseForwardBindAddresses() []string {
	return getStringSlice(sshReverseForwardBindAddresses, []string{"127.0.0.1"})
}

// GetSSHReverseForwardPortMin returns the lowest port a remote forward may bind.
// The default is the first unprivileged port: a forward that could take 22, 53 or
// 443 would shadow the pod's own services, while everything above that is the
// user's own business, as it would be on a dev box.
func GetSSHReverseForwardPortMin() int {
	return getInt(sshReverseForwardPortMin, 1024)
}

// GetSSHReverseForwardPortMax returns the highest port a remote forward may bind.
func GetSSHReverseForwardPortMax() int {
	return getInt(sshReverseForwardPortMax, 65535)
}

// GetSSHReverseForwardMaxPerSession returns how many remote forwards a single SSH
// session may hold at once.
func GetSSHReverseForwardMaxPerSession() int {
	return getInt(sshReverseForwardMaxPerSession, 8)
}

// GetMemoryReservePercent returns the percentage of memory to reserve.
func GetMemoryReservePercent() float64 {
	return getFloat(memoryReservePercent, 0)
}

// GetCpuReservePercent returns the percentage of CPU to reserve.
func GetCpuReservePercent() float64 {
	return getFloat(cpuReservePercent, 0)
}

// GetEphemeralStoreReservePercent returns the percentage of ephemeral storage to reserve.
func GetEphemeralStoreReservePercent() float64 {
	return getFloat(ephemeralStoreReservePercent, 0)
}

// GetMaxEphemeralStorePercent returns the maximum percentage of ephemeral storage allowed.
func GetMaxEphemeralStorePercent() float64 {
	return getFloat(maxEphemeralStorePercent, 0)
}

// GetWorkloadHangCheckInterval returns the interval for checking hung workloads.
func GetWorkloadHangCheckInterval() int {
	return getInt(workloadHangCheckInterval, 0)
}

// GetWorkloadTTLSecond returns the TTL in seconds for completed workloads.
func GetWorkloadTTLSecond() int {
	return getInt(workloadTTLSecond, 3600)
}

// IsOpenSearchEnable returns whether OpenSearch is enabled.
func IsOpenSearchEnable() bool {
	return getBool(openSearchEnable, false)
}

// GetOpenSearchEndpoint returns the OpenSearch endpoint URL.
func GetOpenSearchEndpoint() string {
	return getString(openSearchEndpoint, "")
}

// GetOpenSearchUser returns the OpenSearch username.
func GetOpenSearchUser() string {
	if user := getString(openSearchPrefix+openSearchUser, ""); len(user) > 0 {
		return user
	}
	return getFromFile(openSearchSecretPath, openSearchUser)
}

// GetOpenSearchPasswd returns the OpenSearch password.
func GetOpenSearchPasswd() string {
	if passwd := getString(openSearchPrefix+openSearchPassword, ""); len(passwd) > 0 {
		return passwd
	}
	return getFromFile(openSearchSecretPath, openSearchPassword)
}

// GetOpenSearchIndexPrefix returns the prefix for OpenSearch indices.
func GetOpenSearchIndexPrefix() string {
	return getString(openSearchIndexPrefix, "")
}

// IsDBEnable returns whether the database is enabled.
func IsDBEnable() bool {
	return getBool(dbEnable, false)
}

// GetDBHost returns the database host address.
func GetDBHost() string {
	return getFromFile(dbSecretPath, "host")
}

// GetDBPort returns the database port number.
func GetDBPort() int {
	data := getFromFile(dbSecretPath, "port")
	n, err := strconv.Atoi(data)
	if err != nil {
		return 0
	}
	return n
}

// GetDBName returns the database name.
func GetDBName() string {
	return getFromFile(dbSecretPath, "dbname")
}

// GetDBUser returns the database username.
func GetDBUser() string {
	return getFromFile(dbSecretPath, "user")
}

// GetDBPassword returns the database password.
func GetDBPassword() string {
	return getFromFile(dbSecretPath, "password")
}

// GetDBSslMode returns the database SSL mode.
func GetDBSslMode() string {
	return getString(dbSslMode, "require")
}

// GetDBTargetSessionAttrs returns the session the driver must be given, or an
// empty string to accept whichever it reaches.
//
// `read-write` makes the driver ask, at connect time, whether the session it
// just opened can write, and refuse it when it cannot. Without that a connection
// to a demoted replica is accepted and fails only later, on the first write, with
// SQLSTATE 25006 -- and every component here writes, so a read-only session is
// never one it can use.
//
// Configurable because it constrains what the address in the DB secret is allowed
// to resolve to: a deployment that deliberately points at a replica has to clear
// this, or every connection fails outright rather than serving reads.
func GetDBTargetSessionAttrs() string {
	return getString(dbTargetSessionAttrs, "read-write")
}

// GetDBMaxOpenConns returns the maximum number of open database connections.
func GetDBMaxOpenConns() int {
	return getInt(dbMaxOpenConns, 100)
}

// GetDBMaxIdleConns returns the maximum number of idle database connections.
func GetDBMaxIdleConns() int {
	return getInt(dbMaxIdleConns, 10)
}

// GetDBMaxLifetimeSecond returns the maximum lifetime of database connections in seconds.
func GetDBMaxLifetimeSecond() int {
	return getInt(dbMaxLifetime, 600)
}

// GetDBMaxIdleTimeSecond returns the maximum idle time of database connections in seconds.
func GetDBMaxIdleTimeSecond() int {
	return getInt(dbMaxIdleTimeSecond, 60)
}

// GetDBConnectTimeoutSecond returns the database connection timeout in seconds.
func GetDBConnectTimeoutSecond() int {
	return getInt(dbConnectTimeoutSecond, 10)
}

// GetDBRequestTimeoutSecond returns the database request timeout in seconds.
func GetDBRequestTimeoutSecond() int {
	return getInt(dbRequestTimeoutSecond, 20)
}

// GetOpsJobTTLSecond returns the TTL in seconds for operations jobs.
func GetOpsJobTTLSecond() int {
	return getInt(opsJobTTLSecond, 60)
}

// GetOpsJobTimeoutSecond returns the timeout in seconds for operations jobs.
func GetOpsJobTimeoutSecond() int {
	return getInt(opsJobTimeoutSecond, 0)
}

// GetDownloadJoImage returns the image name for downloading jobs.
func GetDownloadJoImage() string {
	return getString(opsJobDownloadImage, "docker.io/primussafe/s3-downloader:latest")
}

// GetEvalScopeImage returns the image name for evaluation jobs.
func GetEvalScopeImage() string {
	return getString(opsJobEvalImage, "docker.io/primussafe/evalscope:latest")
}

// GetPrewarmTimeoutSecond returns the timeout in seconds for prewarm jobs.
func GetPrewarmTimeoutSecond() int {
	return getInt(prewarmTimeoutSecond, 900)
}

// GetPrewarmWorkerConcurrent returns the number of concurrent workers for prewarm jobs.
func GetPrewarmWorkerConcurrent() int {
	return getInt(prewarmWorkerConcurrent, 10)
}

// GetExportImageTimeoutSecond returns the default timeout in seconds for saving a workload
// as an image: an export is designed to take up to 12 hours (a layer of up to 500 GiB), far
// longer than other ops jobs.
func GetExportImageTimeoutSecond() int {
	return getInt(exportImageTimeoutSecond, 43200)
}

// GetModelPrewarmTimeoutSecond returns the timeout in seconds for model prewarm jobs.
func GetModelPrewarmTimeoutSecond() int {
	return getInt(modelPrewarmTimeoutSecond, 7200)
}

// IsS3Enable returns whether S3 storage is enabled.
func IsS3Enable() bool {
	return getBool(s3Enable, false)
}

// GetS3AccessKey returns the S3 access key.
func GetS3AccessKey() string {
	return getFromFile(s3SecretPath, "access_key")
}

// GetS3SecretKey returns the S3 secret key.
func GetS3SecretKey() string {
	return getFromFile(s3SecretPath, "secret_key")
}

// GetS3Bucket returns the S3 bucket name.
func GetS3Bucket() string {
	return getFromFile(s3SecretPath, "bucket")
}

// GetS3Endpoint returns the S3 endpoint URL.
func GetS3Endpoint() string {
	return getFromFile(s3SecretPath, "endpoint")
}

// GetS3ExpireDay returns the number of days after which S3 objects expire.
func GetS3ExpireDay() int32 {
	resp := getInt(s3ExpireDay, 0)
	return int32(resp)
}

func getFromFile(configPath, item string) string {
	path := getString(configPath, "")
	data, err := os.ReadFile(filepath.Join(path, item))
	if err != nil {
		return ""
	}
	key := string(data)
	return strings.TrimRight(key, "\r\n")
}

// GetRdmaName returns the RDMA resource name.
func GetRdmaName() string {
	return getString(rdmaName, "")
}

// GetAddons returns the list of enabled addons.
func GetAddons(version *string) []string {
	name := addonPrefix
	if version != nil {
		name = fmt.Sprintf("%s-%s", name, *version)
	}
	addons := getStrings(name)
	if len(addons) > 0 {
		return addons
	}
	return getStrings(addonDefault)
}

// GetImageSecret returns the default image pull secret name.
func GetImageSecret() string {
	return getString(imageSecret, "")
}

// GetUserTokenExpire returns the user token expiration time in seconds.
func GetUserTokenExpire() int {
	return getInt(userTokenExpireSecond, -1)
}

// IsOutboundTLSVerifyEnabled reports whether outbound HTTPS requests to model
// endpoints and third-party services (Playground chat, InferenceX) must verify
// TLS certificates. It defaults to false to preserve compatibility with
// self-signed ingress certificates and container images that ship without an
// updated CA bundle. Security-conscious deployments can set
// tls.verify_outbound=true to enforce certificate verification.
func IsOutboundTLSVerifyEnabled() bool {
	return getBool(tlsVerifyOutbound, false)
}

// IsNotificationEnable returns whether notifications are enabled.
func IsNotificationEnable() bool {
	return getBool(notificationEnable, true)
}

// GetNotificationConfig returns the path to the notification configuration file.
func GetNotificationConfig() string {
	return getFromFile(notificationSecretPath, "config")
}

// GetSystemHost returns the host of the system. e.g. tw325.primus-safe.amd.com
func GetSystemHost() string {
	subDomainConfig := GetSubDomain()
	domainConfig := getString(domain, "")
	if subDomainConfig == "" || domainConfig == "" {
		return ""
	}
	return subDomainConfig + "." + domainConfig
}

func GetSubDomain() string {
	return getString(subDomain, "")
}

// GetIngress returns the ingress class name of the system.
func GetIngress() string {
	return getString(ingress, "")
}

func IsSSOEnable() bool {
	return getBool(ssoEnable, false)
}

// GetSSOBridgeAllowlist returns the origins (optionally with a path prefix) that the
// web login page may forward an SSO authorization code to. Empty means same origin only.
func GetSSOBridgeAllowlist() []string {
	return getStringSlice(ssoBridgeAllowlist, nil)
}

func GetSSOClientId() string {
	return getFromFile(ssoSecretPath, "id")
}

func GetSSOClientSecret() string {
	return getFromFile(ssoSecretPath, "secret")
}

func GetSSOEndpoint() string {
	return getFromFile(ssoSecretPath, "endpoint")
}

func GetSSORedirectURI() string {
	return getFromFile(ssoSecretPath, "redirect_uri")
}

func IsCICDEnable() bool {
	return getBool(cicdEnable, false)
}

func GetCICDRoleName() string {
	return getString(cicdRoleName, "")
}

func GetCICDControllerName() string {
	return getString(cicdControllerName, "")
}

// GetCICDNoProxy returns the cluster-wide proxy bypass list merged into every
// CI/CD workload's proxy configuration, for hosts a workload cannot know about
// (package mirrors, internal registries).
func GetCICDNoProxy() string {
	return getString(cicdNoProxy, "")
}

// GetCICDProxyRelayPort returns the loopback port the CI/CD proxy relay listens on.
func GetCICDProxyRelayPort() int {
	return getInt(cicdProxyRelayPort, 3129)
}

// GetCICDFailureEnrichTimeout bounds the ARC controller log lookup that enriches a
// failed CI/CD workload. Enrichment is effectively single-shot -- a failed lookup is
// swallowed, the key is forgotten and the per-dispatch dedup map blocks a re-enqueue
// -- so the budget must cover a cross-namespace OpenSearch query under load. It runs
// on a dedicated worker queue rather than a reconcile path, so waiting costs
// enrichment latency only; the interactive log handler uses 120s for the same query.
func GetCICDFailureEnrichTimeout() time.Duration {
	return time.Duration(getInt(cicdFailureEnrichTimeoutSec, 30)) * time.Second
}

// GetModelDownloaderImage returns the image for model downloader job.
// Used for downloading models from HuggingFace and uploading to S3.
func GetModelDownloaderImage() string {
	return getString(modelDownloaderImage, "docker.io/primussafe/model-downloader:latest")
}

// GetModelCleanupImage returns the image for model cleanup job.
// Used for deleting local model files.
func GetModelCleanupImage() string {
	return getString(modelCleanupImage, "docker.io/library/alpine:3.18")
}

// GetModelMaxConcurrentDownloads returns how many model download jobs may run at the
// same time across all workspaces: 4 when unset; 0 or a negative value means no limit.
func GetModelMaxConcurrentDownloads() int {
	return getInt(modelMaxDownloads, 4)
}

// GetModelDownloadTimeoutSecond returns the timeout of a single model download job.
func GetModelDownloadTimeoutSecond() int {
	return getInt(modelDownloadTimeout, 10800)
}

// GetProxyServices returns the list of configured proxy services.
func GetProxyServices() []ProxyService {
	var services []ProxyService
	if err := viper.UnmarshalKey(proxyList, &services); err != nil {
		return []ProxyService{}
	}
	return services
}

// HostAlias is one /etc/hosts entry put into every Pod of a cluster.
type HostAlias struct {
	IP        string   `json:"ip" yaml:"ip" mapstructure:"ip"`
	Hostnames []string `json:"hostnames" yaml:"hostnames" mapstructure:"hostnames"`
}

// SaveImageCluster says where a workload of one cluster is saved as an image.
//
// Registry is where the saved image is published; the cluster's containers must reach it,
// because each uploads its own changes there, to a repository of StagingProject made for
// that export, and the image is then put together in TargetProject by mounting blobs. It
// must hold the images the containers were started from. StagingPushSecret
// ("<namespace>/<name>") holds a credential limited to StagingProject, which the
// container's upload token is minted with; CASecret ("<namespace>/<name>", key ca.crt)
// signs Registry. HostAliases resolve registry names inside the cluster's Pods where its
// DNS does not.
type SaveImageCluster struct {
	Cluster           string      `json:"cluster" yaml:"cluster" mapstructure:"cluster"`
	Registry          string      `json:"registry" yaml:"registry" mapstructure:"registry"`
	TargetProject     string      `json:"target_project" yaml:"target_project" mapstructure:"target_project"`
	StagingProject    string      `json:"staging_project" yaml:"staging_project" mapstructure:"staging_project"`
	CASecret          string      `json:"ca_secret" yaml:"ca_secret" mapstructure:"ca_secret"`
	StagingPushSecret string      `json:"staging_push_secret" yaml:"staging_push_secret" mapstructure:"staging_push_secret"`
	HostAliases       []HostAlias `json:"host_aliases" yaml:"host_aliases" mapstructure:"host_aliases"`
}

// GetSaveImageCluster returns the save-image settings of a cluster, and whether it has
// any. Settings that cannot be read are an error, not an absence.
func GetSaveImageCluster(clusterID string) (SaveImageCluster, bool, error) {
	var clusters []SaveImageCluster
	if err := viper.UnmarshalKey(saveImageClusters, &clusters); err != nil {
		return SaveImageCluster{}, false, fmt.Errorf("reading %s: %w", saveImageClusters, err)
	}
	for _, c := range clusters {
		if c.Cluster == clusterID {
			return c, true, nil
		}
	}
	return SaveImageCluster{}, false, nil
}

// GetComponents returns the list of deployable components.
func GetComponents() []string {
	val := getString(cdComponents, "")
	return removeBlank(strings.Split(val, ","))
}

// IsCDRequireApproval returns whether CD deployment requires approval from another user.
// When true, users cannot approve their own deployment requests.
// When false, users can approve their own requests (self-approval allowed).
func IsCDRequireApproval() bool {
	return getBool(cdRequireApproval, true)
}

// GetTorchFTLightHouse returns the entorypoint of torchft lighthouse.
func GetTorchFTLightHouse() string {
	return getString(torchFTLightHouse, "")
}

// GetCDJobImage returns the image for CD deployment jobs.
func GetCDJobImage() string {
	return getString(cdJobImage, "docker.io/primussafe/cd-job-runner:latest")
}

// IsTracingEnable returns whether OpenTelemetry tracing is enabled.
func IsTracingEnable() bool {
	return getBool(tracingEnable, false)
}

// GetTracingMode returns the tracing mode: "all" or "error_only".
func GetTracingMode() string {
	return getString(tracingMode, "error_only")
}

// GetTracingSamplingRatio returns the sampling ratio for trace export (0.0 to 1.0).
func GetTracingSamplingRatio() float64 {
	return getFloat(tracingSamplingRatio, 1.0)
}

// GetTracingOtlpEndpoint returns the OTLP exporter endpoint URL.
func GetTracingOtlpEndpoint() string {
	return getString(tracingOtlpEndpoint, "")
}

// GetLangfuseProxyPublicKey returns the shared Langfuse project public key from secret.
func GetLangfuseProxyPublicKey() string {
	return getFromFile(langfuseProxySecretPath, "public_key")
}

// GetLangfuseProxySecretKey returns the shared Langfuse project secret key from secret.
func GetLangfuseProxySecretKey() string {
	return getFromFile(langfuseProxySecretPath, "secret_key")
}

// IsA2AScannerEnable returns whether the A2A service scanner is enabled.
func IsA2AScannerEnable() bool { return getBool(a2aScannerEnable, false) }

// GetA2AScannerInterval returns the A2A scanner interval in seconds.
func GetA2AScannerInterval() int {
	v := getInt(a2aScannerInterval, 60)
	if v <= 0 {
		return 60
	}
	return v
}

// GetA2AScannerNamespaces returns the namespaces to scan for A2A services.
func GetA2AScannerNamespaces() []string {
	return viper.GetStringSlice(a2aScannerNamespaces)
}

// GetA2AScannerLabelSelector returns the label selector for A2A service discovery.
func GetA2AScannerLabelSelector() string {
	v := getString(a2aScannerLabel, "")
	if v == "" {
		return "a2a.primus.io/enabled=true"
	}
	return v
}

// ── LLM Gateway ─────────────────────────────────────────────────────────

// IsLLMGatewayEnable returns whether LLM Gateway feature is enabled.
func IsLLMGatewayEnable() bool {
	return getBool(llmGatewayEnable, false)
}

// GetLLMGatewayEndpoint returns the LiteLLM Proxy endpoint URL (from secret file).
func GetLLMGatewayEndpoint() string {
	return getFromFile(llmGatewaySecretPath, "litellm_endpoint")
}

// GetLLMGatewayAdminKey returns the LiteLLM Master Key for admin API access (from secret file).
func GetLLMGatewayAdminKey() string {
	return getFromFile(llmGatewaySecretPath, "litellm_admin_key")
}

// GetLLMGatewayTeamID returns the global LiteLLM Team ID (from secret file).
func GetLLMGatewayTeamID() string {
	return getFromFile(llmGatewaySecretPath, "litellm_team_id")
}

// GetLLMGatewaySharedApimKeyHashes returns the ComputeApimKeyHash values of the
// APIM keys that several bindings share (from secret file, comma-separated).
func GetLLMGatewaySharedApimKeyHashes() []string {
	var hashes []string
	for _, hash := range strings.Split(getFromFile(llmGatewaySecretPath, "shared_apim_key_hashes"), ",") {
		if hash = strings.TrimSpace(hash); hash != "" {
			hashes = append(hashes, hash)
		}
	}
	return hashes
}

// GetLLMGatewaySharedApimUser returns the upstream user that requests made with a
// shared APIM key are attributed to (from secret file).
func GetLLMGatewaySharedApimUser() string {
	return strings.TrimSpace(getFromFile(llmGatewaySecretPath, "shared_apim_user"))
}

func IsMonarchEnable() bool {
	return getBool(monarchEnable, false)
}

func IsSandboxEnable() bool {
	return getBool(sandboxEnable, false)
}

func GetSandboxNamespace() string {
	return getString(sandboxNamespace, "")
}

func GetSandboxSecret() string {
	return getString(sandboxSecret, "")
}

// ── External execution capacity ─────────────────────────────────────────

// IsExternalExecutionEnable reports whether external execution capacity is admitted.
// Disabled by default: with the flag off, external objects are rejected at admission and
// no capacity demand is published, so a half-deployed integration cannot dispatch work.
func IsExternalExecutionEnable() bool {
	return getBool(externalExecutionEnabled, false)
}

// GetExternalObservationMaxAge caps how long a provider observation stays usable once the
// provider stops reporting, backstopping the validUntil the provider sets for itself.
func GetExternalObservationMaxAge() time.Duration {
	return time.Duration(getInt(externalExecutionObservationMaxAge, 120)) * time.Second
}

// GetExternalWorkspaceResync bounds how long an external workspace goes without a
// reconcile. The controller's own fifteen minute backstop would let a stale provider
// observation stay counted as available capacity for that long.
func GetExternalWorkspaceResync() time.Duration {
	return time.Duration(getInt(externalExecutionWorkspaceResync, 30)) * time.Second
}

// ── MCP (Model Context Protocol) ────────────────────────────────────────

func IsMCPEnable() bool {
	return getBool(mcpEnable, false)
}

func GetMCPBasePath() string {
	return getString(mcpBasePath, "/api/v1/safe-mcp/mcp")
}

func GetMCPInstructions() string {
	return getString(mcpInstructions, "")
}

func GetMCPAllowedOrigins() []string {
	return getStrings(mcpAllowedOrigins)
}

func GetMonarchClientRole() string {
	return getString(monarchClientRole, "")
}

// ── Model Optimization (Hyperloom via PrimusClaw) ───────────────────────

// IsModelOptimizationEnable reports whether the Model Optimization feature
// (which orchestrates Hyperloom runs on PrimusClaw) should register its
// REST + SSE routes and database-backed task store.
func IsModelOptimizationEnable() bool {
	return getBool(modelOptimizationEnable, false)
}

// GetModelOptimizationClawBaseURL returns the base URL used to reach the
// PrimusClaw backend, e.g. "https://core42.primus-safe.amd.com/claw-api/v1".
// Precedence: direct config key > secret file "claw_base_url" > derived from global.sub_domain + global.domain.
func GetModelOptimizationClawBaseURL() string {
	if v := getString(modelOptimizationClawBaseURL, ""); v != "" {
		return v
	}
	if v := getFromFile(modelOptimizationSecretPath, "claw_base_url"); v != "" {
		return v
	}
	d := getString(domain, "")
	if d == "" {
		return ""
	}
	sub := getString(subDomain, "")
	if sub != "" {
		return "https://" + sub + "." + d + "/claw-api/v1"
	}
	return "https://" + d + "/claw-api/v1"
}

// GetModelOptimizationClawAPIKey returns the bearer token used to authenticate
// against PrimusClaw. Stored as a secret file entry "claw_api_key".
func GetModelOptimizationClawAPIKey() string {
	return getFromFile(modelOptimizationSecretPath, "claw_api_key")
}

// GetModelOptimizationClawAgentID returns the default Claw agent identifier.
// Defaults to "agent_default" to match the Claw backend default.
func GetModelOptimizationClawAgentID() string {
	if v := getString(modelOptimizationClawAgentID, ""); v != "" {
		return v
	}
	return "agent_default"
}

// GetModelOptimizationDefaultWorkspace returns the Claw workspace used for
// sandbox scheduling when the client request omits a workspace hint.
func GetModelOptimizationDefaultWorkspace() string {
	return getString(modelOptimizationDefaultWS, "control-plane-sandbox")
}

// GetModelOptimizationMaxConcurrent returns the soft limit on how many
// optimization tasks a single workspace can have running simultaneously.
// Non-positive values disable the limit.
func GetModelOptimizationMaxConcurrent() int {
	return getInt(modelOptimizationConcurrency, 1024)
}

// GetModelOptimizationClawPluginID returns the Claw plugin ID for the
// Hyperloom GPU plugin. When set, the optimization handler attaches this
// plugin_id to outgoing messages so that Claw resolves resource_gpu
// (image, cpu, memory, GPU count) from the plugin definition instead of
// requiring the caller to pass sandbox_image / resource_gpu explicitly.
// Returns 0 when unconfigured (plugin attachment is skipped).
func GetModelOptimizationClawPluginID() int {
	return getInt(modelOptimizationClawPluginID, 4)
}

// Authentication modes (auth.mode).
const (
	// AuthModeBuiltin is the default: local accounts, built-in sessions, SSO login and API keys.
	AuthModeBuiltin = "builtin"
	// AuthModeExternal authenticates requests with the configured authenticator chain.
	AuthModeExternal = "external"
)

// AuthenticatorConfig is one entry of auth.authenticators. Type selects the
// mechanism ("oidc", "introspection" or "builtin"); the other fields apply to one type each.
// Durations are Go duration strings such as "30s" or "30m".
type AuthenticatorConfig struct {
	Type string `json:"type" yaml:"type" mapstructure:"type"`
	Name string `json:"name" yaml:"name" mapstructure:"name"`

	// oidc: validate JWTs from one issuer. introspection: Issuer also names the issuer whose
	// discovery document gives the endpoint (when URL is empty) and whose tokens are accepted.
	Issuer        string `json:"issuer" yaml:"issuer" mapstructure:"issuer"`
	Audience      string `json:"audience" yaml:"audience" mapstructure:"audience"`
	JWKSURL       string `json:"jwks_url" yaml:"jwks_url" mapstructure:"jwks_url"`
	JWKSCacheTTL  string `json:"jwks_cache_ttl" yaml:"jwks_cache_ttl" mapstructure:"jwks_cache_ttl"`
	ClockSkew     string `json:"clock_skew" yaml:"clock_skew" mapstructure:"clock_skew"`
	IDClaim       string `json:"id_claim" yaml:"id_claim" mapstructure:"id_claim"`
	UsernameClaim string `json:"username_claim" yaml:"username_claim" mapstructure:"username_claim"`
	EmailClaim    string `json:"email_claim" yaml:"email_claim" mapstructure:"email_claim"`
	GroupsClaim   string `json:"groups_claim" yaml:"groups_claim" mapstructure:"groups_claim"`

	// introspection: OAuth 2.0 token introspection (RFC 7662) at URL, authenticated with a client id
	// and a client secret read from a file (a mounted Secret).
	URL              string `json:"url" yaml:"url" mapstructure:"url"`
	CABundle         string `json:"ca_bundle" yaml:"ca_bundle" mapstructure:"ca_bundle"` // PEM text or a file path
	ClientID         string `json:"client_id" yaml:"client_id" mapstructure:"client_id"`
	ClientSecretFile string `json:"client_secret_file" yaml:"client_secret_file" mapstructure:"client_secret_file"`
	Timeout          string `json:"timeout" yaml:"timeout" mapstructure:"timeout"`
	CacheTTL         string `json:"cache_ttl" yaml:"cache_ttl" mapstructure:"cache_ttl"`
	CacheNegativeTTL string `json:"cache_negative_ttl" yaml:"cache_negative_ttl" mapstructure:"cache_negative_ttl"`
	StaleIfError     string `json:"stale_if_error" yaml:"stale_if_error" mapstructure:"stale_if_error"`

	// introspection: Audience (above) is required and is what the answer's "aud", or its
	// "client_id" when it has no "aud", must name. AllowMissingAudience accepts an answer
	// that names none, such as an API key's; an answer that names another audience is refused.
	AllowMissingAudience bool `json:"allow_missing_audience" yaml:"allow_missing_audience" mapstructure:"allow_missing_audience"`
}

// AuthGroupMapping maps an identity group to SaFE roles, evaluated on every request.
// Workspaces, when set, scope the mapping: the user gets access to them, and the
// "workspace-admin" role means managing them.
type AuthGroupMapping struct {
	Group      string   `json:"group" yaml:"group" mapstructure:"group"`
	Roles      []string `json:"roles" yaml:"roles" mapstructure:"roles"`
	Workspaces []string `json:"workspaces" yaml:"workspaces" mapstructure:"workspaces"`
}

// GetAuthMode returns auth.mode: "builtin" (default) or "external". Any other value
// is treated as builtin, so a typo never switches authentication off.
func GetAuthMode() string {
	if getString(authMode, AuthModeBuiltin) == AuthModeExternal {
		return AuthModeExternal
	}
	return AuthModeBuiltin
}

// GetAuthenticators returns the authenticator chain, in order. A chain that cannot be
// read is an error: the caller must not fall back to another mode.
func GetAuthenticators() ([]AuthenticatorConfig, error) {
	var list []AuthenticatorConfig
	if err := viper.UnmarshalKey(authAuthenticators, &list); err != nil {
		return nil, fmt.Errorf("reading %s: %w", authAuthenticators, err)
	}
	return list, nil
}

// GetAuthGroupMappings returns the group to role mappings.
func GetAuthGroupMappings() ([]AuthGroupMapping, error) {
	var list []AuthGroupMapping
	if err := viper.UnmarshalKey(authGroupMappings, &list); err != nil {
		return nil, fmt.Errorf("reading %s: %w", authGroupMappings, err)
	}
	return list, nil
}

// GetAuthLoginURL returns auth.login_url, where the console sends users to sign in when
// authentication is external.
func GetAuthLoginURL() string { return getString(authLoginURL, "") }
