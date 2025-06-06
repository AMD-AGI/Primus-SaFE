/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
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
	return getString(cryptoKey, "")
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

func GetLeaderElectionLock() string {
	return getString(leaderElectionLock, "default")
}

func GetServerPort() int {
	return getInt(serverPort, 0)
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

func GetWorkloadHangCheckSecond() int {
	return getInt(workloadHangCheckSecond, 0)
}

func IsWorkloadFailoverEnable() bool {
	return getBool(workloadEnableFailover, true)
}

func IsLogEnable() bool {
	return getBool(logEnable, false)
}

func GetLogServiceEndpoint() string {
	return getString(logEndpoint, "")
}

func GetLogServiceUser() string {
	return getFromFile(logConfigPath, "username")
}

func GetLogServicePasswd() string {
	return getFromFile(logConfigPath, "password")
}

func GetLogServicePrefix() string {
	return getString(logServicePrefix, "")
}

func IsDBEnable() bool {
	return getBool(dbEnable, false)
}

func GetDBHost() string {
	return getFromFile(dbConfigPath, "host")
}

func GetDBPort() int {
	data := getFromFile(dbConfigPath, "port")
	n, err := strconv.Atoi(data)
	if err != nil {
		return 0
	}
	return n
}

func GetDBName() string {
	return getFromFile(dbConfigPath, "dbname")
}

func GetDBUser() string {
	return getFromFile(dbConfigPath, "user")
}

func GetDBPassword() string {
	return getFromFile(dbConfigPath, "password")
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

func getFromFile(configPath, item string) string {
	path := getString(configPath, "")
	data, err := os.ReadFile(filepath.Join(path, item))
	if err != nil {
		return ""
	}
	return string(data)
}

func GetRdmaName() string {
	return getString(rdmaName, "")
}
