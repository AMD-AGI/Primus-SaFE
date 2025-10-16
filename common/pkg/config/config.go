/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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

func SetValue(key, value string) {
	viper.Set(key, value)
}

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

func IsCryptoEnable() bool {
	return getBool(cryptoEnable, true)
}

func GetCryptoKey() string {
	return getFromFile(cryptoSecretPath, "key")
}

func IsHealthCheckEnabled() bool {
	return getBool(healthCheckEnable, true)
}

func GetHealthCheckPort() int {
	return getInt(healthCheckPort, 0)
}

func IsLeaderElectionEnable() bool {
	return getBool(leaderElectionEnable, true)
}

func GetServerPort() int {
	return getInt(serverPort, 0)
}

func IsSSHEnable() bool {
	return getBool(sshEnable, false)
}

func GetSSHServerPort() int {
	return getInt(sshServerPort, 0)
}

func GetSSHRsaPublic() string {
	return getFromFile(sshSecretPath, "id_rsa.pub")
}

func GetSSHRsaPrivate() string {
	return getFromFile(sshSecretPath, "id_rsa")
}

func GetMemoryReservePercent() float64 {
	return getFloat(memoryReservePercent, 0)
}

func GetCpuReservePercent() float64 {
	return getFloat(cpuReservePercent, 0)
}

func GetEphemeralStoreReservePercent() float64 {
	return getFloat(ephemeralStoreReservePercent, 0)
}

func GetMaxEphemeralStorePercent() float64 {
	return getFloat(maxEphemeralStorePercent, 0)
}

func GetWorkloadHangCheckInterval() int {
	return getInt(workloadHangCheckInterval, 0)
}

func GetWorkloadTTLSecond() int {
	return getInt(workloadTTLSecond, 60)
}

func IsOpenSearchEnable() bool {
	return getBool(openSearchEnable, false)
}

func GetOpenSearchEndpoint() string {
	return getString(openSearchEndpoint, "")
}

func GetOpenSearchUser() string {
	if user := getString(openSearchPrefix+openSearchUser, ""); len(user) > 0 {
		return user
	}
	return getFromFile(openSearchSecretPath, openSearchUser)
}

func GetOpenSearchPasswd() string {
	if passwd := getString(openSearchPrefix+openSearchPassword, ""); len(passwd) > 0 {
		return passwd
	}
	return getFromFile(openSearchSecretPath, openSearchPassword)
}

func GetOpenSearchIndexPrefix() string {
	return getString(openSearchIndexPrefix, "")
}

func IsDBEnable() bool {
	return getBool(dbEnable, false)
}

func GetDBHost() string {
	return getFromFile(dbSecretPath, "host")
}

func GetDBPort() int {
	data := getFromFile(dbSecretPath, "port")
	n, err := strconv.Atoi(data)
	if err != nil {
		return 0
	}
	return n
}

func GetDBName() string {
	return getFromFile(dbSecretPath, "dbname")
}

func GetDBUser() string {
	return getFromFile(dbSecretPath, "user")
}

func GetDBPassword() string {
	return getFromFile(dbSecretPath, "password")
}

func GetDBSslMode() string {
	return getString(dbSslMode, "require")
}

func GetDBMaxOpenConns() int {
	return getInt(dbMaxOpenConns, 100)
}

func GetDBMaxIdleConns() int {
	return getInt(dbMaxIdleConns, 10)
}

func GetDBMaxLifetimeSecond() int {
	return getInt(dbMaxLifetime, 600)
}

func GetDBMaxIdleTimeSecond() int {
	return getInt(dbMaxIdleTimeSecond, 60)
}

func GetDBConnectTimeoutSecond() int {
	return getInt(dbConnectTimeoutSecond, 10)
}

func GetDBRequestTimeoutSecond() int {
	return getInt(dbRequestTimeoutSecond, 20)
}

func GetOpsJobTTLSecond() int {
	return getInt(opsJobTTLSecond, 60)
}

func GetOpsJobTimeoutSecond() int {
	return getInt(opsJobTimeoutSecond, 0)
}

func IsS3Enable() bool {
	return getBool(s3Enable, false)
}

func GetS3AccessKey() string {
	if ak := getString(s3Prefix+s3AccessKey, ""); ak != "" {
		return ak
	}
	return getFromFile(s3SecretPath, s3AccessKey)
}

func GetS3SecretKey() string {
	if sk := getString(s3Prefix+s3SecretKey, ""); sk != "" {
		return sk
	}
	return getFromFile(s3SecretPath, s3SecretKey)
}

func GetS3Bucket() string {
	return getString(s3Bucket, "")
}

func GetS3Endpoint() string {
	return getString(s3Endpoint, "")
}

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

func GetRdmaName() string {
	return getString(rdmaName, "")
}

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

func GetImageSecret() string {
	return getString(imageSecret, "")
}

func GetAuthoringImage() string {
	return getString(workloadAuthoringImage, "")
}

func GetUserTokenExpire() int {
	return getInt(userTokenExpireSecond, -1)
}

func IsUserTokenRequired() bool {
	return getBool(userTokenRequired, true)
}
