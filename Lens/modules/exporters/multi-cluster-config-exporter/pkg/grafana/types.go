// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package grafana

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is group version used to register these objects
var GroupVersion = schema.GroupVersion{Group: "grafana.integreatly.org", Version: "v1beta1"}

// GrafanaDatasource is the Schema for the grafanadatasources API
// +kubebuilder:object:root=true
type GrafanaDatasource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GrafanaDatasourceSpec   `json:"spec,omitempty"`
	Status GrafanaDatasourceStatus `json:"status,omitempty"`
}

// GrafanaDatasourceSpec defines the desired state of GrafanaDatasource
type GrafanaDatasourceSpec struct {
	// AllowCrossNamespaceImport toggles if the datasource can be used across namespaces
	AllowCrossNamespaceImport bool `json:"allowCrossNamespaceImport,omitempty"`

	// Datasource contains the actual datasource definition
	Datasource *GrafanaDatasourceInternal `json:"datasource,omitempty"`

	// InstanceSelector is a label selector used to identify target Grafana instance
	InstanceSelector *metav1.LabelSelector `json:"instanceSelector,omitempty"`

	// ResyncPeriod determines the resync period for this datasource
	ResyncPeriod string `json:"resyncPeriod,omitempty"`
}

// GrafanaDatasourceInternal contains the datasource configuration
type GrafanaDatasourceInternal struct {
	// Name is the datasource name
	Name string `json:"name,omitempty"`

	// Type is the datasource type (e.g., prometheus, postgres)
	Type string `json:"type,omitempty"`

	// Access mode - proxy or direct
	Access string `json:"access,omitempty"`

	// URL of the datasource
	URL string `json:"url,omitempty"`

	// User for authentication
	User string `json:"user,omitempty"`

	// IsDefault marks this datasource as default
	IsDefault bool `json:"isDefault,omitempty"`

	// JSONData contains additional settings
	JSONData map[string]interface{} `json:"jsonData,omitempty"`

	// SecureJSONData contains sensitive settings
	SecureJSONData map[string]string `json:"secureJsonData,omitempty"`
}

// GrafanaDatasourceStatus defines the observed state of GrafanaDatasource
type GrafanaDatasourceStatus struct {
	// Hash is the hash of the current spec
	Hash string `json:"hash,omitempty"`

	// UID is the Kubernetes UID
	UID string `json:"uid,omitempty"`

	// LastResync is the last resync time
	LastResync string `json:"lastResync,omitempty"`
}

// GrafanaDatasourceList contains a list of GrafanaDatasource
// +kubebuilder:object:root=true
type GrafanaDatasourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GrafanaDatasource `json:"items"`
}

// DeepCopyObject implements runtime.Object
func (in *GrafanaDatasource) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopy creates a deep copy of GrafanaDatasource
func (in *GrafanaDatasource) DeepCopy() *GrafanaDatasource {
	if in == nil {
		return nil
	}
	out := new(GrafanaDatasource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out
func (in *GrafanaDatasource) DeepCopyInto(out *GrafanaDatasource) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopyInto copies the spec into out
func (in *GrafanaDatasourceSpec) DeepCopyInto(out *GrafanaDatasourceSpec) {
	*out = *in
	if in.Datasource != nil {
		out.Datasource = in.Datasource.DeepCopy()
	}
	if in.InstanceSelector != nil {
		out.InstanceSelector = in.InstanceSelector.DeepCopy()
	}
}

// DeepCopy creates a deep copy of GrafanaDatasourceInternal
func (in *GrafanaDatasourceInternal) DeepCopy() *GrafanaDatasourceInternal {
	if in == nil {
		return nil
	}
	out := new(GrafanaDatasourceInternal)
	*out = *in
	if in.JSONData != nil {
		out.JSONData = make(map[string]interface{})
		for k, v := range in.JSONData {
			out.JSONData[k] = v
		}
	}
	if in.SecureJSONData != nil {
		out.SecureJSONData = make(map[string]string)
		for k, v := range in.SecureJSONData {
			out.SecureJSONData[k] = v
		}
	}
	return out
}

// DeepCopyObject implements runtime.Object
func (in *GrafanaDatasourceList) DeepCopyObject() runtime.Object {
	return in.DeepCopyList()
}

// DeepCopyList creates a deep copy of GrafanaDatasourceList
func (in *GrafanaDatasourceList) DeepCopyList() *GrafanaDatasourceList {
	if in == nil {
		return nil
	}
	out := new(GrafanaDatasourceList)
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]GrafanaDatasource, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

