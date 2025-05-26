/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
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
