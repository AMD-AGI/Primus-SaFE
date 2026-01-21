/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkspacePhase string
type WorkspaceQueuePolicy string

// +kubebuilder:validation:Enum=Train;Infer;Authoring;CICD;Ray
type WorkspaceScope string
type FileSystemType string
type VolumePurpose int
type WorkspaceType string
type WorkspaceVolumeType string

const (
	WorkspaceKind = "Workspace"

	WorkspaceCreating WorkspacePhase = "Creating"
	WorkspaceRunning  WorkspacePhase = "Running"
	WorkspaceAbnormal WorkspacePhase = "Abnormal"
	WorkspaceDeleting WorkspacePhase = "Deleting"

	QueueFifoPolicy    WorkspaceQueuePolicy = "fifo"
	QueueBalancePolicy WorkspaceQueuePolicy = "balance"

	TrainScope     WorkspaceScope = "Train"
	InferScope     WorkspaceScope = "Infer"
	AuthoringScope WorkspaceScope = "Authoring"
	CICDScope      WorkspaceScope = "CICD"
	RayScope       WorkspaceScope = "Ray"

	HOSTPATH WorkspaceVolumeType = "hostpath"
	PFS      WorkspaceVolumeType = "pfs"
)

type WorkspaceSpec struct {
	// The cluster that the workspace belongs to
	Cluster string `json:"cluster"`
	// The node flavor id of workspace, A workspace supports only one node flavor
	NodeFlavor string `json:"nodeFlavor,omitempty"`
	// The expected number of nodes in the workspace
	Replica int `json:"replica,omitempty"`
	// Queuing policy for workloads submitted in this workspace.
	// All workloads currently share the same policy, supports fifo (default) and balance.
	// 1. "Fifo" means first-in, first-out: the workload that enters the queue first is served first.
	//    If the front workload does not meet the conditions for dispatch, it will wait indefinitely,
	//    and other tasks in the queue will also be blocked waiting.
	// 2. "Balance" allows any workload that meets the resource conditions to be dispatched,
	//    avoiding blockage by the front workload in the queue. However, it is still subject to priority constraints.
	//    If a higher-priority task cannot be dispatched, lower-priority tasks will wait.
	QueuePolicy WorkspaceQueuePolicy `json:"queuePolicy,omitempty"`
	// Service modules available in this space. support: Train/Infer/Authoring/CICD, No limitation if not specified
	Scopes []WorkspaceScope `json:"scopes,omitempty"`
	// Volumes used in this workspace
	Volumes []WorkspaceVolume `json:"volumes,omitempty"`
	// Whether preemption is enabled. If enabled, higher-priority workload will preempt the lower-priority one
	EnablePreempt bool `json:"enablePreempt,omitempty"`
	// User id of the workspace administrator
	Managers []string `json:"managers,omitempty"`
	// Set the workspace as the default workspace (i.e., all users can access it)
	IsDefault bool `json:"isDefault,omitempty"`
	// Workspace image secret ID, used for downloading images
	ImageSecrets []corev1.ObjectReference `json:"imageSecrets,omitempty"`
}

type WorkspaceVolume struct {
	// The volume id, which is used to identify the volume. This field is set internally.
	Id int `json:"id"`
	// The volume type, valid values includes: pfs/hostpath
	// If PFS is configured, a PVC will be automatically created in the workspace.
	Type WorkspaceVolumeType `json:"type"`
	// Mount path to be used, equivalent to 'mountPath' in Kubernetes volume mounts.
	// +required
	MountPath string `json:"mountPath"`

	// Path on the host to mount. Required when volume type is hostpath
	HostPath string `json:"hostPath,omitempty"`

	// The following parameters are used for PVC creation. If using hostPath mounting, they are not required.
	// Capacity size, e.g. 100Gi. This is a required parameter when creating a PVC (PersistentVolumeClaim).
	Capacity string `json:"capacity,omitempty"`
	// selector is a label query over volumes to consider for binding.
	// It cannot be used together with storageClass. If both are set, the selector takes priority
	Selector map[string]string `json:"selector,omitempty"`
	// Responsible for automatic PV creation
	StorageClass string `json:"storageClass,omitempty"`
	// access mode, default ReadWriteMany
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`
	// equivalent to 'subPath' in Kubernetes volume mounts
	// +optional
	SubPath string `json:"subPath,omitempty"`
}

type WorkspaceStatus struct {
	// The status of workspace, e.g. Creating, Running, Abnormal, Deleting
	Phase WorkspacePhase `json:"phase,omitempty"`
	// The total resource of workspace
	TotalResources corev1.ResourceList `json:"totalResources,omitempty"`
	// The available resource of workspace
	AvailableResources corev1.ResourceList `json:"availableResources,omitempty"`
	// The abnormal resource of workspace
	AbnormalResources corev1.ResourceList `json:"abnormalResources,omitempty"`
	// The available node count of workspace
	AvailableReplica int `json:"availableReplica,omitempty"`
	// The abnormal node count of workspace
	AbnormalReplica int `json:"abnormalReplica,omitempty"`
	// Last update time
	UpdateTime *metav1.Time `json:"updateTime,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-workspace,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=workspaces,verbs=create;update,versions=v1,name=mworkspace.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=workspaces,verbs=create;update,versions=v1,name=vworkspace.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=amd.com,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=workspaces/status,verbs=get;update;patch

type Workspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkspaceSpec   `json:"spec,omitempty"`
	Status WorkspaceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}

// IsEnd returns true if the fault has ended (completed or failed).
func (w *Workspace) IsEnd() bool {
	if w.Status.Phase == WorkspaceRunning || w.Status.Phase == WorkspaceAbnormal {
		return true
	}
	return false
}

// IsAbnormal returns true if the workspace is abnormal
func (w *Workspace) IsAbnormal() bool {
	if w.Status.Phase == WorkspaceAbnormal {
		return true
	}
	return false
}

// IsPending returns true if the operations job is pending execution.
func (w *Workspace) IsPending() bool {
	if w.Status.Phase == "" || w.Status.Phase == WorkspaceCreating {
		return true
	}
	return false
}

// IsEnableFifo returns true if the workspace uses FIFO (First In, First Out) queue policy.
// If QueuePolicy is not set, FIFO mode is enabled.
func (w *Workspace) IsEnableFifo() bool {
	if w.Spec.QueuePolicy == "" || w.Spec.QueuePolicy == QueueFifoPolicy {
		return true
	}
	return false
}

// CurrentReplica returns the current number of replicas in the workspace.
func (w *Workspace) CurrentReplica() int {
	return w.Status.AvailableReplica + w.Status.AbnormalReplica
}

func (w *Workspace) HasImageSecret(name string) bool {
	for _, secret := range w.Spec.ImageSecrets {
		if secret.Name == name {
			return true
		}
	}
	return false
}

func (w *Workspace) HasScope(scope WorkspaceScope) bool {
	for _, s := range w.Spec.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func (v *WorkspaceVolume) GenFullVolumeId() string {
	return GenFullVolumeId(v.Type, v.Id)
}

func GenFullVolumeId(volumeType WorkspaceVolumeType, id int) string {
	return string(volumeType) + "-" + strconv.Itoa(id)
}
