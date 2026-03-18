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

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// ==================== SFT Job Handlers ====================

// CreateSftJob handles POST /api/v1/sft/jobs
func (h *Handler) CreateSftJob(c *gin.Context) {
	handle(c, h.createSftJob)
}

// ListSftJobs handles GET /api/v1/sft/jobs
func (h *Handler) ListSftJobs(c *gin.Context) {
	handle(c, h.listSftJobs)
}

// GetSftJob handles GET /api/v1/sft/jobs/:id
func (h *Handler) GetSftJob(c *gin.Context) {
	handle(c, h.getSftJob)
}

// DeleteSftJob handles DELETE /api/v1/sft/jobs/:id
func (h *Handler) DeleteSftJob(c *gin.Context) {
	handle(c, h.deleteSftJob)
}

// StopSftJob handles POST /api/v1/sft/jobs/:id/stop
func (h *Handler) StopSftJob(c *gin.Context) {
	handle(c, h.stopSftJob)
}

// ==================== Create ====================

func (h *Handler) createSftJob(c *gin.Context) (interface{}, error) {
	var req CreateSftJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request body: %v", err))
	}

	ctx := c.Request.Context()

	if req.ModelSource != "model_square" && req.ModelSource != "huggingface" {
		return nil, commonerrors.NewBadRequest("modelSource must be 'model_square' or 'huggingface'")
	}

	var hfModelName string
	var baseModelId string

	switch req.ModelSource {
	case "model_square":
		if req.ModelId == "" {
			return nil, commonerrors.NewBadRequest("modelId is required when modelSource is 'model_square'")
		}
		model, err := h.dbClient.GetModelByID(ctx, req.ModelId)
		if err != nil {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("model not found: %s", req.ModelId))
		}
		hfModelName = extractHfModelName(model)
		baseModelId = req.ModelId
	case "huggingface":
		if req.HfModelName == "" {
			return nil, commonerrors.NewBadRequest("hfModelName is required when modelSource is 'huggingface'")
		}
		hfModelName = req.HfModelName
	}

	recipe, err := InferModelRecipe(hfModelName)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	FillSftDefaults(&req, recipe.Size)

	datasetPath, err := h.resolveDatasetPath(ctx, req.DatasetId, req.Workspace)
	if err != nil {
		return nil, err
	}

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

	env := map[string]string{
		"PYTORCH_HIP_ALLOC_CONF": "expandable_segments:True",
	}
	for k, v := range req.Env {
		env[k] = v
	}

	customerLabels := map[string]string{
		SftLabelWorkloadType: SftWorkloadTypeValue,
		SftLabelModel:        hfModelName,
		SftLabelDataset:      req.DatasetId,
		SftLabelPeft:         req.TrainConfig.Peft,
	}
	if baseModelId != "" {
		customerLabels[SftLabelBaseModelId] = baseModelId
	}

	workload := &v1.Workload{}
	workload.Name = generateSftWorkloadName(req.DisplayName)
	workload.Labels = map[string]string{
		v1.DisplayNameLabel: req.DisplayName,
	}
	workload.Annotations = map[string]string{
		v1.DescriptionAnnotation: req.Description,
	}

	timeout := 0
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	workload.Spec = v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind, Version: common.DefaultVersion},
		Images:           []string{req.Image},
		EntryPoints:      []string{encodedEntrypoint},
		Resources: []v1.WorkloadResource{{
			Replica:          1,
			CPU:              req.Cpu,
			GPU:              strconv.Itoa(req.GpuCount),
			Memory:           req.Memory,
			EphemeralStorage: req.EphemeralStorage,
		}},
		Env:            env,
		CustomerLabels: customerLabels,
		Hostpath:       req.Hostpath,
		Priority:       req.Priority,
		Workspace:      req.Workspace,
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

	return &SftJobResponse{
		WorkloadId:  workload.Name,
		DisplayName: req.DisplayName,
		Phase:       string(v1.WorkloadPending),
		ModelSource: req.ModelSource,
		ModelName:   hfModelName,
		BaseModelId: baseModelId,
		DatasetId:   req.DatasetId,
		Peft:        req.TrainConfig.Peft,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// ==================== List ====================

func (h *Handler) listSftJobs(c *gin.Context) (interface{}, error) {
	var req ListSftJobsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid query: %v", err))
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 200 {
		req.Limit = 200
	}

	ctx := c.Request.Context()
	dbTags := dbclient.GetWorkloadFieldTags()

	gvkStr := fmt.Sprintf(`{"version":"%s","kind":"%s"}`, common.DefaultVersion, common.PytorchJobKind)
	query := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "IsDeleted"): false},
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "GVK"): gvkStr},
		sqrl.Like{dbclient.GetFieldTag(dbTags, "CustomerLabels"): fmt.Sprintf("%%%s%%", SftWorkloadTypeValue)},
	}
	if req.Workspace != "" {
		query = append(query, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Workspace"): req.Workspace})
	}
	if req.Phase != "" {
		query = append(query, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Phase"): req.Phase})
	}

	orderBy := []string{"creation_time DESC NULLS LAST", "id DESC"}

	totalCount, err := h.dbClient.CountWorkloads(ctx, query)
	if err != nil {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to count SFT jobs: %v", err))
	}

	workloads, err := h.dbClient.SelectWorkloads(ctx, query, orderBy, req.Limit, req.Offset)
	if err != nil {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to list SFT jobs: %v", err))
	}

	items := make([]SftJobResponse, 0, len(workloads))
	for _, w := range workloads {
		items = append(items, convertWorkloadToSftResponse(w))
	}

	return &ListSftJobsResponse{
		Items:      items,
		TotalCount: totalCount,
	}, nil
}

// ==================== Get Detail ====================

func (h *Handler) getSftJob(c *gin.Context) (interface{}, error) {
	workloadId := c.Param("id")
	if workloadId == "" {
		return nil, commonerrors.NewBadRequest("workload id is required")
	}

	ctx := c.Request.Context()
	workload, err := h.dbClient.GetWorkload(ctx, workloadId)
	if err != nil {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to get workload: %v", err))
	}

	labels := parseSftLabelsFromDB(workload)
	if labels[SftLabelWorkloadType] != SftWorkloadTypeValue {
		return nil, commonerrors.NewBadRequest("workload is not an SFT job")
	}

	detail := &SftJobDetailResponse{
		SftJobResponse: convertWorkloadToSftResponse(workload),
		Image:          workload.Image,
		OutputPath:     "nemo_experiments/default/checkpoints/",
	}

	if str := dbutils.ParseNullString(workload.EntryPoints); str != "" {
		var eps []string
		if json.Unmarshal([]byte(str), &eps) == nil && len(eps) > 0 {
			if decoded, decErr := base64.StdEncoding.DecodeString(eps[0]); decErr == nil {
				detail.EntryPoint = string(decoded)
			}
		}
	}

	if str := dbutils.ParseNullString(workload.Env); str != "" {
		var envMap map[string]string
		if json.Unmarshal([]byte(str), &envMap) == nil {
			detail.Env = envMap
		}
	}

	if str := dbutils.ParseNullString(workload.Resources); str != "" {
		var resources interface{}
		if json.Unmarshal([]byte(str), &resources) == nil {
			detail.Resource = resources
		}
	}

	if str := dbutils.ParseNullString(workload.Pods); str != "" {
		var pods interface{}
		if json.Unmarshal([]byte(str), &pods) == nil {
			detail.Pods = pods
		}
	}

	if str := dbutils.ParseNullString(workload.Conditions); str != "" {
		var conditions interface{}
		if json.Unmarshal([]byte(str), &conditions) == nil {
			detail.Conditions = conditions
		}
	}

	detail.TrainConfig = SftTrainConfig{
		Peft: labels[SftLabelPeft],
	}

	// TODO: query Lens API for real-time training metrics when phase is Running

	return detail, nil
}

// ==================== Stop / Delete ====================

func (h *Handler) stopSftJob(c *gin.Context) (interface{}, error) {
	workloadId := c.Param("id")
	if workloadId == "" {
		return nil, commonerrors.NewBadRequest("workload id is required")
	}

	ctx := c.Request.Context()
	workload := &v1.Workload{}
	key := ctrlclient.ObjectKey{Name: workloadId}
	if err := h.k8sClient.Get(ctx, key, workload); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("workload not found: %s", workloadId))
		}
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to get workload: %v", err))
	}

	if workload.Annotations == nil {
		workload.Annotations = make(map[string]string)
	}
	workload.Annotations["primus-safe/stop"] = "true"
	if err := h.k8sClient.Update(ctx, workload); err != nil {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to stop workload: %v", err))
	}

	return gin.H{"message": "SFT job stop requested", "workloadId": workloadId}, nil
}

