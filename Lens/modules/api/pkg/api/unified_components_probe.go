// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ComponentsProbeRequest is the request for the unified components probe endpoint.
type ComponentsProbeRequest struct {
	Cluster string `query:"cluster" json:"cluster"`
}

// KubeSystemProbePodItem represents one pod in kube-system probe result.
type KubeSystemProbePodItem struct {
	Name  string `json:"name"`
	Node  string `json:"node"`
	Ready bool   `json:"ready"`
}

// KubeSystemProbeComponentItem represents one kube-system component (e.g. coredns, node-local-dns).
type KubeSystemProbeComponentItem struct {
	Name        string                   `json:"name"`
	DisplayName string                   `json:"displayName"`
	Kind        string                   `json:"kind"`
	Desired     int32                    `json:"desired"`
	Ready       int32                    `json:"ready"`
	Healthy     bool                     `json:"healthy"`
	Pods        []KubeSystemProbePodItem `json:"pods,omitempty"`
}

// PlatformComponentPodItem represents one pod in platform component probe result.
type PlatformComponentPodItem struct {
	Name  string `json:"name"`
	Node  string `json:"node"`
	Ready bool   `json:"ready"`
}

// PlatformComponentItem represents one platform component (Primus-SaFE or Primus-Lens).
type PlatformComponentItem struct {
	AppName   string                     `json:"appName"`
	Namespace string                     `json:"namespace"`
	Total     int                        `json:"total"`
	Ready     int                        `json:"ready"`
	Healthy   bool                       `json:"healthy"`
	Pods      []PlatformComponentPodItem `json:"pods,omitempty"`
}

// KubeSystemSection contains kube-system core components probe result.
type KubeSystemSection struct {
	Components []KubeSystemProbeComponentItem `json:"components"`
}

// PlatformSection contains Primus-SaFE and Primus-Lens components.
type PlatformSection struct {
	PrimusSafe struct {
		Components []PlatformComponentItem `json:"components"`
	} `json:"primusSafe"`
	PrimusLens struct {
		Components []PlatformComponentItem `json:"components"`
	} `json:"primusLens"`
}

// ComponentsProbeResponse is the response for the unified components probe endpoint.
type ComponentsProbeResponse struct {
	Cluster    string            `json:"cluster"`
	KubeSystem KubeSystemSection `json:"kubeSystem"`
	Platform   PlatformSection   `json:"platform"`
}

func handleComponentsProbe(ctx context.Context, req *ComponentsProbeRequest) (*ComponentsProbeResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}
	raw, err := rc.GetRaw(ctx, "/components/health", nil)
	if err != nil {
		return nil, fmt.Errorf("robust components health: %w", err)
	}
	var resp ComponentsProbeResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("robust components health decode: %w", err)
	}
	return &resp, nil
}

func init() {
	unified.Register(&unified.EndpointDef[ComponentsProbeRequest, ComponentsProbeResponse]{
		Name:        "components_probe",
		Description: "Get kube-system and platform component health from Robust API",
		HTTPMethod:  "GET",
		HTTPPath:    "/components/probe",
		MCPToolName: "lens_components_probe",
		Handler:     handleComponentsProbe,
	})
}
