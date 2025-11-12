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
	AddonKind = "Addon"
)

type AddonPhaseType string

const (
	AddonCreating AddonPhaseType = "creating"
	AddonDeleting AddonPhaseType = "deleting"
	AddonRunning                 = "running"
	AddonDeleted                 = "deleted"
	AddonDeployed                = "deployed"
	// From running to failed
	AddonFailed = "failed"
	// If the creation process fails, it is in error state
	AddonError = "error"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:rbac:groups=amd.com,resources=addons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=addons/status,verbs=get;update;patch

type Addon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AddonSpec   `json:"spec,omitempty"`
	Status AddonStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type AddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Addon `json:"items"`
}

type AddonSpec struct {
	Cluster     *corev1.ObjectReference `json:"cluster"`
	AddonSource AddonSource             `json:"addonSource,omitempty"`
}

type AddonSource struct {
	HelmRepository *HelmRepository `json:"helm,omitempty"`
}

type HelmRepository struct {
	ReleaseName     string                  `json:"releaseName"`
	PlainHTTP       bool                    `json:"plainHttp,omitempty"`
	ChartVersion    string                  `json:"chartVersion,omitempty"`
	Namespace       string                  `json:"namespace,omitempty"`
	Values          string                  `json:"values,omitempty"`
	PreviousVersion *int                    `json:"previousVersion,omitempty"`
	Template        *corev1.ObjectReference `json:"template,omitempty"`
	URL             string                  `json:"url,omitempty"`
}

type AddonStatus struct {
	Phase             AddonPhaseType    `json:"phase"`
	AddonSourceStatus AddonSourceStatus `json:"addonSourceStatus,omitempty"`
}

type AddonSourceStatus struct {
	HelmRepositoryStatus *HelmRepositoryStatus `json:"helm"`
}

type HelmRepositoryStatus struct {
	// FirstDeployed is when the release was first deployed.
	FirstDeployed metav1.Time `json:"firstDeployed,omitempty"`
	// LastDeployed is when the release was last deployed.
	LastDeployed metav1.Time `json:"lastDeployed,omitempty"`
	// Deleted tracks when this object was deleted.
	Deleted metav1.Time `json:"deleted,omitempty"`
	// Description is human-friendly "log entry" about this release.
	Description string `json:"description,omitempty"`
	// Status is the current state of the release
	Status string `json:"status,omitempty"`
	// Contains the rendered templates/NOTES.txt if available
	Notes string `json:"notes,omitempty"`
	// Contains the deployed resources information
	//Resources    map[string][]runtime.Object `json:"resources,omitempty"`
	Version         int                     `json:"version,omitempty"`
	ChartVersion    string                  `json:"chartVersion,omitempty"`
	Values          string                  `json:"values,omitempty"`
	PreviousVersion int                     `json:"previousVersion,omitempty"`
	Template        *corev1.ObjectReference `json:"template,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Addon{}, &AddonList{})
}
