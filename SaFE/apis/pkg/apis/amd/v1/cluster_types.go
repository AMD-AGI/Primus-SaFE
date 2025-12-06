/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ClusterKind = "Cluster"

	FlannelNetworkPlugin = "flannel"
)

type (
	ClusterPhase        string
	ClusterManageAction string
	StorageUseType      string
)

const (
	// PendingPhase represents the cluster's first phase after being created
	PendingPhase           ClusterPhase        = "Pending"
	CreatingPhase          ClusterPhase        = "Creating"
	CreatedPhase           ClusterPhase        = "Created"
	ReadyPhase             ClusterPhase        = "Ready"
	CreationFailed         ClusterPhase        = "Failed"
	DeletingPhase          ClusterPhase        = "Deleting"
	DeletedPhase           ClusterPhase        = "Deleted"
	DeleteFailedPhase      ClusterPhase        = "DeleteFailed"
	ClusterCreateAction    ClusterManageAction = "create"
	ClusterScaleUpAction   ClusterManageAction = "up"
	ClusterScaleDownAction ClusterManageAction = "down"
	ClusterResetAction     ClusterManageAction = "reset"
)

const (
	RBD StorageUseType = "rbd"
	OBS StorageUseType = "obs"
	FS  StorageUseType = "cephfs"
)

// ErasureCodedSpec represents the spec for erasure code in a pool
type ErasureCodedSpec struct {
	CodingChunks uint   `json:"codingChunks"`
	DataChunks   uint   `json:"dataChunks"`
	Algorithm    string `json:"algorithm,omitempty"`
}

// ReplicatedSpec represents the spec for replication in a pool
type ReplicatedSpec struct {
	Size uint `json:"size"`
	// +optional
	// TargetSizeRatio          float64            `json:"targetSizeRatio,omitempty"`
	RequireSafeReplicaSize   bool               `json:"requireSafeReplicaSize,omitempty"`
	ReplicasPerFailureDomain uint               `json:"replicasPerFailureDomain,omitempty"`
	SubFailureDomain         string             `json:"subFailureDomain,omitempty"`
	HybridStorage            *HybridStorageSpec `json:"hybridStorage,omitempty"`
}

// HybridStorageSpec represents the settings for hybrid storage pool
type HybridStorageSpec struct {
	PrimaryDeviceClass   string `json:"primaryDeviceClass"`
	SecondaryDeviceClass string `json:"secondaryDeviceClass"`
}

type Storage struct {
	Name           string         `json:"name"`
	Type           StorageUseType `json:"type"`
	StorageCluster string         `json:"storageCluster"`
	StorageClass   string         `json:"storageClass,omitempty"`
	Secret         string         `json:"secret,omitempty"`
	Namespace      string         `json:"namespace,omitempty"`
	// The replication settings
	// +optional
	Replicated *ReplicatedSpec `json:"replicated,omitempty"`

	// The erasure code settings
	// +optional
	ErasureCoded *ErasureCodedSpec `json:"erasureCoded,omitempty"`
}

type StorageStatus struct {
	Storage   `json:",inline"`
	ClusterId string                  `json:"clusterId"`
	Monitors  []string                `json:"monitors,omitempty"`
	Pool      string                  `json:"pool"`
	Phase     string                  `json:"phase,omitempty"`
	Ref       *corev1.ObjectReference `json:"ref,omitempty"`
	AccessKey string                  `json:"accessKey,omitempty"`
	SecretKey string                  `json:"secretKey,omitempty"`
	Subsets   []corev1.EndpointSubset `json:"subsets,omitempty"`
}

type ClusterSpec struct {
	ControlPlane ControlPlane `json:"controlPlane,omitempty"`
	// Reserved field, logic not implemented currently
	Storages []Storage `json:"storages,omitempty"`
}

type ClusterStatus struct {
	ControlPlaneStatus ControlPlaneStatus `json:"controlPlaneStatus,omitempty"`
	// Reserved field, logic not implemented currently.
	StorageStatus []StorageStatus `json:"storageStatus,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-cluster,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=clusters,verbs=create;update,versions=v1,name=mcluster.kb.io,admissionReviewVersions={v1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-cluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=cluster,verbs=create;update,versions=v1,name=vcluster.kb.io,admissionReviewVersions={v1}
// +kubebuilder:rbac:groups=amd.com,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=clusters/status,verbs=get;update;patch

type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

type ControlPlane struct {
	// The nodes of control plane
	Nodes []string `json:"nodes"`
	// SSH secret for accessing cluster nodes
	SSHSecret *corev1.ObjectReference `json:"secret,omitempty"`
	// KubeSpray image name used for installation
	KubeSprayImage *string `json:"kubeSprayImage,omitempty"`
	// The Image secret used for download image
	ImageSecret *corev1.ObjectReference `json:"imageSecret,omitempty"`
	// Pod subnet configuration
	KubePodsSubnet *string `json:"kubePodsSubnet,omitempty"`
	// Service Address configuration
	KubeServiceAddress    *string `json:"kubeServiceAddress,omitempty"`
	KubeNetworkNodePrefix *uint32 `json:"kubeNetworkNodePrefix,omitempty"`
	// Network plugin, default flannel
	KubeNetworkPlugin *string `json:"kubeNetworkPlugin,omitempty"`
	// Kubernetes version
	KubeVersion    *string `json:"kubernetesVersion,omitempty"`
	KubeProxyMode  *string `json:"kubeProxyMode,omitempty"`
	NodeLocalDNSIP *string `json:"nodeLocalDNSIP,omitempty"`
	// Some parameter settings for Kubernetes
	KubeApiServerArgs      map[string]string  `json:"kubeApiServerArgs,omitempty"`
	KubeletLogFilesMaxSize *resource.Quantity `json:"kubeletLogFilesMaxSize,omitempty"`
	KubeletConfigArgs      map[string]string  `json:"kubeletConfigArgs,omitempty"`
}

type ControlPlaneStatus struct {
	Phase ClusterPhase `json:"phase,omitempty"`
	// CertData holds PEM-encoded bytes (typically read from a client certificate file).
	// CertData takes precedence over CertFile
	CertData string `json:"certData,omitempty"`
	// KeyData holds PEM-encoded bytes (typically read from a client certificate key file).
	// KeyData takes precedence over KeyFile
	KeyData string `json:"keyData,omitempty"`
	// CAData holds PEM-encoded bytes (typically read from a root certificates bundle).
	// CAData takes precedence over CAFile
	CAData    string   `json:"CAData,omitempty"`
	Endpoints []string `json:"endpoints,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}

// IsReady returns true if the cluster is in Ready phase.
func (cluster *Cluster) IsReady() bool {
	if cluster != nil && cluster.Status.ControlPlaneStatus.Phase == ReadyPhase {
		return true
	}
	return false
}

// DeleteStorageStatus removes storage status for the specified storage name.
func (cluster *Cluster) DeleteStorageStatus(name string) {
	newStatus := make([]StorageStatus, 0, len(cluster.Spec.Storages))
	for i, stats := range cluster.Status.StorageStatus {
		if stats.Name == name && stats.Ref == nil {
			continue
		}
		newStatus = append(newStatus, cluster.Status.StorageStatus[i])
	}
	cluster.Status.StorageStatus = newStatus
}

// GetStorage retrieves storage configuration by name.
func (cluster *Cluster) GetStorage(name string) (Storage, bool) {
	for i, storage := range cluster.Spec.Storages {
		if storage.Name == name {
			return cluster.Spec.Storages[i], true
		}
	}
	return Storage{}, false
}
