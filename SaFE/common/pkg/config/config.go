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

	"github.com/spf13/viper"
)

// ProxyService represents a proxy service configuration
type ProxyService struct {
	Name    string `json:"name" yaml:"name"`       // Service name
	Prefix  string `json:"prefix" yaml:"prefix"`   // URL prefix for the proxy route
	Target  string `json:"target" yaml:"target"`   // Target service URL
	Enabled bool   `json:"enabled" yaml:"enabled"` // Whether the proxy is enabled
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
	return getInt(workloadTTLSecond, 60)
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

// GetPrewarmTimeoutSecond returns the timeout in seconds for prewarm jobs.
func GetPrewarmTimeoutSecond() int {
	return getInt(prewarmTimeoutSecond, 900)
}

// GetPrewarmWorkerConcurrent returns the number of concurrent workers for prewarm jobs.
func GetPrewarmWorkerConcurrent() int {
	return getInt(prewarmWorkerConcurrent, 10)
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

// IsUserTokenRequired returns whether user token is required for API access.
func IsUserTokenRequired() bool {
	return getBool(userTokenRequired, true)
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
	subDomainConfig := getString(subDomain, "")
	domainConfig := getString(domain, "")
	if subDomainConfig == "" || domainConfig == "" {
		return ""
	}
	return subDomainConfig + "." + domainConfig
}

// GetIngress returns the ingress class name of the system.
func GetIngress() string {
	return getString(ingress, "")
}

func IsSSOEnable() bool {
	return getBool(ssoEnable, false)
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

func GetCICDControllerNamespace() string {
	return getString(cicdControllerNamespace, "")
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

// GetProxyServices returns the list of configured proxy services.
func GetProxyServices() []ProxyService {
	var services []ProxyService
	if err := viper.UnmarshalKey(proxyList, &services); err != nil {
		return []ProxyService{}
	}
	return services
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
