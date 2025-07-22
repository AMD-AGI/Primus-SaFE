/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package common

const (
	PrimusSafeNamespace        = "primus-safe"
	PrimusFault                = "primus-safe-fault"
	PrimusImageSecret          = "primus-safe-image"
	PrimusCryptoSecret         = "primus-safe-crypto"
	PrimusRouterCustomRootPath = "api/v1"

	AuthoringKind   = "Authoring"
	PytorchJobKind  = "PyTorchJob"
	JobKind         = "Job"
	DeploymentKind  = "Deployment"
	StatefulSetKind = "StatefulSet"
	PodKind         = "Pod"
	EventKind       = "Event"
	DefaultVersion  = "v1"

	HigressNamespace = "higress-system"
	HigressGateway   = "higress-gateway"
	HigressSSHPort   = 22

	NodeNameSelector    = "spec.nodeName="
	KubeSystemNamespace = "kube-system"
	KubePublicNamespace = "kube-public"
	PytorchJobPortName  = "pytorchjob-port"

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
	NvidiaVfio           = "nvidia.com/gpu.deploy.vfio-manager"
	AMDGpuIdentification = "feature.node.kubernetes.io/amd-gpu"
	AmdGpu               = "amd.com/gpu"

	CustomerLabelPrefix = "customer."
	K8sHostNameLabel    = "kubernetes.io/hostname"
)
