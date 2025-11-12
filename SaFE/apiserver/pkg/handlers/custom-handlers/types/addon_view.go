/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AddonBody struct {
	// The addon name specified by the user
	ReleaseName string `json:"releaseName"`
	// The addon description
	Description string `json:"description,omitempty"`
	// The cluster reference
	// AddonTemplate reference (required)
	Template string `json:"template"`
	// Optional Helm configuration overrides
	// Override target namespace (optional, uses template default namespace by default)
	Namespace string `json:"namespace,omitempty"`
	// Override or merge Helm values in YAML format (optional, uses template values by default)
	Values string `json:"values,omitempty"`
}

type AddonStatus struct {
	// FirstDeployed is when the release was first deployed.
	FirstDeployed metav1.Time `json:"firstDeployed,omitempty"`
	// LastDeployed is when the release was last deployed.
	LastDeployed metav1.Time `json:"lastDeployed,omitempty"`
	// Deleted tracks when this object was deleted.
	Deleted *metav1.Time `json:"deleted,omitempty"`
	// Description is human-friendly "log entry" about this release.
	Description string `json:"description,omitempty"`
	// Status is the current state of the release
	Status string `json:"status,omitempty"`
	// Contains the rendered templates/NOTES.txt if available
	Notes string `json:"notes,omitempty"`
	// Contains the deployed resources information
	//Resources    map[string][]runtime.Object `json:"resources,omitempty"`
	Version         int    `json:"version,omitempty"`
	ChartVersion    string `json:"chartVersion,omitempty"`
	Values          string `json:"values,omitempty"`
	PreviousVersion int    `json:"previousVersion,omitempty"`
}

type AddonResponseBody struct {
	AddonBody
	CreationTime string            `json:"creationTime"`
	Name         string            `json:"name"`
	ReleaseName  string            `json:"releaseName"`
	Cluster      string            `json:"cluster"`
	Phase        v1.AddonPhaseType `json:"phase,omitempty"`
	Status       AddonStatus       `json:"status,omitempty"`
}

type CreateAddonRequestBody struct {
	AddonBody
}

type ListAddonResponse struct {
	// The total number of addons, not limited by pagination
	TotalCount int                 `json:"totalCount"`
	Items      []AddonResponseBody `json:"items"`
}

type PatchAddonRequest struct {
	// The addon description
	Description *string `json:"description,omitempty"`
	// AddonTemplate reference (required)
	Template *string `json:"template,omitempty"`
	// Override or merge Helm values in YAML format (optional, uses template values by default)
	Values *string `json:"values,omitempty"`
}
