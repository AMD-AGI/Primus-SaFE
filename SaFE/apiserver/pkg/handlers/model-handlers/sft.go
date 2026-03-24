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
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// CreateSftJob handles POST /api/v1/sft/jobs
func (h *Handler) CreateSftJob(c *gin.Context) {
	handle(c, h.createSftJob)
}

func (h *Handler) createSftJob(c *gin.Context) (interface{}, error) {
	var req CreateSftJobRequest
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

	// Step 2: Infer recipe/flavor from model name
	recipe, err := InferModelRecipe(hfModelName)
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

	// Step 5: Build entrypoint script
	entrypoint := BuildEntrypoint(EntrypointConfig{
		PrimusPath:    DefaultPrimusPath,
		Recipe:        recipe.Recipe,
		Flavor:        recipe.Flavor,
		HfPath:        hfModelName,
		DatasetPath:   datasetPath,
		DatasetFormat: req.TrainConfig.DatasetFormat,
		ExpName:       req.DisplayName,
		ModelSize:     recipe.Size,
		TrainConfig:   req.TrainConfig,
	})

	encodedEntrypoint := base64.StdEncoding.EncodeToString([]byte(entrypoint))

	// Step 6: Build env
	env := map[string]string{
		"PYTORCH_HIP_ALLOC_CONF": "expandable_segments:True",
		"GPUS_PER_NODE":          strconv.Itoa(req.GpuCount),
	}
	if req.NodeCount > 1 {
		env["NNODES"] = strconv.Itoa(req.NodeCount)
	}
	for k, v := range req.Env {
		env[k] = v
	}

	// Step 7: Build SFT metadata
	// Labels: only k8s-safe values (alphanumeric, '-', '_', '.'); used for list filtering.
	// Annotations: free-form values (model names with '/', usernames with spaces, etc.).
	userId := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)
	sftLabels := map[string]string{
		SftLabelWorkloadType: SftWorkloadTypeValue,
		SftLabelPeft:         req.TrainConfig.Peft,
		SftLabelBaseModelId:  req.ModelId,
		SftLabelUserId:       userId,
	}
	sftAnnotations := map[string]string{
		SftLabelModel:    hfModelName,
		SftLabelDataset:  req.DatasetId,
		SftLabelUserName: userName,
	}

	// Step 8: Build resources — PyTorchJob splits into master + workers
	nodeResource := v1.WorkloadResource{
		Replica:          1,
		CPU:              req.Cpu,
		GPU:              strconv.Itoa(req.GpuCount),
		Memory:           req.Memory,
		EphemeralStorage: req.EphemeralStorage,
	}

	var resources []v1.WorkloadResource
	var images []string
	var entryPoints []string

	if req.NodeCount > 1 {
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

	// Step 9: Create PyTorchJob workload
	workload := &v1.Workload{}
	workload.Name = generateSftWorkloadName(req.DisplayName)
	workload.Labels = map[string]string{
		v1.DisplayNameLabel: req.DisplayName,
	}
	for k, v := range sftLabels {
		workload.Labels[k] = v
	}
	workload.Annotations = map[string]string{
		v1.DescriptionAnnotation: req.Description,
	}
	for k, v := range sftAnnotations {
		workload.Annotations[k] = v
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
		"model", hfModelName,
		"dataset", req.DatasetId,
		"peft", req.TrainConfig.Peft,
	)

	return &CreateSftJobResponse{
		WorkloadId: workload.Name,
	}, nil
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
	url := model.SourceURL
	url = strings.TrimPrefix(url, "https://huggingface.co/")
	url = strings.TrimSuffix(url, "/")
	if url != "" {
		return url
	}
	return model.ModelName
}

func generateSftWorkloadName(displayName string) string {
	name := strings.ToLower(displayName)
	name = strings.ReplaceAll(name, " ", "-")
	if len(name) > 40 {
		name = name[:40]
	}
	return fmt.Sprintf("sft-%s-%d", name, time.Now().UnixMilli()%100000)
}
