/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package common

const (
	PrimusSafeName             = "primus-safe"
	PrimusSafeNamespace        = "primus-safe"
	PrimusFault                = "primus-safe-fault"
	PrimusPvmName              = "primus-safe-pv"
	PrimusFailover             = "primus-safe-failover"
	DefaultVersion             = "v1"
	PrimusRouterCustomRootPath = "api/" + DefaultVersion
	ImageImportSecretName      = "primus-safe-image-import-reg-cred"
	SecretPath                 = "/etc/secrets"

	AuthoringKind           = "Authoring"
	PytorchJobKind          = "PyTorchJob"
	JobKind                 = "Job"
	DeploymentKind          = "Deployment"
	StatefulSetKind         = "StatefulSet"
	CICDScaleRunnerSetKind  = "AutoscalingRunnerSet"
	CICDEphemeralRunnerKind = "EphemeralRunner"
	UnifiedJobKind          = "UnifiedJob"
	TorchFTKind             = "TorchFT"
	MonarchJob              = "MonarchJob"
	MonarchMesh             = "MonarchMesh"
	MonarchClient           = "MonarchClient"
	RayJobKind              = "RayJob"
	PodKind                 = "Pod"
	ConfigmapKind           = "ConfigMap"
	ClusterRoleKind         = "ClusterRole"
	ServiceAccountKind      = "ServiceAccount"

	GithubConfigUrl   = "GITHUB_CONFIG_URL"
	UnifiedJobEnable  = "UNIFIED_JOB_ENABLE"
	ScaleRunnerSetID  = "SCALE_RUNNER_SET_ID"
	ScaleRunnerID     = "SCALE_RUNNER_ID"
	ReplicaCount      = "REPLICA_COUNT"
	HostPerReplica    = "HOST_PER_REPLICA"
	MaxReplicaCount   = "MAX_REPLICA_COUNT"
	MinReplicaCount   = "MIN_REPLICA_COUNT"
	TorchFTLightHouse = "TORCHFT_LIGHTHOUSE"
	RayJobEntrypoint  = "RAY_JOB_ENTRYPOINT"
	// for preflight job
	GPU_PRODUCT = "GPU_PRODUCT"

	HigressClassname = "higress"
	HigressNamespace = "higress-system"
	HigressGateway   = "higress-gateway"
	HigressSSHPort   = 22

	NodeNameSelector       = "spec.nodeName="
	KubeSystemNamespace    = "kube-system"
	KubePublicNamespace    = "kube-public"
	PytorchJobPortName     = "pytorchjob-port"
	PfsSelectorKey         = "pfs-name"
	JsonContentType        = "application/json; charset=utf-8"
	KubernetesControlPlane = "node-role.kubernetes.io/control-plane"
	SpecifiedNodes         = "specified-nodes"
	ExcludedNodes          = "excluded-nodes"
	TaintAction            = "taint"
	CICDArcNamespace       = "arc-systems"

	RayJobSubmitterName    = "ray-job-submitter"
	RayJobSubmitterCpu     = "1"
	RayJobSubmitterMemory  = "1Gi"
	RayJobSubmitterStorage = "10Gi"
	RayJobGcsServerPort    = 6379
	RayJobDashboard        = 8265

	MonarchMeshPortNum = 26600
	MonarchPort        = "MONARCH_PORT"
	MonarchMeshPrefix  = "MONARCH_MESH_PREFIX"

	DefaultBurst          = 1000
	DefaultQPS            = 1000
	DefaultMaxUnavailable = "25%"
	DefaultMaxMaxSurge    = "25%"

	HighPriority    = "high-priority"
	MedPriority     = "med-priority"
	LowPriority     = "low-priority"
	HighPriorityInt = 2
	MedPriorityInt  = 1
	LowPriorityInt  = 0

	NvidiaGpu            = "nvidia.com/gpu"
	NvidiaIdentification = "nvidia.com/gpu.present"
	AMDGpuIdentification = "feature.node.kubernetes.io/amd-gpu"
	AmdGpu               = "amd.com/gpu"

	UserName              = "userName"
	UserId                = "userId"
	UserType              = "userType"
	ApiKeyId              = "apiKeyId"
	UserSelf              = "self"
	UserSystem            = "primus-safe-system"
	UserWorkspaces        = "workspaces"
	UserManagedWorkspaces = "managedWorkspaces"
	Name                  = "name"
	PodId                 = "podId"
	AddonName             = "addonName"
)

const (
	ImageImportReadyStatus   = "Ready"
	ImageImportingStatus     = "Importing"
	ImageImportFailedStatus  = "Failed"
	ImageImportPendingStatus = "Pending"
)

const (
	NodesAffinityRequired  = "required"
	NodesAffinityPreferred = "preferred"
)
