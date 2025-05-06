/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package config

const (
	// global
	globalPrefix = "global."
	cryptoEnable = globalPrefix + "enable_crypto"

	// log
	logPrefix        = "log."
	logEnable        = logPrefix + "enable"
	logServiceHost   = logPrefix + "host"
	logServicePort   = logPrefix + "port"
	logServicePrefix = logPrefix + "prefix"
	logServiceUser   = logPrefix + "user"
	logServicePasswd = logPrefix + "password"

	// workload
	workloadPrefix         = "workload."
	workloadEnableFailover = workloadPrefix + "enable_failover"
	// Maximum percentage of local storage a single task can request. No limit if not set or set to 0
	maxEphemeralStorePercent = workloadPrefix + "max_ephemeral_store_percent"

	// workspace
	workspacePrefix = "workspace."
	// The reservation ratio defaults to 0 and only affects the configurations listed below.
	memoryReservePercent         = workspacePrefix + "mem_reserve_percent"
	cpuReservePercent            = workspacePrefix + "cpu_reserve_percent"
	ephemeralStoreReservePercent = workspacePrefix + "ephemeral_store_reserve_percent"

	// s3
	s3Prefix                = "s3."
	s3Namespace             = s3Prefix + "namespace"
	s3Secret                = s3Prefix + "secret"
	s3Service               = s3Prefix + "service"
	s3Endpoint              = s3Prefix + "endpoint"
	s3DefaultBucket         = s3Prefix + "default_bucket"
	s3ExpireDays            = s3Prefix + "expire_days"
	s3Timeout               = s3Prefix + "timeout"
	s3DefaultLlmModelBucket = s3Prefix + "default_llm_model_bucket"
	s3BucketRegion          = s3Prefix + "bucket_region"
	s3DefaultDataSetBucket  = s3Prefix + "default_dataset_bucket"
	s3Enable                = s3Prefix + "enable"
	s3Clusters              = s3Prefix + "clusters"
)
