/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type RequestWorkQueue = workqueue.TypedRateLimitingInterface[reconcile.Request]

const (
	PrimusSafePrefix = "primus-safe."
	PrimusSafeDomain = "primus-safe/"

	// general
	DisplayNameLabel = PrimusSafePrefix + "display.name"
	// Corresponding resource names in Kubernetes ResourceList, e.g. amd.com/gpu or nvidia.com/gpu
	GpuResourceNameAnnotation = PrimusSafePrefix + "gpu.resource.name"
	// the label for Control-plane node
	KubernetesControlPlane = "node-role.kubernetes.io/control-plane"
	// total retry count
	RetryCountAnnotation        = PrimusSafePrefix + "retry.count"
	DescriptionAnnotation       = PrimusSafePrefix + "description"
	ProtectLabel                = PrimusSafePrefix + "protect"
	MainContainerAnnotation     = PrimusSafePrefix + "main.container"
	OwnerLabel                  = PrimusSafePrefix + "owner"
	RequireNodeSpreadAnnotation = PrimusSafePrefix + "require.node.spread"

	// cicd
	CICDScaleSetIdAnnotation = "runner-scale-set-id"
	CICDScaleRunnerIdLabel   = PrimusSafePrefix + "scale.runner.id"
	GithubSecretIdAnnotation = PrimusSafePrefix + "github.secret.id"

	// node
	NodePrefix    = PrimusSafePrefix + "node."
	NodeFinalizer = PrimusSafeDomain + "node.finalizer"
	// The expected GPU count for the node, it should be annotated as a label
	NodeGpuCountLabel = NodePrefix + "gpu.count"
	// The node's last startup time
	NodeStartupTimeLabel            = NodePrefix + "startup.time"
	NodeLabelAction                 = NodePrefix + "label.action"
	NodeAnnotationAction            = NodePrefix + "annotation.action"
	NodeTemplateInstalledAnnotation = NodePrefix + "template.installed"
	NodeIdLabel                     = NodePrefix + "id"
	NodeResetAnnotation             = NodePrefix + "reset"
	NodeManageRebootLabel           = "manage.reboot"
	NodeUnmanageNoRebootLabel       = "unmanage.noreboot"
	NodeActionAdd                   = "add"
	NodeActionRemove                = "remove"

	// cluster
	ClusterPrefix                 = PrimusSafePrefix + "cluster."
	ClusterFinalizer              = PrimusSafeDomain + "cluster.finalizer"
	ClusterManagePrefix           = ClusterPrefix + "manage."
	ClusterManageActionLabel      = ClusterManagePrefix + "action"
	ClusterManageClusterLabel     = ClusterManagePrefix + "cluster"
	ClusterManageNodeLabel        = ClusterManagePrefix + "node"
	ClusterManageNodeClusterLabel = ClusterManagePrefix + "node.cluster"
	ClusterManageScaleDownLabel   = ClusterManagePrefix + "scale.down"
	ClusterIdLabel                = ClusterPrefix + "id"

	// storage
	StoragePrefix              = PrimusSafePrefix + "storage."
	StorageFinalizer           = PrimusSafeDomain + "storage.finalizer"
	StorageDefaultClusterLabel = StoragePrefix + "default.cluster"
	StorageClusterNameLabel    = StoragePrefix + "cluster.name"

	// nodeflavor
	NodeFlavorPrefix  = PrimusSafePrefix + "nodeflavor."
	NodeFlavorIdLabel = NodeFlavorPrefix + "id"

	// workspace
	WorkspacePrefix        = PrimusSafePrefix + "workspace."
	WorkspaceFinalizer     = PrimusSafeDomain + "workspace.finalizer"
	WorkspaceIdLabel       = WorkspacePrefix + "id"
	WorkspaceNodesAction   = WorkspacePrefix + "nodes.action"
	WorkspaceIdsAnnotation = WorkspacePrefix + "ids"

	// fault
	FaultPrefix    = PrimusSafePrefix + "fault."
	FaultFinalizer = PrimusSafeDomain + "fault.finalizer"
	FaultId        = FaultPrefix + "id"

	// workload
	WorkloadPrefix                    = PrimusSafePrefix + "workload."
	WorkloadFinalizer                 = PrimusSafeDomain + "workload.finalizer"
	WorkloadIdLabel                   = WorkloadPrefix + "id"
	WorkloadDispatchedAnnotation      = WorkloadPrefix + "dispatched"
	WorkloadScheduledAnnotation       = WorkloadPrefix + "scheduled"
	WorkloadPreemptedAnnotation       = WorkloadPrefix + "preempted"
	EnableHostNetworkAnnotation       = WorkloadPrefix + "enable.host.network"
	WorkloadKindLabel                 = WorkloadPrefix + "kind"
	WorkloadVersionLabel              = WorkloadPrefix + "version"
	WorkloadDispatchCntLabel          = WorkloadPrefix + "dispatch.count"
	WorkloadReScheduledAnnotation     = WorkloadPrefix + "rescheduled"
	WorkloadDisableFailoverAnnotation = WorkloadPrefix + "disable.failover"
	WorkloadEnablePreemptAnnotation   = WorkloadPrefix + "enable.preempt"
	CronJobTimestampAnnotation        = WorkloadPrefix + "cronjob"
	EnvToBeRemovedAnnotation          = WorkloadPrefix + "env.to.remove"
	AdminControlPlaneAnnotation       = WorkloadPrefix + "admin.control.plane"
	ResourceIdAnnotation              = WorkloadPrefix + "resource.id"

	// user
	UserPrefix              = PrimusSafePrefix + "user."
	UserIdLabel             = UserPrefix + "id"
	UserNameAnnotation      = UserPrefix + "name"
	UserNameMd5Label        = UserPrefix + "name.md5"
	UserEmailAnnotation     = UserPrefix + "email"
	UserEmailMd5Label       = UserPrefix + "email.md5"
	UserAvatarUrlAnnotation = UserPrefix + "avatar.url"
	UserTypeLabel           = UserPrefix + "type"

	// secret
	SecretPrefix    = PrimusSafePrefix + "secret."
	SecretFinalizer = PrimusSafeDomain + "secret.finalizer"
	SecretTypeLabel = SecretPrefix + "type"

	// exporter
	ExporterFinalizer = PrimusSafeDomain + "exporter.finalizer"

	// job
	OpsJobPrefix                    = PrimusSafePrefix + "ops.job."
	OpsJobIdLabel                   = OpsJobPrefix + "id"
	OpsJobTypeLabel                 = OpsJobPrefix + "type"
	OpsJobSecurityUpgradeAnnotation = OpsJobPrefix + "security.upgrade"
	OpsJobBatchCountAnnotation      = OpsJobPrefix + "batch.count"
	OpsJobAvailRatioAnnotation      = OpsJobPrefix + "avail.ratio"
	OpsJobFinalizer                 = PrimusSafeDomain + "ops.job.finalizer"

	// addon
	AddonPrefix       = PrimusSafePrefix + "addon."
	AddonFinalizer    = AddonPrefix + "finalizer"
	AddonDefaultLabel = AddonPrefix + "default"

	// model
	ModelPrefix  = PrimusSafePrefix + "model."
	ModelIdLabel = ModelPrefix + "id"

	// inference
	InferencePrefix    = PrimusSafePrefix + "inference."
	InferenceFinalizer = PrimusSafeDomain + "inference.finalizer"
	InferenceIdLabel   = InferencePrefix + "id"
)

type SecretType string

const (
	TrueStr = "true"

	K8sHostName = "kubernetes.io/hostname"

	SecretImage   SecretType = "image"
	SecretSSH     SecretType = "ssh"
	SecretGeneral SecretType = "general"
)
