/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package common

const (
	PrimusSafeNamespace        = "primus-safe"
	PrimusFault                = "primus-safe-fault"
	PrimusFailover             = "primus-safe-failover"
	PrimusRouterCustomRootPath = "api/v1"

	AuthoringKind   = "Authoring"
	PytorchJobKind  = "PyTorchJob"
	JobKind         = "Job"
	DeploymentKind  = "Deployment"
	StatefulSetKind = "StatefulSet"
	PodKind         = "Pod"
	EventKind       = "Event"
	ConfigmapKind   = "ConfigMap"
	DefaultVersion  = "v1"

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
	UserSystem            = "system"
	UserWorkspaces        = "workspaces"
	UserManagedWorkspaces = "managedWorkspaces"
	Name                  = "name"
	PodId                 = "podId"
)

const (
	ImageImportReadyStatus  = "Ready"
	ImageImportingStatus    = "Importing"
	ImageImportFailedStatus = "Failed"
)
