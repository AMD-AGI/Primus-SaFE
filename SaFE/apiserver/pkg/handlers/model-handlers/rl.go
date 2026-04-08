/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

// CreateRlJob handles POST /api/v1/rl/jobs
func (h *Handler) CreateRlJob(c *gin.Context) {
	handle(c, h.createRlJob)
}

// GetRlConfig handles GET /api/v1/playground/models/:id/rl-config
func (h *Handler) GetRlConfig(c *gin.Context) {
	handle(c, h.getRlConfig)
}

func (h *Handler) createRlJob(c *gin.Context) (interface{}, error) {
	var req CreateRlJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request body: %v", err))
	}

	ctx := c.Request.Context()

	// Step 1: Resolve model from Model Square
	model, err := h.dbClient.GetModelByID(ctx, req.ModelId)
	if err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("model not found: %s", req.ModelId))
	}
	hfModelName := extractHfModelName(model)

	// Step 2: Determine model size for defaults (best-effort, fallback to 8b)
	modelSize := inferModelSize(hfModelName)

	// Step 3: Fill smart defaults
	FillRlDefaults(&req, modelSize)

	// Step 4: Resolve dataset path
	datasetPath, err := h.resolveDatasetPath(ctx, req.DatasetId, req.Workspace)
	if err != nil {
		return nil, err
	}

	// Step 5: Resolve model local path (verl needs HF model on shared storage)
	modelPath, err := h.resolveModelLocalPath(ctx, req.ModelId, req.Workspace)
	if err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("model not available locally: %v", err))
	}

	// Step 6: Generate workload name
	workloadName := generateRlWorkloadName(req.DisplayName)
	pfsBasePath := "/wekafs"
	var workspaceObj *v1.Workspace
	if req.Workspace != "" {
		ws := &v1.Workspace{}
		if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: req.Workspace}, ws); err == nil {
			workspaceObj = ws
			if p := commonworkspace.GetNfsPathFromWorkspace(ws); p != "" {
				pfsBasePath = p
			}
		}
	}
	exportPath := fmt.Sprintf("%s/custom/models/%s", pfsBasePath, workloadName)

	// Step 7: Build training script
	trainScript := BuildRlTrainScript(RlEntrypointConfig{
		ModelPath:   modelPath,
		ModelName:   hfModelName,
		DatasetPath: datasetPath,
		NodeCount:   req.NodeCount,
		GpuCount:    req.GpuCount,
		TrainConfig: req.TrainConfig,
		ExportModel: *req.ExportModel,
		ExportPath:  exportPath,
		Workspace:   req.Workspace,
		ModelId:     req.ModelId,
		BaseModel:   hfModelName,
		RlJobId:     workloadName,
		ExpName:     req.DisplayName,
	})

	// Step 8: Build container entrypoints (head writes training script, worker is minimal)
	headInit := BuildRlContainerEntrypoint(trainScript, true)
	workerInit := BuildRlContainerEntrypoint("", false)
	encodedHeadInit := base64.StdEncoding.EncodeToString([]byte(headInit))
	encodedWorkerInit := base64.StdEncoding.EncodeToString([]byte(workerInit))

	// Step 9: Build env
	userId := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)
	env := map[string]string{
		common.RayJobEntrypoint:  "bash /tmp/rl_train.sh",
		"PYTORCH_HIP_ALLOC_CONF": "expandable_segments:True",
		"RL_USER_ID":             userId,
		"RL_USER_NAME":           userName,
		"RL_ALGORITHM":           req.TrainConfig.Algorithm,
		"RL_TRAIN_BATCH_SIZE":    strconv.Itoa(req.TrainConfig.TrainBatchSize),
		"RL_MINI_BATCH_SIZE":     strconv.Itoa(req.TrainConfig.MiniPatchSize),
	}
	if shouldApplyAinicEnv(workspaceObj) && req.GpuCount > 0 {
		applyAinicWorkloadEnv(env)
	}
	for k, v := range req.Env {
		env[k] = v
	}

	// Step 10: Build labels and annotations
	rlLabels := map[string]string{
		SftLabelWorkloadType: RlWorkloadTypeValue,
		RlLabelAlgorithm:     req.TrainConfig.Algorithm,
		RlLabelRewardType:    req.TrainConfig.RewardType,
		RlLabelBaseModelId:   req.ModelId,
		RlLabelUserId:        userId,
	}
	rlAnnotations := map[string]string{
		SftLabelModel:   hfModelName,
		SftLabelDataset: req.DatasetId,
		RlLabelUserName: userName,
	}

	// Step 11: Build resources — RayJob: head (index 0) + workers (index 1)
	nodeResource := v1.WorkloadResource{
		CPU:              req.Cpu,
		GPU:              strconv.Itoa(req.GpuCount),
		Memory:           req.Memory,
		SharedMemory:     req.SharedMemory,
		EphemeralStorage: req.EphemeralStorage,
		RdmaResource:     DefaultRdmaResource,
	}

	headResource := nodeResource
	headResource.Replica = 1

	workerResource := nodeResource
	workerResource.Replica = req.NodeCount - 1
	if workerResource.Replica < 1 {
		workerResource.Replica = 1
	}

	resources := []v1.WorkloadResource{headResource, workerResource}
	images := []string{req.Image, req.Image}
	entryPoints := []string{encodedHeadInit, encodedWorkerInit}

	// Step 12: Create RayJob workload
	workload := &v1.Workload{}
	workload.Name = workloadName
	workload.Labels = map[string]string{
		v1.DisplayNameLabel: req.DisplayName,
		v1.UserIdLabel:      userId,
	}
	for k, v := range rlLabels {
		workload.Labels[k] = v
	}
	workload.Annotations = map[string]string{
		v1.DescriptionAnnotation: req.Description,
		v1.UserNameAnnotation:    userName,
	}
	for k, v := range rlAnnotations {
		workload.Annotations[k] = v
	}

	timeout := 0
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	workload.Spec = v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.RayJobKind, Version: common.DefaultVersion},
		Images:           images,
		EntryPoints:      entryPoints,
		Resources:        resources,
		Env:              env,
		Priority:         req.Priority,
		Workspace:        req.Workspace,
	}
	if timeout > 0 {
		workload.Spec.Timeout = &timeout
	}

	if err := h.k8sClient.Create(ctx, workload); err != nil {
		klog.ErrorS(err, "failed to create RL workload", "name", workload.Name)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to create workload: %v", err))
	}

	klog.InfoS("created RL training job",
		"workloadId", workload.Name,
		"model", hfModelName,
		"dataset", req.DatasetId,
		"algorithm", req.TrainConfig.Algorithm,
		"nodes", req.NodeCount,
	)

	return &CreateRlJobResponse{
		WorkloadId: workload.Name,
	}, nil
}

