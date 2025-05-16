/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourceTemplateSpec struct {
	GroupVersionKind GroupVersionKind `json:"groupVersionKind"`
	Templates        []Template       `json:"templates,omitempty"`
	EndState         EndState         `json:"endState,omitempty"`
	ActiveState      ActiveState      `json:"activeState,omitempty"`
}

type Template struct {
	// Pre-paths to template & replicas
	PrePaths []string `json:"prePaths,omitempty"`
	// PodTemplateSpec
	TemplatePaths []string `json:"templatePaths,omitempty"`
	ReplicasPaths []string `json:"replicasPaths,omitempty"`
}

type GroupVersionKind struct {
	Group   string `json:"group,omitempty"`
	Version string `json:"version,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

func (gvk GroupVersionKind) String() string {
	return gvk.Group + "/" + gvk.Version + ", Kind=" + gvk.Kind
}

type EndState struct {
	PrePaths     []string        `json:"prePaths,omitempty"`
	MessagePaths []string        `json:"messagePaths,omitempty"`
	ReasonPaths  []string        `json:"reasonPaths,omitempty"`
	Phases       []TemplatePhase `json:"phases,omitempty"`
}

type TemplatePhase struct {
	MatchExpressions map[string]string `json:"matchExpressions"`
	Phase            string            `json:"phase"`
}

type ActiveState struct {
	PrePaths []string `json:"prePaths,omitempty"`
	Active   string   `json:"active,omitempty"`
}

type ResourceTemplateStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:rbac:groups=amd.com,resources=resourcetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=resourcetemplates/status,verbs=get;update;patch
type ResourceTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceTemplateSpec   `json:"spec,omitempty"`
	Status ResourceTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ResourceTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceTemplate{}, &ResourceTemplateList{})
}

func (rt *ResourceTemplate) ResourceGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   rt.Spec.GroupVersionKind.Group,
		Version: rt.Spec.GroupVersionKind.Version,
		Kind:    rt.Spec.GroupVersionKind.Kind,
	}
}

func (rt *ResourceTemplate) ResourceApiVersion() string {
	return rt.Spec.GroupVersionKind.Group + "/" + rt.Spec.GroupVersionKind.Version
}
