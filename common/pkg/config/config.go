/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package config

import (
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
	if viper.IsSet(key) {
		return viper.GetString(key)
	} else {
		return defaultValue
	}
}

func getBool(key string, defaultValue bool) bool {
	if viper.IsSet(key) {
		return viper.GetBool(key)
	} else {
		return defaultValue
	}
}

func getInt(key string, defaultValue int) int {
	if viper.IsSet(key) {
		return viper.GetInt(key)
	} else {
		return defaultValue
	}
}

func getFloat(key string, defaultValue float64) float64 {
	if !viper.IsSet(key) {
		return defaultValue
	}
	str := getString(key, "")
	if str == "" {
		return 0
	}
	f, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0
	}
	return f
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

func GetLogServiceHost() string {
	return getString(logServiceHost, "")
}

func GetLogServicePort() int {
	return getInt(logServicePort, 9200)
}

func GetLogServiceUser() string {
	return getString(logServiceUser, "")
}

func GetLogServicePasswd() string {
	return getString(logServicePasswd, "")
}

func GetLogServicePrefix() string {
	return getString(logServicePrefix, "")
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

func GetS3Configs() map[string]any {
	return viper.GetStringMap("s3")
}

func GetS3Namespace() string {
	return getString(s3Namespace, "")
}

func GetS3Secret() string {
	return getString(s3Secret, "")
}

func GetS3Service() string {
	return getString(s3Service, "")
}

func GetS3Endpoint() string {
	return getString(s3Endpoint, "")
}

func GetS3Clusters() string {
	return getString(s3Clusters, "")
}

func GetS3DefaultBucket() string {
	return getString(s3DefaultBucket, "")
}

func GetS3BucketRegion() string {
	return getString(s3BucketRegion, "")
}

func GetS3ExpireDays() int {
	return getInt(s3ExpireDays, 1)
}

func GetS3Timeout() int {
	return getInt(s3Timeout, 0)
}

func GetS3DefaultLlmModelBucket() string {
	return getString(s3DefaultLlmModelBucket, "")
}

func GetS3DefaultDataSetBucket() string {
	return getString(s3DefaultDataSetBucket, "dataset")
}

func IsS3Enable() bool {
	return getBool(s3Enable, false)
}
