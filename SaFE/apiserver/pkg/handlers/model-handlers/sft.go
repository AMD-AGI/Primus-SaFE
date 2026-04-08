/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

// CreateSftJob handles POST /api/v1/sft/jobs
func (h *Handler) CreateSftJob(c *gin.Context) {
	handle(c, h.createSftJob)
}

// GetSftConfig handles GET /api/v1/playground/models/:id/sft-config
func (h *Handler) GetSftConfig(c *gin.Context) {
	handle(c, h.getSftConfig)
}

func (h *Handler) createSftJob(c *gin.Context) (interface{}, error) {
	var req CreateSftJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request body: %v", err))
	}

	ctx := c.Request.Context()

	// Step 1: Resolve model from Model Square
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: req.ModelId}, k8sModel); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("model not found: %s", req.ModelId))
		}
		return nil, commonerrors.NewInternalError("failed to fetch model: " + err.Error())
	}
	if k8sModel.Spec.Source.AccessMode != v1.AccessModeLocal &&
		k8sModel.Spec.Source.AccessMode != v1.AccessModeLocalPath {
		return nil, commonerrors.NewBadRequest("SFT training requires a local or local_path model on shared storage")
	}
	if k8sModel.Status.Phase != v1.ModelPhaseReady {
		return nil, commonerrors.NewBadRequest(
			fmt.Sprintf("model is not ready, current phase: %s", k8sModel.Status.Phase),
		)
	}
	selectedModelName := extractHfModelNameFromURLOrModelName(
		k8sModel.Spec.Source.URL,
		k8sModel.Spec.Source.ModelName,
	)
	if selectedModelName == "" {
		selectedModelName = k8sModel.Spec.DisplayName
	}
	baseModelName := resolveTrainingBaseModelNameFromK8sModel(k8sModel)
	modelPath, err := resolveModelLocalPathFromK8sModel(k8sModel, req.Workspace)
	if err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("model not available locally: %v", err))
	}

	// Step 2: Infer recipe/flavor from model name
	recipe, err := InferModelRecipe(baseModelName)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	// Step 3: Fill smart defaults
	FillSftDefaults(&req, recipe.Size)

	// Step 4: Resolve dataset path
	datasetPath, err := h.resolveDatasetPath(ctx, req.DatasetId, req.Workspace)
	if err != nil {
		return nil, err
	}

	// Step 5: Build workload name (needed for export)
	workloadName := generateSftWorkloadName(req.DisplayName)

	// Step 5.5: Resolve PFS base path from workspace (e.g. /wekafs, /shared_nfs)
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

	// Step 6: Build entrypoint script
	entrypoint := BuildEntrypoint(EntrypointConfig{
		PrimusPath:    DefaultPrimusPath,
		Recipe:        recipe.Recipe,
		Flavor:        recipe.Flavor,
		HfPath:        modelPath,
		DatasetPath:   datasetPath,
		DatasetFormat: req.TrainConfig.DatasetFormat,
		ExpName:       req.DisplayName,
		ModelSize:     recipe.Size,
		TrainConfig:   req.TrainConfig,
		ExportModel:   *req.ExportModel,
		Workspace:     req.Workspace,
		ModelId:       req.ModelId,
		BaseModel:     baseModelName,
		SftJobId:      workloadName,
		PfsBasePath:   pfsBasePath,
	})

	encodedEntrypoint := base64.StdEncoding.EncodeToString([]byte(entrypoint))

	// Step 7: Build env
	env := map[string]string{
		"PYTORCH_HIP_ALLOC_CONF": "expandable_segments:True",
		"GPUS_PER_NODE":          strconv.Itoa(req.GpuCount),
	}
	if shouldApplyAinicEnv(workspaceObj) && req.GpuCount > 0 {
		applyAinicWorkloadEnv(env)
	}
	if req.NodeCount > 1 {
		env["NNODES"] = strconv.Itoa(req.NodeCount)
		env["DATA_PATH"] = fmt.Sprintf("%s/sft-shared-data/%s", pfsBasePath, workloadName)
	}
	for k, v := range req.Env {
		env[k] = v
	}

	// Step 8: Build SFT metadata
	userId := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)
	env["SFT_USER_ID"] = userId
	env["SFT_USER_NAME"] = userName
	sftLabels := map[string]string{
		SftLabelWorkloadType: SftWorkloadTypeValue,
		SftLabelPeft:         req.TrainConfig.Peft,
		SftLabelBaseModelId:  req.ModelId,
		SftLabelUserId:       userId,
	}
	sftAnnotations := map[string]string{
		SftLabelModel:    selectedModelName,
		SftLabelDataset:  req.DatasetId,
		SftLabelUserName: userName,
	}

	// Step 9: Build resources — PyTorchJob splits into master + workers
	nodeResource := v1.WorkloadResource{
		Replica:          1,
		CPU:              req.Cpu,
		GPU:              strconv.Itoa(req.GpuCount),
		Memory:           req.Memory,
		SharedMemory:     req.SharedMemory,
		EphemeralStorage: req.EphemeralStorage,
	}

	var resources []v1.WorkloadResource
	var images []string
	var entryPoints []string

	if req.NodeCount > 1 {
		nodeResource.RdmaResource = DefaultRdmaResource
		masterResource := nodeResource
		masterResource.Replica = 1
		workerResource := nodeResource
		workerResource.Replica = req.NodeCount - 1
		resources = []v1.WorkloadResource{masterResource, workerResource}
		images = []string{req.Image, req.Image}
		entryPoints = []string{encodedEntrypoint, encodedEntrypoint}
	} else {
		resources = []v1.WorkloadResource{nodeResource}
		images = []string{req.Image}
		entryPoints = []string{encodedEntrypoint}
	}

	// Step 10: Create PyTorchJob workload
	workload := &v1.Workload{}
	workload.Name = workloadName
	workload.Labels = map[string]string{
		v1.DisplayNameLabel: req.DisplayName,
		v1.UserIdLabel:      userId,
	}
	for k, v := range sftLabels {
		workload.Labels[k] = v
	}
	workload.Annotations = map[string]string{
		v1.DescriptionAnnotation: req.Description,
		v1.UserNameAnnotation:    userName,
	}
	for k, v := range sftAnnotations {
		workload.Annotations[k] = v
	}
	v1.SetAnnotation(workload, v1.UseWorkspaceStorageAnnotation, v1.TrueStr)
	if req.ForceHostNetwork {
		v1.SetAnnotation(workload, v1.ForceHostNetworkAnnotation, v1.TrueStr)
	}

	timeout := 0
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	workload.Spec = v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind, Version: common.DefaultVersion},
		Images:           images,
		EntryPoints:      entryPoints,
		Resources:        resources,
		Env:              env,
		Hostpath:         req.Hostpath,
		Priority:         req.Priority,
		Workspace:        req.Workspace,
	}
	if timeout > 0 {
		workload.Spec.Timeout = &timeout
	}

	if err := h.k8sClient.Create(ctx, workload); err != nil {
		klog.ErrorS(err, "failed to create SFT workload", "name", workload.Name)
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to create workload: %v", err))
	}

	klog.InfoS("created SFT training job",
		"workloadId", workload.Name,
		"model", selectedModelName,
		"modelPath", modelPath,
		"baseModel", baseModelName,
		"dataset", req.DatasetId,
		"peft", req.TrainConfig.Peft,
	)

	return &CreateSftJobResponse{
		WorkloadId: workload.Name,
	}, nil
}

