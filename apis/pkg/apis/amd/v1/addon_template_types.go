/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	AddOnTemplateKind = "AddonTemplate"
)

type AddonTemplateType string
type GpuChipType string
type GpuChipProduct string

const (
	AddonTemplateHelm    AddonTemplateType = "helm"
	AddonTemplateDefault AddonTemplateType = "default"

	AmdGpuChip    GpuChipType = "amd"
	NvidiaGpuChip GpuChipType = "nvidia"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-addontemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=addontemplates,verbs=create;update,versions=v1,name=maddontemplate.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-addontemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=addontemplates,verbs=create;update,versions=v1,name=vaddontemplate.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=amd.com,resources=addontemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=addontemplates/status,verbs=get;update;patch

type AddonTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AddonTemplateSpec   `json:"spec"`
	Status            AddonTemplateStatus `json:"status,omitempty"`
}

type AddonTemplateSpec struct {
	// Type of template
	Type AddonTemplateType `json:"type,omitempty"`
	// Category of template. e.g. system/gpu
	Category string `json:"category"`
	// The address for template, only for helm
	URL string `json:"url,omitempty"`
	// Version of template
	Version string `json:"version,omitempty"`
	// The description of template
	Description string `json:"description,omitempty"`
	// The installation action for this template (base64 encoded)
	Action string `json:"action,omitempty"`
	// Icon url, base64 encoded
	Icon string `json:"icon,omitempty"`
	// Target gpu chip(amd or nvidia), If left empty, it applies to all chip.
	GpuChip GpuChipType `json:"gpuChip,omitempty"`
	// If it is true, installation failure will terminate the installation of other packages and raise an error.
	// If it is false, only an error log will be printed and the failure can be ignored.
	Required bool `json:"required,omitempty"`
	// The default value for helm install
	HelmDefaultValues    string `json:"helmDefaultValues,omitempty"`
	HelmDefaultNamespace string `json:"helmDefaultNamespace,omitempty"`
}

type HelmStatus struct {
	Values     string `json:"values,omitempty"`
	ValuesYAML string `json:"valuesYaml,omitempty"`
}

type AddonTemplateStatus struct {
	HelmStatus HelmStatus `json:"helmStatus,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type AddonTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddonTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddonTemplate{}, &AddonTemplateList{})
}