func (h *Handler) deleteSftJob(c *gin.Context) (interface{}, error) {
	workloadId := c.Param("id")
	if workloadId == "" {
		return nil, commonerrors.NewBadRequest("workload id is required")
	}

	ctx := c.Request.Context()
	workload := &v1.Workload{}
	workload.Name = workloadId
	if err := h.k8sClient.Delete(ctx, workload); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("workload not found: %s", workloadId))
		}
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to delete workload: %v", err))
	}

	return gin.H{"message": "SFT job deleted", "workloadId": workloadId}, nil
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

func convertWorkloadToSftResponse(w *dbclient.Workload) SftJobResponse {
	labels := parseSftLabelsFromDB(w)

	resp := SftJobResponse{
		WorkloadId:  w.WorkloadId,
		DisplayName: w.DisplayName,
		Phase:       dbutils.ParseNullString(w.Phase),
		ModelSource: "huggingface",
		ModelName:   labels[SftLabelModel],
		BaseModelId: labels[SftLabelBaseModelId],
		DatasetId:   labels[SftLabelDataset],
		Peft:        labels[SftLabelPeft],
	}

	if resp.BaseModelId != "" {
		resp.ModelSource = "model_square"
	}

	if w.CreationTime.Valid {
		resp.CreatedAt = w.CreationTime.Time.Format(time.RFC3339)
	}
	if w.StartTime.Valid && w.EndTime.Valid {
		resp.Duration = w.EndTime.Time.Sub(w.StartTime.Time).String()
	} else if w.StartTime.Valid {
		resp.Duration = time.Since(w.StartTime.Time).Truncate(time.Second).String()
	}

	return resp
}

func parseSftLabelsFromDB(w *dbclient.Workload) map[string]string {
	labels := make(map[string]string)
	str := dbutils.ParseNullString(w.CustomerLabels)
	if str != "" {
		json.Unmarshal([]byte(str), &labels)
	}
	return labels
}
