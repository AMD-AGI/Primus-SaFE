/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package common

const (
	PrimusSafeNamespace = "primus-safe"
	PrimusFault         = "primus-fault"

	NodeNameSelector    = "spec.nodeName="
	KubeSystemNamespace = "kube-system"
	KubePublicNamespace = "kube-public"
	PytorchJobKind      = "PyTorchJob"
	PodKind             = "Pod"
	EventKind           = "Event"
	DeploymentKind      = "Deployment"
	StatefulSetKind     = "StatefulSet"
	PytorchMaster       = "Master"
	PytorchWorker       = "Worker"
	MainContainer       = "mainContainer"

	DefaultBurst = 1000
	DefaultQPS   = 1000

	NvidiaGpu            = "nvidia.com/gpu"
	NvidiaIdentification = "nvidia.com/gpu.present"
	NvidiaVfio           = "nvidia.com/gpu.deploy.vfio-manager"
	AMDGpuIdentification = "feature.node.kubernetes.io/amd-gpu"
	AmdGpu               = "amd.com/gpu"
)
