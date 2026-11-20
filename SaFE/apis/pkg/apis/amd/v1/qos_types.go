/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	QosKind = "Qos"
)

// ResourceCap is a GPU, CPU and memory ceiling. An empty field is unset.
type ResourceCap struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	GPU    string `json:"gpu,omitempty"`
}

// UserQosScope is one user's ceiling in one workspace.
type UserQosScope struct {
	MaxResources *ResourceCap `json:"maxResources,omitempty"`
	// MaxRuntime is the timeout ceiling in seconds, keyed by workspace scope
	// (Train, Infer, Authoring, CICD, Ray, Sandbox). A missing or non-positive entry is unset.
	MaxRuntime map[string]int `json:"maxRuntime,omitempty"`
}

// UserQos is stored on the User. Workspaces holds the ceilings keyed by workspace id.
// The top-level MaxResources and MaxRuntime are not used.
type UserQos struct {
	MaxResources *ResourceCap `json:"maxResources,omitempty"`
	// MaxRuntime is the timeout ceiling in seconds, keyed by workspace scope
	// (Train, Infer, Authoring, CICD, Ray, Sandbox). A missing or non-positive entry is unset.
	MaxRuntime map[string]int          `json:"maxRuntime,omitempty"`
	Workspaces map[string]UserQosScope `json:"workspaces,omitempty"`
}

// QosSpec is the ceiling for one workspace. The object name is the workspace id.
type QosSpec struct {
	MaxResources *ResourceCap `json:"maxResources,omitempty"`
	// MaxRuntime is the timeout ceiling in seconds, keyed by workspace scope
	// (Train, Infer, Authoring, CICD, Ray, Sandbox). A missing or non-positive entry is unset.
	MaxRuntime map[string]int `json:"maxRuntime,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +resourceName=qos
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,path=qos,singular=qos
// +kubebuilder:rbac:groups=amd.com,resources=qos,verbs=get;list;watch;create;update;patch;delete

// Qos is the ceiling for one workspace. It does not carry per-user values.
type Qos struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec QosSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// QosList is a list of Qos.
type QosList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Qos `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Qos{}, &QosList{})
}

// DeepCopyInto copies a ResourceCap.
func (in *ResourceCap) DeepCopyInto(out *ResourceCap) {
	*out = *in
}

// DeepCopy copies a ResourceCap.
func (in *ResourceCap) DeepCopy() *ResourceCap {
	if in == nil {
		return nil
	}
	out := new(ResourceCap)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies a UserQosScope.
func (in *UserQosScope) DeepCopyInto(out *UserQosScope) {
	*out = *in
	out.MaxResources = in.MaxResources.DeepCopy()
	if in.MaxRuntime != nil {
		out.MaxRuntime = make(map[string]int, len(in.MaxRuntime))
		for k, v := range in.MaxRuntime {
			out.MaxRuntime[k] = v
		}
	}
}

// DeepCopy copies a UserQosScope.
func (in *UserQosScope) DeepCopy() *UserQosScope {
	if in == nil {
		return nil
	}
	out := new(UserQosScope)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies a UserQos.
func (in *UserQos) DeepCopyInto(out *UserQos) {
	*out = *in
	out.MaxResources = in.MaxResources.DeepCopy()
	if in.MaxRuntime != nil {
		out.MaxRuntime = make(map[string]int, len(in.MaxRuntime))
		for k, v := range in.MaxRuntime {
			out.MaxRuntime[k] = v
		}
	}
	if in.Workspaces != nil {
		out.Workspaces = make(map[string]UserQosScope, len(in.Workspaces))
		for k, v := range in.Workspaces {
			dst := UserQosScope{}
			v.DeepCopyInto(&dst)
			out.Workspaces[k] = dst
		}
	}
}

// DeepCopy copies a UserQos.
func (in *UserQos) DeepCopy() *UserQos {
	if in == nil {
		return nil
	}
	out := new(UserQos)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies a QosSpec.
func (in *QosSpec) DeepCopyInto(out *QosSpec) {
	*out = *in
	out.MaxResources = in.MaxResources.DeepCopy()
	if in.MaxRuntime != nil {
		out.MaxRuntime = make(map[string]int, len(in.MaxRuntime))
		for k, v := range in.MaxRuntime {
			out.MaxRuntime[k] = v
		}
	}
}

// DeepCopy copies a QosSpec.
func (in *QosSpec) DeepCopy() *QosSpec {
	if in == nil {
		return nil
	}
	out := new(QosSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies a Qos.
func (in *Qos) DeepCopyInto(out *Qos) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy copies a Qos.
func (in *Qos) DeepCopy() *Qos {
	if in == nil {
		return nil
	}
	out := new(Qos)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies a QosList.
func (in *QosList) DeepCopyInto(out *QosList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Qos, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy copies a QosList.
func (in *QosList) DeepCopy() *QosList {
	if in == nil {
		return nil
	}
	out := new(QosList)
	in.DeepCopyInto(out)
	return out
}
