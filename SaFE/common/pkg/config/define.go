/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

const (
	// global
	globalPrefix = "global."
	imageSecret  = globalPrefix + "image_secret"
	domain       = globalPrefix + "domain"
	subDomain    = globalPrefix + "sub_domain"

	netPrefix = "net."
	rdmaName  = netPrefix + "rdma_name"
	ingress   = netPrefix + "ingress"

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
	sshServerIP   = sshPrefix + "server_ip"
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
	opsJobDownloadImage = opsJobPrefix + "download_image"

	// prewarm
	prewarmPrefix           = opsJobPrefix + "prewarm."
	prewarmTimeoutSecond    = prewarmPrefix + "timeout_second"
	prewarmWorkerConcurrent = prewarmPrefix + "worker_concurrent"

	// s3
	s3Prefix     = "s3."
	s3Enable     = s3Prefix + "enable"
	s3SecretPath = s3Prefix + "secret_path"
	s3ExpireDay  = s3Prefix + "expire_day"

	// addon
	addonPrefix  = "addon."
	addonDefault = addonPrefix + "default"

	// user
	userPrefix            = "user."
	userTokenRequired     = userPrefix + "token_required"
	userTokenExpireSecond = userPrefix + "token_expire"

	// notification
	notificationPrefix     = "notification."
	notificationEnable     = notificationPrefix + "enable"
	notificationSecretPath = notificationPrefix + "secret_path"

	// sso
	ssoPrefix     = "sso."
	ssoEnable     = ssoPrefix + "enable"
	ssoSecretPath = ssoPrefix + "secret_path"

	// cicd
	cicdPrefix              = "cicd."
	cicdEnable              = cicdPrefix + "enable"
	cicdRoleName            = cicdPrefix + "role_name"
	cicdControllerName      = cicdPrefix + "controller_name"
	cicdControllerNamespace = cicdPrefix + "controller_namespace"

	// model
	modelPrefix          = "model."
	modelDownloaderImage = modelPrefix + "downloader_image"
	modelCleanupImage    = modelPrefix + "cleanup_image"

	// proxy
	proxyPrefix = "proxy."
	proxyList   = proxyPrefix + "services"

	// cd
	cdPrefix          = "cd."
	cdRequireApproval = cdPrefix + "require_approval"
	// Deployable components for CD
	cdComponents = cdPrefix + "components"

	// torchft
	torchftPrefix     = "torchft."
	torchFTLightHouse = torchftPrefix + "lighthouse"
)
