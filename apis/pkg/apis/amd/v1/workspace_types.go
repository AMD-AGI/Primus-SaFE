/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkspacePhase string
type WorkspaceQueuePolicy string

// +kubebuilder:validation:Enum=Train;Infer;Authoring
type WorkspaceScope string
type FileSystemType string
type VolumePurpose int
type WorkspaceType string

const (
	WorkspaceKind = "Workspace"

	WorkspaceCreating WorkspacePhase = "Creating"
	WorkspaceRunning  WorkspacePhase = "Running"
	WorkspaceAbnormal WorkspacePhase = "Abnormal"
	WorkspaceDeleted  WorkspacePhase = "Deleted"

	QueueFifoPolicy    WorkspaceQueuePolicy = "fifo"
	QueueBalancePolicy WorkspaceQueuePolicy = "balance"

	TrainScope     WorkspaceScope = "Train"
	InferScope     WorkspaceScope = "Infer"
	AuthoringScope WorkspaceScope = "Authoring"
)

type WorkspaceSpec struct {
	// The name of the cluster that the workspace belongs to
	Cluster string `json:"cluster"`
	// node flavor id
	NodeFlavor string `json:"nodeFlavor,omitempty"`
	// node count
	Replica int `json:"replica,omitempty"`
	// Queuing policy for tasks submitted in this workspace.
	// All tasks currently share the same policy (no per-task customization). default is fifo.
	QueuePolicy WorkspaceQueuePolicy `json:"queuePolicy,omitempty"`
	// Service modules available in this space. No limitation if not specified.
	Scopes []WorkspaceScope `json:"scopes,omitempty"`
	// volumes used in this space
	Volumes []WorkspaceVolume `json:"volumes,omitempty"`
	// Is preemption enabled. default is false
	EnablePreempt bool `json:"enablePreempt,omitempty"`
}

type WorkspaceVolume struct {
	// The storage type, which is also used as the PVC name
	StorageType StorageUseType `json:"storageType"`
	// Mount path to be used, equivalent to 'mountPath' in Kubernetes volume mounts. Required field.
	MountPath string `json:"mountPath"`
	// equivalent to 'subPath' in Kubernetes volume mounts
	SubPath string `json:"subPath,omitempty"`
	// Path on the host to mount. Required when storage type is gpfs.
	HostPath string `json:"hostPath,omitempty"`
	// Capacity size, such as 100Gi. This is a required parameter when creating a PVC (PersistentVolumeClaim).
	Capacity string `json:"capacity,omitempty"`
	// volumeName specifies the name of an existing PersistentVolume. It cannot be used together with storageClassName.
	// If both are set, the volumeName takes priority
	PersistentVolumeName string `json:"PersistentVolumeName,omitempty"`
	// Responsible for automatic PV creation
	StorageClass string `json:"storageClass,omitempty"`
	// access mode，default: ReadWriteMany
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`
}

type WorkspaceStatus struct {
	Phase              WorkspacePhase      `json:"phase,omitempty"`
	TotalResources     corev1.ResourceList `json:"totalResources,omitempty"`
	AvailableResources corev1.ResourceList `json:"availableResources,omitempty"`
	AvailableReplica   int                 `json:"availableReplica,omitempty"`
	AbnormalReplica    int                 `json:"abnormalReplica,omitempty"`
	UpdateTime         *metav1.Time        `json:"updateTime,omitempty"`
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

func (w *Workspace) IsEnd() bool {
	if w.Status.Phase == WorkspaceRunning || w.Status.Phase == WorkspaceAbnormal {
		return true
	}
	return false
}

func (w *Workspace) IsAbnormal() bool {
	if w.Status.Phase == WorkspaceAbnormal {
		return true
	}
	return false
}

func (w *Workspace) IsPending() bool {
	if w.Status.Phase == "" || w.Status.Phase == WorkspaceCreating {
		return true
	}
	return false
}

func (w *Workspace) IsEnableFifo() bool {
	if w.Spec.QueuePolicy == "" || w.Spec.QueuePolicy == QueueFifoPolicy {
		return true
	}
	return false
}
