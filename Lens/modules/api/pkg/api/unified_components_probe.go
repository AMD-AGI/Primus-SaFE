// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	promModel "github.com/prometheus/common/model"
)

var errComponentsProbeStorageNotAvailable = errors.New("Storage client not available for components probe (cannot query VictoriaMetrics)")

// ComponentsProbeRequest is the request for the unified components probe endpoint.
type ComponentsProbeRequest struct {
	Cluster string `query:"cluster" json:"cluster"`
}

// KubeSystemProbePodItem represents one pod in kube-system probe result (optional; VM-based probe does not fill this).
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

// PlatformComponentPodItem represents one pod in platform component probe result (optional; VM-based probe does not fill this).
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
	Cluster    string           `json:"cluster"`
	KubeSystem KubeSystemSection `json:"kubeSystem"`
	Platform   PlatformSection  `json:"platform"`
}

const (
	platformLabelKubeSystem = "kube_system"
	platformLabelPrimusLens = "primus_lens"
	platformLabelPrimusSafe = "primus_safe"
)

func handleComponentsProbe(ctx context.Context, req *ComponentsProbeRequest) (*ComponentsProbeResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}
	if clients == nil || clients.StorageClientSet == nil {
		return nil, errComponentsProbeStorageNotAvailable
	}
	storage := clients.StorageClientSet
	clusterName := clients.ClusterName

	resp := &ComponentsProbeResponse{
		Cluster:    clusterName,
		KubeSystem: KubeSystemSection{Components: nil},
		Platform:   PlatformSection{},
	}

	// Query primus_component_* metrics from VictoriaMetrics (written by component-health-exporter)
	healthyQuery := fmt.Sprintf(`primus_component_healthy{cluster=%q}`, clusterName)
	desiredQuery := fmt.Sprintf(`primus_component_replicas_desired{cluster=%q}`, clusterName)
	readyQuery := fmt.Sprintf(`primus_component_replicas_ready{cluster=%q}`, clusterName)

	healthySamples, err := prom.QueryInstant(ctx, storage, healthyQuery)
	if err != nil {
		return nil, err
	}
	desiredSamples, _ := prom.QueryInstant(ctx, storage, desiredQuery)
	readySamples, _ := prom.QueryInstant(ctx, storage, readyQuery)

	desiredByKey := sampleMapByLabels(desiredSamples)
	readyByKey := sampleMapByLabels(readySamples)

	for _, s := range healthySamples {
		platform := string(s.Metric[promModel.LabelName("platform")])
		appName := string(s.Metric[promModel.LabelName("app_name")])
		namespace := string(s.Metric[promModel.LabelName("namespace")])
		kind := string(s.Metric[promModel.LabelName("kind")])
		healthy := s.Value == 1
		key := labelKey(platform, appName, namespace, kind)
		desired := int32(getSampleValue(desiredByKey[key]))
		ready := int32(getSampleValue(readyByKey[key]))
		total := int(desired)
		if total < int(ready) {
			total = int(ready)
		}

		switch platform {
		case platformLabelKubeSystem:
			displayName := appName
			if appName == "coredns" {
				displayName = "CoreDNS"
			} else if appName == "node-local-dns" {
				displayName = "NodeLocal DNS"
			}
			resp.KubeSystem.Components = append(resp.KubeSystem.Components, KubeSystemProbeComponentItem{
				Name:        appName,
				DisplayName: displayName,
				Kind:        kind,
				Desired:     desired,
				Ready:       ready,
				Healthy:     healthy,
				Pods:        nil,
			})
		case platformLabelPrimusLens:
			resp.Platform.PrimusLens.Components = append(resp.Platform.PrimusLens.Components, PlatformComponentItem{
				AppName:   appName,
				Namespace: namespace,
				Total:     total,
				Ready:     int(ready),
				Healthy:   healthy,
				Pods:      nil,
			})
		case platformLabelPrimusSafe:
			resp.Platform.PrimusSafe.Components = append(resp.Platform.PrimusSafe.Components, PlatformComponentItem{
				AppName:   appName,
				Namespace: namespace,
				Total:     total,
				Ready:     int(ready),
				Healthy:   healthy,
				Pods:      nil,
			})
		}
	}

	return resp, nil
}

func labelKey(platform, appName, namespace, kind string) string {
	return platform + "\t" + appName + "\t" + namespace + "\t" + kind
}

func sampleMapByLabels(samples []*promModel.Sample) map[string]*promModel.Sample {
	m := make(map[string]*promModel.Sample)
	for _, s := range samples {
		m[labelKey(
			string(s.Metric[promModel.LabelName("platform")]),
			string(s.Metric[promModel.LabelName("app_name")]),
			string(s.Metric[promModel.LabelName("namespace")]),
			string(s.Metric[promModel.LabelName("kind")]),
		)] = s
	}
	return m
}

func getSampleValue(s *promModel.Sample) float64 {
	if s == nil {
		return 0
	}
	return float64(s.Value)
}

func init() {
	unified.Register(&unified.EndpointDef[ComponentsProbeRequest, ComponentsProbeResponse]{
		Name:        "components_probe",
		Description: "Get kube-system and platform component health from VictoriaMetrics (data from component-health-exporter)",
		HTTPMethod:  "GET",
		HTTPPath:    "/components/probe",
		MCPToolName: "lens_components_probe",
		Handler:     handleComponentsProbe,
	})
}
