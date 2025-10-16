/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

const (
	// global
	globalPrefix = "global."
	rdmaName     = globalPrefix + "rdma_name"
	imageSecret  = globalPrefix + "image_secret"

	// crypto
	cryptoPrefix     = "crypto."
	cryptoEnable     = cryptoPrefix + "enable"
	cryptoSecretPath = cryptoPrefix + "secret_path"

	// server
	serverPrefix = "server."
	serverPort   = serverPrefix + "port"

	// ssh
	sshPrefix     = "ssh."
	sshEnable     = sshPrefix + "enable"
	sshServerPort = sshPrefix + "server_port"
	sshSecretPath = sshPrefix + "secret_path"

	// health_check
	healthCheckPrefix = "health_check."
	healthCheckEnable = healthCheckPrefix + "enable"
	healthCheckPort   = healthCheckPrefix + "port"

	// leader_election
	leaderElectionPrefix = "leader_election."
	leaderElectionEnable = leaderElectionPrefix + "enable"

	// workspace
	workspacePrefix              = "workspace."
	memoryReservePercent         = workspacePrefix + "mem_reserve_percent"
	cpuReservePercent            = workspacePrefix + "cpu_reserve_percent"
	ephemeralStoreReservePercent = workspacePrefix + "ephemeral_store_reserve_percent"

	// workload
	workloadPrefix = "workload."
	// The maximum percentage of total local storage that a single task can allocate
	// No configuration or a value of 0 indicates no limit.
	maxEphemeralStorePercent  = workloadPrefix + "max_ephemeral_store_percent"
	workloadHangCheckInterval = workloadPrefix + "hang_check_interval"
	workloadTTLSecond         = workloadPrefix + "ttl_second"
	workloadAuthoringImage    = workloadPrefix + "authoring_image"

	// opensearch
	openSearchPrefix      = "opensearch."
	openSearchEnable      = openSearchPrefix + "enable"
	openSearchSecretPath  = openSearchPrefix + "secret_path"
	openSearchEndpoint    = openSearchPrefix + "endpoint"
	openSearchUser        = "username"
	openSearchPassword    = "password"
	openSearchIndexPrefix = openSearchPrefix + "prefix"

	// db
	dbPrefix               = "db."
	dbEnable               = dbPrefix + "enable"
	dbSecretPath           = dbPrefix + "secret_path"
	dbSslMode              = dbPrefix + "ssl_mode"
	dbMaxOpenConns         = dbPrefix + "max_open_conns"
	dbMaxIdleConns         = dbPrefix + "max_idle_conns"
	dbMaxLifetime          = dbPrefix + "max_life_time_second"
	dbMaxIdleTimeSecond    = dbPrefix + "max_idle_time_second"
	dbConnectTimeoutSecond = dbPrefix + "connect_timeout_second"
	dbRequestTimeoutSecond = dbPrefix + "request_timeout_second"

	// ops_job
	opsJobPrefix        = "ops_job."
	opsJobTTLSecond     = opsJobPrefix + "ttl_second"
	opsJobTimeoutSecond = opsJobPrefix + "timeout_second"

	// s3
	s3Prefix     = "s3."
	s3Enable     = s3Prefix + "enable"
	s3SecretPath = s3Prefix + "secret_path"
	s3Endpoint   = s3Prefix + "endpoint"
	s3Bucket     = s3Prefix + "bucket"
	s3AccessKey  = "access_key"
	s3SecretKey  = "secret_key"
	s3ExpireDay  = s3Prefix + "expire_day"

	// addon
	addonPrefix  = "addon."
	addonDefault = addonPrefix + "default"

	// user
	userPrefix            = "user."
	userTokenRequired     = userPrefix + "token_required"
	userTokenExpireSecond = userPrefix + "token_expire"
)
