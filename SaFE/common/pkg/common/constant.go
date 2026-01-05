/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package common

const (
	PrimusSafeName             = "primus-safe"
	PrimusSafeNamespace        = "primus-safe"
	PrimusFault                = "primus-safe-fault"
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

	PodKind            = "Pod"
	EventKind          = "Event"
	ConfigmapKind      = "ConfigMap"
	ClusterRoleKind    = "ClusterRole"
	ServiceAccountKind = "ServiceAccount"

	GithubConfigUrl  = "GITHUB_CONFIG_URL"
	UnifiedJobEnable = "UNIFIED_JOB_ENABLE"
	ScaleRunnerSetID = "SCALE_RUNNER_SET_ID"
	ScaleRunnerID    = "SCALE_RUNNER_ID"
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
	SSHPortName            = "ssh-port"
	JsonContentType        = "application/json; charset=utf-8"
	KubernetesControlPlane = "node-role.kubernetes.io/control-plane"
	ExcludedNodes          = "excluded-nodes"

	DefaultBurst = 1000
	DefaultQPS   = 1000

	AddonMonitorId     = "501"
	PreflightMonitorId = "502"

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

type InferenceModelForm string

const (
	// InferenceModelFormAPI represents models imported via API
	InferenceModelFormAPI InferenceModelForm = "API"
	// InferenceModelFormModelSquare represents models from model-square
	InferenceModelFormModelSquare InferenceModelForm = "ModelSquare"
)

// InferencePhaseType represents the phase of an inference service
type InferencePhaseType string

const (
	// InferencePhasePending represents the inference service is pending
	InferencePhasePending InferencePhaseType = "Pending"
	// InferencePhaseRunning represents the inference service is running (this is the normal state for inference services)
	InferencePhaseRunning InferencePhaseType = "Running"
	// InferencePhaseFailure represents the inference service failed (terminal state, will be deleted)
	InferencePhaseFailure InferencePhaseType = "Failure"
	// InferencePhaseStopped represents the inference service is stopped (terminal state, will stop workload and delete)
	InferencePhaseStopped InferencePhaseType = "Stopped"
)
