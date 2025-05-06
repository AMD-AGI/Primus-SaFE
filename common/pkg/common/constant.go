/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package common

const (
	SafeNamespace       = "safe"
	KubeSystemNamespace = "kube-system"
	KubePublicNamespace = "kube-public"

	DefaultBurst = 1000
	DefaultQPS   = 1000

	NvidiaGpu            = "nvidia.com/gpu"
	NvidiaIdentification = "nvidia.com/gpu.present"
	NvidiaVfio           = "nvidia.com/gpu.deploy.vfio-manager"
	NvidiaProduct        = "nvidia.com/gpu.product"

	AMDGpuIdentification = "feature.node.kubernetes.io/amd-gpu"
	AmdGpu               = "amd.com/gpu"

	RDMA      = "rdma/hca"
	GpuMemory = "gpuMemory"
	GpuType   = "gpuType"

	PytorchJobKind   = "PyTorchJob"
	JobKind          = "Job"
	PodKind          = "Pod"
	EventKind        = "Event"
	DeploymentKind   = "Deployment"
	StatefulSetKind  = "StatefulSet"
	PytorchMaster    = "Master"
	PytorchWorker    = "Worker"
	MainContainer    = "mainContainer"
	NodeNameSelector = "spec.nodeName="

	K8sV1APIVersion         = "v1"
	BatchGroup              = "batch"
	KubeflowGroup           = "kubeflow.org"
	K8sHostNameLabel        = "kubernetes.io/hostname"
	SystemUserId            = "system"
	ControlPlaneClusterName = "control-plane"
	FormContentType         = "application/x-www-form-urlencoded"

	HighPriority    = "high-priority"
	MedPriority     = "med-priority"
	LowPriority     = "low-priority"
	HighPriorityInt = 2
	MedPriorityInt  = 1
	LowPriorityInt  = 0
)
