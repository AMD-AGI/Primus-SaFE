/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CephCSIConfigName   = "ceph-csi-config"
	CephRBDCSINamespace = "ceph-csi-rbd"
	CephFSCSINamespace  = "ceph-csi-cephfs"
	DefaultNamespace    = "default"
)

type ClusterInfo struct {
	// ClusterID is used for unique identification
	ClusterID string `json:"clusterID"`
	// Monitors is monitor list for corresponding cluster ID
	Monitors []string `json:"monitors"`
	// CephFS contains CephFS specific options
	CephFS CephFSSpecific `json:"cephFS"`
	// RBD Contains RBD specific options
	RBD RBDSpecific `json:"rbd"`
	// NFS contains NFS specific options
	NFS NFSSpecific `json:"nfs"`
	// Read affinity map options
	ReadAffinity ReadAffinity `json:"readAffinity"`
}

type CephFSSpecific struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
	// SubvolumeGroup contains the name of the SubvolumeGroup for CSI volumes
	SubvolumeGroup string `json:"subvolumeGroup"`
	// KernelMountOptions contains the kernel mount options for CephFS volumes
	KernelMountOptions string `json:"kernelMountOptions"`
	// FuseMountOptions contains the fuse mount options for CephFS volumes
	FuseMountOptions string `json:"fuseMountOptions"`
}
type RBDSpecific struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
	// RadosNamespace is a rados namespace in the pool
	RadosNamespace string `json:"radosNamespace"`
	// RBD mirror daemons running in the ceph cluster.
	MirrorDaemonCount int `json:"mirrorDaemonCount"`
}

type NFSSpecific struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
}

type ReadAffinity struct {
	Enabled             bool     `json:"enabled"`
	CrushLocationLabels []string `json:"crushLocationLabels"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=ClusterName

type StorageCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              StorageClusterSpec   `json:"spec"`
	Status            StorageClusterStatus `json:"status,omitempty"`
}

type StorageType string

const (
	NVME StorageType = "nvme"
	SSD  StorageType = "ssd"
	HDD  StorageType = "hdd"
)

type StorageClusterSpec struct {
	Flavor    string                                 `json:"flavor"`
	Cluster   string                                 `json:"cluster"`
	Count     int                                    `json:"count"`
	Resources map[string]corev1.ResourceRequirements `json:"resources,omitempty"`
	Image     *string                                `json:"image,omitempty"`
}

type StorageClusterStatus struct {
	//State             string             `json:"state,omitempty"`
	Phase             Phase              `json:"phase,omitempty"`
	CephClusterStatus *CephClusterStatus `json:"cephStatus,omitempty"`
}

// CephClusterStatus represents the status of a Ceph cluster
type CephClusterStatus struct {
	Health         string   `json:"health,omitempty"`
	Capacity       Capacity `json:"capacity,omitempty"`
	LastChecked    string   `json:"lastChecked,omitempty"`
	LastChanged    string   `json:"lastChanged,omitempty"`
	PreviousHealth string   `json:"previousHealth,omitempty"`
	Monitors       []string `json:"monitors,omitempty"`
	AccessKey      string   `json:"accessKey,omitempty"`
	SecretKey      string   `json:"secretKey,omitempty"`
	ClusterId      string   `json:"clusterId,omitempty"`
	OSD            int      `json:"osd,omitempty"`
}

type Status struct {
	Name      string                  `json:"name"`
	AccessKey string                  `json:"accessKey"`
	SecretKey string                  `yaml:"secretKey"`
	Subsets   []corev1.EndpointSubset `json:"subsets,omitempty"`
}

// Capacity is the capacity information of a Ceph Cluster
type Capacity struct {
	TotalBytes     uint64 `json:"bytesTotal,omitempty"`
	UsedBytes      uint64 `json:"bytesUsed,omitempty"`
	AvailableBytes uint64 `json:"bytesAvailable,omitempty"`
	LastUpdated    string `json:"lastUpdated,omitempty"`
}

func (kc *Cluster) DeleteStorageStatus(name string) {
	newStatus := make([]StorageStatus, 0, len(kc.Spec.Storages))
	for i, stats := range kc.Status.StorageStatus {
		if stats.Name == name && stats.Ref == nil {
			continue
		}
		newStatus = append(newStatus, kc.Status.StorageStatus[i])
	}
	kc.Status.StorageStatus = newStatus
}

func (kc *Cluster) GetStorage(name string) (Storage, bool) {
	for i, storage := range kc.Spec.Storages {
		if storage.Name == name {
			return kc.Spec.Storages[i], true
		}
	}
	return Storage{}, false
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type StorageClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StorageCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StorageCluster{}, &StorageClusterList{})
}