func (h *Handler) getRlConfig(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	var query GetRlConfigQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid query: %v", err))
	}

	ctx := c.Request.Context()
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, commonerrors.NewNotFound("model", modelId)
		}
		return nil, commonerrors.NewInternalError("failed to fetch model: " + err.Error())
	}

	hfModelName := extractHfModelNameFromURLOrModelName(k8sModel.Spec.Source.URL, k8sModel.Spec.Source.ModelName)
	resp := &RlConfigResponse{
		Supported: false,
		Model: RlConfigModelInfo{
			ID:          k8sModel.Name,
			DisplayName: k8sModel.Spec.DisplayName,
			ModelName:   hfModelName,
			AccessMode:  string(k8sModel.Spec.Source.AccessMode),
			Phase:       string(k8sModel.Status.Phase),
			Workspace:   k8sModel.Spec.Workspace,
		},
		DatasetFilter: RlConfigDatasetFilter{
			DatasetType: "rlhf",
			Workspace:   query.Workspace,
			Status:      "Ready",
		},
		Options: RlConfigOptions{
			AlgorithmOptions:  []string{"grpo", "ppo"},
			StrategyOptions:   []string{"fsdp2", "megatron"},
			RewardTypeOptions: []string{"math", "custom"},
			PriorityOptions:   getSftPriorityOptions(),
		},
	}

	// RL requires a local model in HuggingFace format
	if k8sModel.Spec.Source.AccessMode != v1.AccessModeLocal && k8sModel.Spec.Source.AccessMode != v1.AccessModeLocalPath {
		resp.Reason = "RL training requires a local model (HuggingFace format on shared storage)"
		return resp, nil
	}
	if k8sModel.Status.Phase != v1.ModelPhaseReady {
		resp.Reason = fmt.Sprintf("model is not ready, current phase: %s", k8sModel.Status.Phase)
		return resp, nil
	}

	modelSize := inferModelSize(hfModelName)
	defaultReq := CreateRlJobRequest{
		Workspace: query.Workspace,
		ModelId:   modelId,
	}
	if query.Strategy != "" {
		defaultReq.TrainConfig.Strategy = query.Strategy
	}
	FillRlDefaults(&defaultReq, modelSize)

	resp.Supported = true
	resp.Defaults = &RlConfigDefaults{
		ExportModel:      *defaultReq.ExportModel,
		Image:            defaultReq.Image,
		NodeCount:        defaultReq.NodeCount,
		GpuCount:         defaultReq.GpuCount,
		Cpu:              defaultReq.Cpu,
		Memory:           defaultReq.Memory,
		SharedMemory:     defaultReq.SharedMemory,
		EphemeralStorage: defaultReq.EphemeralStorage,
		Priority:         defaultReq.Priority,
		TrainConfig:      defaultReq.TrainConfig,
	}

	return resp, nil
}

// ==================== Helpers ====================

// resolveModelLocalPath finds the shared-storage path for a model in the given workspace.
func (h *Handler) resolveModelLocalPath(ctx context.Context, modelId, workspace string) (string, error) {
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		return "", fmt.Errorf("model %s not found in k8s: %v", modelId, err)
	}
	return resolveModelLocalPathFromK8sModel(k8sModel, workspace)
}

// inferModelSize guesses model size from name string for default parameter selection.
func inferModelSize(hfModelName string) string {
	lower := strings.ToLower(hfModelName)
	if strings.Contains(lower, "70b") || strings.Contains(lower, "72b") || strings.Contains(lower, "65b") {
		return "70b"
	}
	if strings.Contains(lower, "32b") || strings.Contains(lower, "30b") || strings.Contains(lower, "34b") {
		return "32b"
	}
	return "8b"
}

func generateRlWorkloadName(displayName string) string {
	name := strings.ToLower(displayName)
	name = strings.ReplaceAll(name, " ", "-")
	if len(name) > 40 {
		name = name[:40]
	}
	return fmt.Sprintf("rl-%s-%d", name, time.Now().UnixMilli()%100000)
}
