// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package action_task_executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Action type constants
const (
	ActionTypeGetProcessTree = "get_process_tree"
)

// ProcessTreeParams represents parameters for get_process_tree action
type ProcessTreeParams struct {
	PodUID           string `json:"pod_uid"`
	PodName          string `json:"pod_name"`
	PodNamespace     string `json:"pod_namespace"`
	IncludeEnv       bool   `json:"include_env"`
	IncludeCmdline   bool   `json:"include_cmdline"`
	IncludeResources bool   `json:"include_resources"`
	IncludeGPU       bool   `json:"include_gpu"`
}

// RegisterDefaultHandlers registers all default action handlers
func RegisterDefaultHandlers(executor *ActionTaskExecutor) {
	executor.RegisterHandler(ActionTypeGetProcessTree, HandleGetProcessTree)
	// Add more handlers here as needed:
	// executor.RegisterHandler("pyspy_sample", HandlePySpySample)
	// executor.RegisterHandler("trigger_diag", HandleTriggerDiag)
}

// HandleGetProcessTree handles the get_process_tree action
// It calls the local node-exporter to get the process tree for a pod
func HandleGetProcessTree(ctx context.Context, task *model.ActionTasks, k8sClient *clientsets.K8SClientSet) (interface{}, error) {
	// Parse parameters
	var params ProcessTreeParams
	if task.Parameters != nil {
		paramsBytes, err := json.Marshal(task.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal parameters: %w", err)
		}
		if err := json.Unmarshal(paramsBytes, &params); err != nil {
			return nil, fmt.Errorf("failed to parse parameters: %w", err)
		}
	}

	if task.TargetNode == "" {
		return nil, fmt.Errorf("target_node is required for get_process_tree")
	}

	log.Infof("Getting process tree for pod %s on node %s", params.PodUID, task.TargetNode)

	// Get node-exporter client for the target node
	nodeExporterClient, err := clientsets.GetOrInitNodeExportersClient(
		ctx,
		task.TargetNode,
		k8sClient.ControllerRuntimeClient,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get node-exporter client for node %s: %w", task.TargetNode, err)
	}

	// Build request body for node-exporter
	nodeExporterReq := map[string]interface{}{
		"pod_name":          params.PodName,
		"pod_namespace":     params.PodNamespace,
		"pod_uid":           params.PodUID,
		"include_env":       params.IncludeEnv,
		"include_cmdline":   params.IncludeCmdline,
		"include_resources": params.IncludeResources,
		"include_gpu":       params.IncludeGPU,
	}

	// Call node-exporter process-tree API
	resp, err := nodeExporterClient.GetRestyClient().R().
		SetContext(ctx).
		SetBody(nodeExporterReq).
		Post("/v1/process-tree/pod")

	if err != nil {
		return nil, fmt.Errorf("failed to call node-exporter: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode(), resp.String())
	}

	// Parse response from node-exporter
	var nodeExporterResp struct {
		Meta struct {
			Code    int    `json:"code"`
			Message string `json:"message,omitempty"`
		} `json:"meta"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(resp.Body(), &nodeExporterResp); err != nil {
		return nil, fmt.Errorf("failed to parse node-exporter response: %w", err)
	}

	// Node-exporter uses code 2000 for success, 0 is also acceptable
	if nodeExporterResp.Meta.Code != 0 && nodeExporterResp.Meta.Code != 2000 {
		return nil, fmt.Errorf("node-exporter error (code %d): %s", nodeExporterResp.Meta.Code, nodeExporterResp.Meta.Message)
	}

	// Return the data directly (it's already in the correct format)
	var result interface{}
	if err := json.Unmarshal(nodeExporterResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse process tree data: %w", err)
	}

	log.Infof("Successfully retrieved process tree for pod %s on node %s", params.PodUID, task.TargetNode)
	return result, nil
}
