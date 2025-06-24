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
type ChipType string

const (
	AddonTemplateDriver  AddonTemplateType = "driver"
	AddonTemplateHelm    AddonTemplateType = "helm"
	AddonTemplateDpkg    AddonTemplateType = "dpkg"
	AddonTemplateConfig  AddonTemplateType = "config"
	AddonTemplateSystemd AddonTemplateType = "systemd"

	AddOnObserve = "observe"
	AddOnAction  = "action"

	AmdGpuChip    ChipType = "amd"
	NvidiaGpuChip ChipType = "nvidia"
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
	// type of template
	Type AddonTemplateType `json:"type"`
	// category of template. e.g. system/gpu
	Category string `json:"category"`
	// only for helm
	URL string `json:"url,omitempty"`
	// version of template
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	// used for the action or observe commands (base64 encoded).
	Extensions map[string]string `json:"extensions,omitempty"`
	// icon url，base64 encoded
	Icon string `json:"icon,omitempty"`
	// target chip， If left empty, it applies to all chip.
	Chip ChipType `json:"chip,omitempty"`
	// If it is a One-shot Service, the reload operation is not applicable.
	IsOneShotService bool `json:"isOneShotService,omitempty"`
	// the default value for helm install
	HelmDefaultValues    string `json:"helmDefaultValues,omitempty"`
	HelmDefaultNamespace string `json:"helmDefaultNamespace,omitempty"`
}

type HelmStatus struct {
	Values     string `json:"values,omitempty"`
	ValuesYAMl string `json:"valuesYaml,omitempty"`
}

type AddonTemplateStatus struct {
	HelmStatus HelmStatus `json:"helmStatus,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type AddonTemplateList struct {
	metav1.TypeMeta `json:",inlineomite"`
	metav1.ListMeta `json:"metadata,mpty"`
	Items           []AddonTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddonTemplate{}, &AddonTemplateList{})
}
