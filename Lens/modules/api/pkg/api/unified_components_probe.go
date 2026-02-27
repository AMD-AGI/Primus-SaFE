// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"errors"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

var errComponentsProbeK8sNotAvailable = errors.New("K8s client not available for components probe")

// ComponentsProbeRequest is the request for the unified components probe endpoint.
type ComponentsProbeRequest struct {
	Cluster string `query:"cluster" json:"cluster"`
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
	Cluster    string           `json:"cluster"`
	KubeSystem KubeSystemSection `json:"kubeSystem"`
	Platform   PlatformSection   `json:"platform"`
}

func handleComponentsProbe(ctx context.Context, req *ComponentsProbeRequest) (*ComponentsProbeResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}
	if clients == nil || clients.K8SClientSet == nil || clients.K8SClientSet.ControllerRuntimeClient == nil {
		return nil, errComponentsProbeK8sNotAvailable
	}
	c := clients.K8SClientSet.ControllerRuntimeClient
	clusterName := clients.ClusterName

	resp := &ComponentsProbeResponse{
		Cluster: clusterName,
		KubeSystem: KubeSystemSection{Components: nil},
		Platform:   PlatformSection{},
	}

	if item, err := probeCoreDNS(ctx, c, clusterName); err == nil {
		resp.KubeSystem.Components = append(resp.KubeSystem.Components, item)
	}
	if item, err := probeNodeLocalDNS(ctx, c, clusterName); err == nil {
		resp.KubeSystem.Components = append(resp.KubeSystem.Components, item)
	}

	safeList, _ := listComponentsByLabel(ctx, c, labelPrimusSafeAppName)
	resp.Platform.PrimusSafe.Components = safeList
	lensList, _ := listComponentsByLabel(ctx, c, labelPrimusLensAppName)
	resp.Platform.PrimusLens.Components = lensList

	return resp, nil
}

func init() {
	unified.Register(&unified.EndpointDef[ComponentsProbeRequest, ComponentsProbeResponse]{
		Name:        "components_probe",
		Description: "Get kube-system core components (CoreDNS, NodeLocal DNS) and Primus-SaFE/Primus-Lens platform components liveness (single endpoint for agents)",
		HTTPMethod:  "GET",
		HTTPPath:    "/components/probe",
		MCPToolName: "lens_components_probe",
		Handler:     handleComponentsProbe,
	})
}
