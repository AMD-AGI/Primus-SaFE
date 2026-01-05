/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

type ListAddonTemplateResponse struct {
	// The total number of addon templates, not limited by pagination
	TotalCount int                         `json:"totalCount"`
	Items      []AddonTemplateResponseItem `json:"items"`
}

type AddonTemplateResponseItem struct {
	// The addon template id
	AddonTemplateId string `json:"addonTemplateId"`
	// Type of template (helm or default)
	Type string `json:"type"`
	// Category of template (e.g. system/gpu)
	Category string `json:"category"`
	// Version of template
	Version string `json:"version,omitempty"`
	// The description of template
	Description string `json:"description,omitempty"`
	// Target gpu chip (amd or nvidia)
	GpuChip string `json:"gpuChip,omitempty"`
	// Whether this template is required
	Required bool `json:"required"`
	// The addon template creation time
	CreationTime string `json:"creationTime"`
}

type GetAddonTemplateResponse struct {
	AddonTemplateResponseItem
	// The URL address for template (helm only)
	URL string `json:"url,omitempty"`
	// The installation action for this template (base64 encoded)
	Action string `json:"action,omitempty"`
	// Icon url, base64 encoded
	Icon string `json:"icon,omitempty"`
	// The default value for helm install
	HelmDefaultValues string `json:"helmDefaultValues,omitempty"`
	// The default namespace for helm install
	HelmDefaultNamespace string `json:"helmDefaultNamespace,omitempty"`
	// Helm status information
	HelmStatus *HelmStatusResponse `json:"helmStatus,omitempty"`
}

type HelmStatusResponse struct {
	// Helm values
	Values string `json:"values,omitempty"`
	// Helm values in YAML format
	ValuesYAML string `json:"valuesYaml,omitempty"`
}
