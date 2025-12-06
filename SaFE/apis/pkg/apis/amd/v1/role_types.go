/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RoleVerb string

const (
	RoleKind = "Role"

	CreateVerb RoleVerb = "create"
	DeleteVerb RoleVerb = "delete"
	UpdateVerb RoleVerb = "update"
	GetVerb    RoleVerb = "get"
	ListVerb   RoleVerb = "list"
	AllVerb    RoleVerb = "*"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-role,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=roles,verbs=create;update,versions=v1,name=mrole.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-role,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=roles,verbs=create;update,versions=v1,name=vrole.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=amd.com,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=roles/status,verbs=get;update;patch

type Role struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Rules []PolicyRule `json:"rules"`
}

type PolicyRule struct {
	// Resources is a list of resources this rule applies to. '*' represents all resources.
	// e.g. workload, workspace/dev
	// +required
	Resources []string `json:"resources"`
	// grantedUsers is a list of users permitted to access the resource.
	// Setting 'owner' means that only the resource owner is allowed
	// Setting 'workspace-user' means that users of the workspace are allowed
	// '*' represents all users are allowed.
	// +optional
	GrantedUsers []string `json:"grantedUsers,omitempty"`
	// Verbs is a list of Verbs that apply to ALL the ResourceKinds contained in this rule.
	// '*' represents all verbs.
	// +required
	Verbs []RoleVerb `json:"verbs"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type RoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Role `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Role{}, &RoleList{})
}
