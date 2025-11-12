/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type UserType string
type UserRole string
type UserRestrictedType int

const (
	UserKind = "User"

	DefaultUserType UserType = "default"
	SSOUserType     UserType = "sso"

	UserNormal UserRestrictedType = 0
	UserFrozen UserRestrictedType = 1

	SystemAdminRole UserRole = "system-admin"
	// only for internal-use, The user will not be assigned the workspace-admin role
	WorkspaceAdminRole UserRole = "workspace-admin"
	DefaultRole        UserRole = "default"
)

type UserSpec struct {
	// User type, required. default/sso
	Type UserType `json:"type"`
	// User password, base64 encoded, optional
	Password string `json:"password,omitempty"`
	// 0: normal; 1 frozen. default: 0
	// +optional
	RestrictedType UserRestrictedType `json:"restrictedType,omitempty"`
	// User role, e.g. system-admin/default
	// permission check passes if any single role is satisfied.
	// +required
	Roles []UserRole `json:"roles"`
	// The key of resources is the name to be managed (e.g. workspace)
	// values are its corresponding values(e.g. workspace-id)"
	// +optional
	Resources map[string][]string `json:"resources,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-user,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=users,verbs=create;update,versions=v1,name=muser.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-user,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=users,verbs=create;update,versions=v1,name=vuser.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=amd.com,resources=users,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=users/status,verbs=get;update;patch

type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec UserSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}

func init() {
	SchemeBuilder.Register(&User{}, &UserList{})
}

// IsSystemAdmin returns true if the condition is met.
func (u *User) IsSystemAdmin() bool {
	return IsContainRole(u.Spec.Roles, SystemAdminRole)
}

// IsRestricted returns true if the condition is met.
func (u *User) IsRestricted() bool {
	return u.Spec.RestrictedType > 0
}

// IsContainRole returns true if the condition is met.
func IsContainRole(roles []UserRole, input UserRole) bool {
	for _, r := range roles {
		if r == input {
			return true
		}
	}
	return false
}
