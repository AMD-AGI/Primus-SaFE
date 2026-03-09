// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pipeline

import (
	"encoding/json"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
)

// WorkloadJSON is the unified evidence document sent to the Python intent-service.
type WorkloadJSON struct {
	WorkloadUID  string                 `json:"workload_uid"`
	ClusterID    string                 `json:"cluster_id,omitempty"`
	Namespace    string                 `json:"namespace,omitempty"`
	WorkloadName string                 `json:"workload_name,omitempty"`
	WorkloadKind string                 `json:"workload_kind,omitempty"`
	GPUCount     int                    `json:"gpu_count,omitempty"`
	NodeCount    int                    `json:"node_count,omitempty"`
	Labels       map[string]string      `json:"labels,omitempty"`
	Annotations  map[string]string      `json:"annotations,omitempty"`
	Containers   []ContainerJSON        `json:"containers,omitempty"`
	Processes    []ProcessJSON          `json:"processes,omitempty"`
	FrameworkEvidence map[string]interface{} `json:"framework_evidence,omitempty"`
}

// ContainerJSON holds container info for WorkloadJSON.
type ContainerJSON struct {
	Name      string            `json:"name,omitempty"`
	Image     string            `json:"image,omitempty"`
	ImageInfo *ImageInfoJSON    `json:"image_info,omitempty"`
	Command   []string          `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Resources map[string]interface{} `json:"resources,omitempty"`
}

// ImageInfoJSON holds image analysis results for WorkloadJSON.
type ImageInfoJSON struct {
	FullRef           string                 `json:"full_ref,omitempty"`
	Registry          string                 `json:"registry,omitempty"`
	Repository        string                 `json:"repository,omitempty"`
	Tag               string                 `json:"tag,omitempty"`
	Digest            string                 `json:"digest,omitempty"`
	BaseImage         string                 `json:"base_image,omitempty"`
	LayerCount        int                    `json:"layer_count,omitempty"`
	LayerHistory      []LayerHistoryJSON     `json:"layer_history,omitempty"`
	ImageLabels       map[string]string      `json:"image_labels,omitempty"`
	ImageEnv          map[string]string      `json:"image_env,omitempty"`
	ImageEntrypoint   string                 `json:"image_entrypoint,omitempty"`
	InstalledPackages []string               `json:"installed_packages,omitempty"`
	FrameworkHints    map[string]interface{} `json:"framework_hints,omitempty"`
}

// LayerHistoryJSON is a single layer history entry.
type LayerHistoryJSON struct {
	Command    string   `json:"command,omitempty"`
	SizeBytes  int64    `json:"size_bytes,omitempty"`
	Packages   []string `json:"packages,omitempty"`
	PipPackages []string `json:"pip_packages,omitempty"`
	EnvVars    map[string]string `json:"env_vars,omitempty"`
}

// ProcessJSON holds process info for WorkloadJSON.
type ProcessJSON struct {
	PID     int    `json:"pid,omitempty"`
	Cmdline string `json:"cmdline,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
}

// BuildWorkloadJSON constructs a WorkloadJSON from IntentEvidence.
func BuildWorkloadJSON(
	workloadUID string,
	clusterID string,
	evidence *intent.IntentEvidence,
	gpuCount int,
	replicas int,
) (*WorkloadJSON, error) {
	if evidence == nil {
		return nil, fmt.Errorf("evidence is nil")
	}

	wj := &WorkloadJSON{
		WorkloadUID:  workloadUID,
		ClusterID:    clusterID,
		Namespace:    evidence.WorkloadNamespace,
		WorkloadName: evidence.WorkloadName,
		WorkloadKind: evidence.WorkloadKind,
		GPUCount:     gpuCount,
		NodeCount:    replicas,
		Labels:       evidence.Labels,
	}

	container := ContainerJSON{
		Image:   evidence.Image,
		Command: evidence.Args,
		Env:     evidence.Env,
	}

	if evidence.ImageRegistry != nil {
		reg := evidence.ImageRegistry
		imgInfo := &ImageInfoJSON{
			FullRef:        evidence.Image,
			Digest:         reg.Digest,
			BaseImage:      reg.BaseImage,
			FrameworkHints: reg.FrameworkHints,
			ImageEnv:       make(map[string]string),
			ImageLabels:    make(map[string]string),
		}

		for _, layer := range reg.LayerHistory {
			imgInfo.LayerHistory = append(imgInfo.LayerHistory, LayerHistoryJSON{
				Command: layer.CreatedBy,
			})
		}

		for _, pkg := range reg.InstalledPackages {
			imgInfo.InstalledPackages = append(imgInfo.InstalledPackages,
				pkg.Name+"=="+pkg.Version)
		}

		container.ImageInfo = imgInfo
	}

	wj.Containers = []ContainerJSON{container}

	if evidence.Command != "" {
		wj.Processes = []ProcessJSON{{Cmdline: evidence.Command}}
	}

	return wj, nil
}

// MarshalWorkloadJSON serializes a WorkloadJSON to JSON bytes.
func MarshalWorkloadJSON(wj *WorkloadJSON) ([]byte, error) {
	return json.Marshal(wj)
}
