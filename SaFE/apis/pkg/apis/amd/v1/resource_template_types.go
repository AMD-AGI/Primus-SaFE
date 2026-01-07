/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	ResourceTemplateKind = "ResourceTemplate"
)

type GroupVersionKind struct {
	Group   string `json:"group,omitempty"`
	Version string `json:"version,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

// ToSchemaGVK converts GroupVersionKind to schema.GroupVersionKind.
func (gvk GroupVersionKind) ToSchemaGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind,
	}
}

// String returns a string representation of the HTTP response.
func (gvk GroupVersionKind) String() string {
	return gvk.ToSchemaGVK().String()
}

// VersionKind returns a string representation of version and kind.
func (gvk GroupVersionKind) VersionKind() string {
	return gvk.Kind + "/" + gvk.Version
}

type ResourceTemplateSpec struct {
	// Specifies the Group, Version, and Kind (GVK) for a Kubernetes object
	// This GVK is not the same as the workload's GVK, which is defined in the label selector.
	GroupVersionKind GroupVersionKind `json:"groupVersionKind"`
	// Definition used to retrieve the spec of a Kubernetes object
	ResourceSpecs []ResourceSpec `json:"resourceSpecs,omitempty"`
	// Definition used to retrieve the status of a Kubernetes object
	ResourceStatus ResourceStatus `json:"resourceStatus,omitempty"`
	// Definition used to retrieve the active replica count of a Kubernetes object
	ActiveReplica ActiveReplica `json:"activeReplica,omitempty"`
}

type ResourceSpec struct {
	// Pre-path to k8s object's spec
	PrePaths []string `json:"prePaths,omitempty"`
	// The relative path of pod template
	TemplatePaths []string `json:"templatePaths,omitempty"`
	// The relative path of pod replica
	ReplicasPaths []string `json:"replicasPaths,omitempty"`
	// The relative path of pod completions(only for job)
	CompletionsPaths []string `json:"completionsPaths,omitempty"`
}

// GetTemplatePath returns the path components for locating the resource template.
func (t *ResourceSpec) GetTemplatePath() []string {
	if t == nil {
		return nil
	}
	path := append(t.PrePaths, t.TemplatePaths...)
	return path
}

type ResourceStatus struct {
	// Prefix path for retrieving the object's phase, commonly referencing the status condition.
	PrePaths []string `json:"prePaths,omitempty"`
	// The relative path of message
	MessagePaths []string `json:"messagePaths,omitempty"`
	// The relative path of reason
	ReasonPaths []string `json:"reasonPaths,omitempty"`
	// Expression for retrieving the phase value.
	Phases []PhaseExpression `json:"phases,omitempty"`
}

type PhaseExpression struct {
	MatchExpressions map[string]string `json:"matchExpressions"`
	Phase            string            `json:"phase"`
}

type ActiveReplica struct {
	PrePaths    []string `json:"prePaths,omitempty"`
	ReplicaPath string   `json:"replicaPath,omitempty"`
}

type ResourceTemplateStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-resourcetemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=resourcetemplates,verbs=create;update,versions=v1,name=mresourcetemplate.kb.io,admissionReviewVersions={v1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-resourcetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=resourcetemplates,verbs=create;update,versions=v1,name=vresourcetemplate.kb.io,admissionReviewVersions={v1}
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

// ToSchemaGVK converts the resource template GVK to schema.GroupVersionKind.
func (rt *ResourceTemplate) ToSchemaGVK() schema.GroupVersionKind {
	return rt.Spec.GroupVersionKind.ToSchemaGVK()
}

// SpecKind returns the kind string from the resource spec.
func (rt *ResourceTemplate) SpecKind() string {
	return rt.Spec.GroupVersionKind.Kind
}
