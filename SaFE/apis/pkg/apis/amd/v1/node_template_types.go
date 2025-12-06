/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	NodeTemplateKind = "NodeTemplate"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:rbac:groups=amd.com,resources=nodetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=nodetemplates/status,verbs=get;update;patch

type NodeTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NodeTemplateSpec `json:"spec,omitempty"`
}

type NodeTemplateSpec struct {
	// The list of Addon Template id
	AddOnTemplates []string `json:"addOnTemplates,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type NodeTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeTemplate{}, &NodeTemplateList{})
}