func (h *Handler) getSftConfig(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	var query GetSftConfigQuery
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

	trainingModelName := resolveTrainingBaseModelNameFromK8sModel(k8sModel)
	resp := &SftConfigResponse{
		Supported: false,
		Model: SftConfigModelInfo{
			ID:          k8sModel.Name,
			DisplayName: k8sModel.Spec.DisplayName,
			ModelName:   trainingModelName,
			AccessMode:  string(k8sModel.Spec.Source.AccessMode),
			Phase:       string(k8sModel.Status.Phase),
			Workspace:   k8sModel.Spec.Workspace,
			MaxTokens:   k8sModel.Spec.MaxTokens,
		},
		DatasetFilter: SftConfigDatasetFilter{
			DatasetType: "sft",
			Workspace:   query.Workspace,
			Status:      "Ready",
		},
		Options: SftConfigOptions{
			DatasetFormatOptions: getSupportedDatasetFormats(),
			PriorityOptions:      getSftPriorityOptions(),
		},
	}

	if k8sModel.Spec.Source.AccessMode != v1.AccessModeLocal &&
		k8sModel.Spec.Source.AccessMode != v1.AccessModeLocalPath {
		resp.Reason = "only local or local_path models can be fine-tuned"
		return resp, nil
	}

	if k8sModel.Status.Phase != v1.ModelPhaseReady {
		resp.Reason = fmt.Sprintf("model is not ready, current phase: %s", k8sModel.Status.Phase)
		return resp, nil
	}

	recipe, err := InferModelRecipe(trainingModelName)
	if err != nil {
		resp.Reason = err.Error()
		return resp, nil
	}

	defaultReq := CreateSftJobRequest{
		Workspace: query.Workspace,
		ModelId:   modelId,
	}
	if query.Peft != "" {
		defaultReq.TrainConfig.Peft = query.Peft
	}
	FillSftDefaults(&defaultReq, recipe.Size)

	resp.Supported = true
	resp.Defaults = &SftConfigDefaults{
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
	resp.Options.PeftOptions = getSupportedPeftOptions(recipe.Size)

	return resp, nil
}

// ==================== Helpers ====================

func (h *Handler) resolveDatasetPath(ctx context.Context, datasetId, workspace string) (string, error) {
	dataset, err := h.dbClient.GetDataset(ctx, datasetId)
	if err != nil {
		return "", commonerrors.NewBadRequest(fmt.Sprintf("dataset not found: %s", datasetId))
	}

	if dataset.LocalPaths != "" {
		var localPaths []dbclient.DatasetLocalPathDB
		if json.Unmarshal([]byte(dataset.LocalPaths), &localPaths) == nil {
			for _, lp := range localPaths {
				if lp.Workspace == workspace && lp.Status == dbclient.DatasetStatusReady {
					return lp.Path, nil
				}
			}
			for _, lp := range localPaths {
				if lp.Status == dbclient.DatasetStatusReady {
					return lp.Path, nil
				}
			}
		}
	}

	if dataset.S3Path != "" {
		return dataset.S3Path, nil
	}

	return "", commonerrors.NewBadRequest(fmt.Sprintf("dataset %s has no available path", datasetId))
}

func extractHfModelName(model *dbclient.Model) string {
	return extractHfModelNameFromURLOrModelName(model.SourceURL, model.ModelName)
}

func extractHfModelNameFromURLOrModelName(url, modelName string) string {
	url = strings.TrimPrefix(url, "https://huggingface.co/")
	url = strings.TrimSuffix(url, "/")
	if url != "" {
		return url
	}
	return modelName
}

func resolveTrainingBaseModelNameFromK8sModel(k8sModel *v1.Model) string {
	if k8sModel.Spec.Source.AccessMode == v1.AccessModeLocalPath && k8sModel.Spec.BaseModel != "" {
		return k8sModel.Spec.BaseModel
	}
	name := extractHfModelNameFromURLOrModelName(k8sModel.Spec.Source.URL, k8sModel.Spec.Source.ModelName)
	if name != "" {
		return name
	}
	return k8sModel.Spec.DisplayName
}

func resolveModelLocalPathFromK8sModel(k8sModel *v1.Model, workspace string) (string, error) {
	if k8sModel.Spec.Source.AccessMode == v1.AccessModeLocalPath && k8sModel.Spec.Source.LocalPath != "" {
		return k8sModel.Spec.Source.LocalPath, nil
	}

	for _, lp := range k8sModel.Status.LocalPaths {
		if lp.Status == v1.LocalPathStatusReady {
			if lp.Workspace == workspace || workspace == "" {
				return lp.Path, nil
			}
		}
	}

	for _, lp := range k8sModel.Status.LocalPaths {
		if lp.Status == v1.LocalPathStatusReady {
			return lp.Path, nil
		}
	}

	return "", fmt.Errorf("no local path available for model %s in workspace %s", k8sModel.Name, workspace)
}

func getSupportedDatasetFormats() []string {
	return []string{"alpaca"}
}

func getSupportedPeftOptions(modelSize string) []string {
	presets, ok := trainPresets[modelSize]
	if !ok {
		presets = trainPresets["8b"]
	}

	ordered := []string{"none", "lora"}
	var options []string
	for _, option := range ordered {
		if _, exists := presets[option]; exists {
			options = append(options, option)
		}
	}
	return options
}

func getSftPriorityOptions() []SftConfigPriorityRef {
	return []SftConfigPriorityRef{
		{Label: "Low", Value: common.LowPriorityInt},
		{Label: "Medium", Value: common.MedPriorityInt},
		{Label: "High", Value: common.HighPriorityInt},
	}
}

func generateSftWorkloadName(displayName string) string {
	name := strings.ToLower(displayName)
	name = strings.ReplaceAll(name, " ", "-")
	if len(name) > 40 {
		name = name[:40]
	}
	return fmt.Sprintf("sft-%s-%d", name, time.Now().UnixMilli()%100000)
}
