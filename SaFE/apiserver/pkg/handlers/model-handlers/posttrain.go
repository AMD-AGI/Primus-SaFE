/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

type posttrainStore interface {
	UpsertPosttrainRun(ctx context.Context, run *dbclient.PosttrainRun) error
	ListPosttrainRunViews(ctx context.Context, filter *dbclient.PosttrainRunFilter) ([]*dbclient.PosttrainRunView, int, error)
	GetPosttrainRunView(ctx context.Context, runID string) (*dbclient.PosttrainRunView, error)
	SetPosttrainRunDeleted(ctx context.Context, runID string) error
}

// ListPosttrainRuns handles GET /api/v1/posttrain/runs.
func (h *Handler) ListPosttrainRuns(c *gin.Context) {
	handle(c, h.listPosttrainRuns)
}

// GetPosttrainRun handles GET /api/v1/posttrain/runs/:id.
func (h *Handler) GetPosttrainRun(c *gin.Context) {
	handle(c, h.getPosttrainRun)
}

// GetPosttrainRunMetrics handles GET /api/v1/posttrain/runs/:id/metrics.
func (h *Handler) GetPosttrainRunMetrics(c *gin.Context) {
	handle(c, h.getPosttrainRunMetrics)
}

// DeletePosttrainRun handles DELETE /api/v1/posttrain/runs/:id.
func (h *Handler) DeletePosttrainRun(c *gin.Context) {
	handle(c, h.deletePosttrainRun)
}

func (h *Handler) listPosttrainRuns(c *gin.Context) (interface{}, error) {
	var query ListPosttrainRunQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid query: %v", err))
	}
	store, err := h.getPosttrainStore()
	if err != nil {
		return nil, err
	}
	filter := &dbclient.PosttrainRunFilter{
		Workspace: query.Workspace,
		TrainType: query.TrainType,
		Strategy:  query.Strategy,
		Status:    query.Status,
		Search:    query.Search,
		UserID:    query.UserID,
		Limit:     query.Limit,
		Offset:    query.Offset,
		SortBy:    query.SortBy,
		Order:     query.Order,
	}
	runs, total, err := store.ListPosttrainRunViews(c.Request.Context(), filter)
	if err != nil {
		return nil, commonerrors.NewInternalError("failed to list posttrain runs: " + err.Error())
	}
	items := make([]PosttrainRunItem, 0, len(runs))
	for _, run := range runs {
		items = append(items, buildPosttrainRunItem(run, nil, nil))
	}
	if query.IncludeMetrics {
		h.enrichPosttrainItemsWithLoss(c.Request.Context(), items)
	}
	return &ListPosttrainRunResponse{
		Total: total,
		Items: items,
	}, nil
}

func (h *Handler) getPosttrainRun(c *gin.Context) (interface{}, error) {
	runID := c.Param("id")
	if runID == "" {
		return nil, commonerrors.NewBadRequest("run id is required")
	}
	store, err := h.getPosttrainStore()
	if err != nil {
		return nil, err
	}
	run, err := store.GetPosttrainRunView(c.Request.Context(), runID)
	if err != nil {
		return nil, err
	}
	loss, availableMetrics, _ := h.getLatestLossForRun(c.Request.Context(), run)
	resp := &PosttrainRunDetailResponse{
		PosttrainRunItem:  buildPosttrainRunItem(run, loss, availableMetrics),
		ParameterSnapshot: decodeJSONString(run.ParameterSnapshot),
		ResourceSnapshot:  decodeJSONString(run.ResourceSnapshot),
	}
	return resp, nil
}

func (h *Handler) getPosttrainRunMetrics(c *gin.Context) (interface{}, error) {
	runID := c.Param("id")
	if runID == "" {
		return nil, commonerrors.NewBadRequest("run id is required")
	}
	var query GetPosttrainMetricsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid query: %v", err))
	}
	store, err := h.getPosttrainStore()
	if err != nil {
		return nil, err
	}
	run, err := store.GetPosttrainRunView(c.Request.Context(), runID)
	if err != nil {
		return nil, err
	}
	workloadUID := nullStringValue(run.WorkloadUID)
	if workloadUID == "" {
		return nil, commonerrors.NewBadRequest("workload uid is not available yet")
	}
	availableMetrics, err := fetchLensAvailableMetrics(c.Request.Context(), workloadUID, run.Cluster)
	if err != nil {
		return nil, commonerrors.NewInternalError("failed to query Lens available metrics: " + err.Error())
	}
	points, err := fetchLensMetricData(c.Request.Context(), workloadUID, run.Cluster, query.Metrics, query.DataSource, query.Start, query.End)
	if err != nil {
		return nil, commonerrors.NewInternalError("failed to query Lens metric data: " + err.Error())
	}
	loss, _, _ := h.getLatestLossForRun(c.Request.Context(), run)
	return &PosttrainMetricsResponse{
		RunID:            run.RunID,
		WorkloadUID:      workloadUID,
		AvailableMetrics: availableMetrics,
		Data:             points,
		LatestLoss:       lossValue(loss),
		LossMetricName:   lossMetricName(loss),
		LossDataSource:   lossDataSource(loss),
	}, nil
}

func (h *Handler) deletePosttrainRun(c *gin.Context) (interface{}, error) {
	runID := c.Param("id")
	if runID == "" {
		return nil, commonerrors.NewBadRequest("run id is required")
	}
	store, err := h.getPosttrainStore()
	if err != nil {
		return nil, err
	}
	if err := store.SetPosttrainRunDeleted(c.Request.Context(), runID); err != nil {
		return nil, commonerrors.NewInternalError("failed to delete posttrain run: " + err.Error())
	}
	return gin.H{
		"message": "posttrain run deleted successfully",
		"runId":   runID,
	}, nil
}

func (h *Handler) maybeRecordSFTPosttrainRun(ctx context.Context, req *CreateSftJobRequest, workloadName, clusterID, userID, userName, baseModelName, datasetName, outputPath string) {
	if req == nil {
		return
	}
	params, resources, err := buildSFTSnapshots(*req)
	if err != nil {
		klog.Warningf("failed to build SFT posttrain snapshots for %s: %v", workloadName, err)
	}
	run := &dbclient.PosttrainRun{
		RunID:             workloadName,
		WorkloadID:        workloadName,
		DisplayName:       req.DisplayName,
		TrainType:         "sft",
		Strategy:          sftStrategy(req.TrainConfig.Peft),
		Algorithm:         sql.NullString{},
		Workspace:         req.Workspace,
		Cluster:           clusterID,
		UserID:            nullString(userID),
		UserName:          nullString(userName),
		BaseModelID:       req.ModelId,
		BaseModelName:     baseModelName,
		DatasetID:         req.DatasetId,
		DatasetName:       nullString(datasetName),
		Image:             nullString(req.Image),
		NodeCount:         nullInt32(req.NodeCount),
		GpuPerNode:        nullInt32(req.GpuCount),
		Cpu:               nullString(req.Cpu),
		Memory:            nullString(req.Memory),
		SharedMemory:      nullString(req.SharedMemory),
		EphemeralStorage:  nullString(req.EphemeralStorage),
		Priority:          nullInt32(req.Priority),
		Timeout:           nullInt32(req.Timeout),
		ExportModel:       req.ExportModel != nil && *req.ExportModel,
		OutputPath:        nullString(outputPath),
		Status:            nullString("Pending"),
		ParameterSnapshot: nullString(params),
		ResourceSnapshot:  nullString(resources),
		CreatedAt:         pq.NullTime{Time: time.Now().UTC(), Valid: true},
	}
	h.upsertPosttrainRun(ctx, run)
}

func (h *Handler) maybeRecordRLPosttrainRun(ctx context.Context, req *CreateRlJobRequest, workloadName, clusterID, userID, userName, baseModelName, datasetName, outputPath string) {
	if req == nil {
		return
	}
	params, resources, err := buildRLSnapshots(*req)
	if err != nil {
		klog.Warningf("failed to build RL posttrain snapshots for %s: %v", workloadName, err)
	}
	run := &dbclient.PosttrainRun{
		RunID:             workloadName,
		WorkloadID:        workloadName,
		DisplayName:       req.DisplayName,
		TrainType:         "rl",
		Strategy:          req.TrainConfig.Strategy,
		Algorithm:         nullString(req.TrainConfig.Algorithm),
		Workspace:         req.Workspace,
		Cluster:           clusterID,
		UserID:            nullString(userID),
		UserName:          nullString(userName),
		BaseModelID:       req.ModelId,
		BaseModelName:     baseModelName,
		DatasetID:         req.DatasetId,
		DatasetName:       nullString(datasetName),
		Image:             nullString(req.Image),
		NodeCount:         nullInt32(req.NodeCount),
		GpuPerNode:        nullInt32(req.GpuCount),
		Cpu:               nullString(req.Cpu),
		Memory:            nullString(req.Memory),
		SharedMemory:      nullString(req.SharedMemory),
		EphemeralStorage:  nullString(req.EphemeralStorage),
		Priority:          nullInt32(req.Priority),
		Timeout:           nullInt32(req.Timeout),
		ExportModel:       req.ExportModel != nil && *req.ExportModel,
		OutputPath:        nullString(outputPath),
		Status:            nullString("Pending"),
		ParameterSnapshot: nullString(params),
		ResourceSnapshot:  nullString(resources),
		CreatedAt:         pq.NullTime{Time: time.Now().UTC(), Valid: true},
	}
	h.upsertPosttrainRun(ctx, run)
}

func (h *Handler) getPosttrainStore() (posttrainStore, error) {
	store, ok := h.dbClient.(posttrainStore)
	if !ok {
		return nil, commonerrors.NewInternalError("posttrain database access is not configured")
	}
	return store, nil
}

func (h *Handler) upsertPosttrainRun(ctx context.Context, run *dbclient.PosttrainRun) {
	store, ok := h.dbClient.(posttrainStore)
	if !ok {
		klog.Warning("posttrain store is not available on current db client")
		return
	}
	if err := store.UpsertPosttrainRun(ctx, run); err != nil {
		klog.ErrorS(err, "failed to upsert posttrain run", "runId", run.RunID)
	}
}

func buildPosttrainRunItem(run *dbclient.PosttrainRunView, loss *lossSummary, availableMetrics []string) PosttrainRunItem {
	item := PosttrainRunItem{
		RunID:            run.RunID,
		WorkloadID:       run.WorkloadID,
		WorkloadUID:      nullStringValue(run.WorkloadUID),
		DisplayName:      run.DisplayName,
		TrainType:        run.TrainType,
		Strategy:         run.Strategy,
		Algorithm:        nullStringValue(run.Algorithm),
		Workspace:        run.Workspace,
		Cluster:          run.Cluster,
		UserID:           nullStringValue(run.UserID),
		UserName:         nullStringValue(run.UserName),
		BaseModelID:      run.BaseModelID,
		BaseModelName:    run.BaseModelName,
		DatasetID:        run.DatasetID,
		DatasetName:      nullStringValue(run.DatasetName),
		Image:            nullStringValue(run.Image),
		NodeCount:        nullInt32Value(run.NodeCount),
		GpuPerNode:       nullInt32Value(run.GpuPerNode),
		Cpu:              nullStringValue(run.Cpu),
		Memory:           nullStringValue(run.Memory),
		SharedMemory:     nullStringValue(run.SharedMemory),
		EphemeralStorage: nullStringValue(run.EphemeralStorage),
		Priority:         nullInt32Value(run.Priority),
		Timeout:          nullInt32Value(run.Timeout),
		ExportModel:      run.ExportModel,
		OutputPath:       nullStringValue(run.OutputPath),
		Status:           defaultStatus(run.Status),
		CreatedAt:        nullTimeString(run.CreatedAt),
		StartTime:        nullTimeString(run.StartTime),
		EndTime:          nullTimeString(run.EndTime),
		Duration:         formatDuration(run.StartTime, run.EndTime, run.DeletionTime),
		ModelID:          nullStringValue(run.ModelID),
		ModelDisplayName: nullStringValue(run.ModelDisplayName),
		ModelPhase:       nullStringValue(run.ModelPhase),
		ModelOrigin:      nullStringValue(run.ModelOrigin),
		AvailableMetrics: availableMetrics,
		ParameterSummary: summarizeParameters(run.TrainType, run.Strategy, run.Algorithm, run.ParameterSnapshot),
	}
	if loss != nil {
		value := loss.Value
		item.LatestLoss = &value
		item.LossMetricName = loss.MetricName
		item.LossDataSource = loss.DataSource
	}
	return item
}

func (h *Handler) enrichPosttrainItemsWithLoss(ctx context.Context, items []PosttrainRunItem) {
	const maxConcurrent = 4
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for i := range items {
		if items[i].WorkloadUID == "" {
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			summary, availableMetrics, err := fetchLatestLossSummary(ctx, items[idx].WorkloadUID, items[idx].Cluster)
			if err != nil {
				klog.V(4).Infof("failed to enrich latest loss for run %s: %v", items[idx].RunID, err)
				return
			}
			items[idx].AvailableMetrics = availableMetrics
			if summary != nil {
				value := summary.Value
				items[idx].LatestLoss = &value
				items[idx].LossMetricName = summary.MetricName
				items[idx].LossDataSource = summary.DataSource
			}
		}(i)
	}
	wg.Wait()
}

func (h *Handler) getLatestLossForRun(ctx context.Context, run *dbclient.PosttrainRunView) (*lossSummary, []string, error) {
	workloadUID := nullStringValue(run.WorkloadUID)
	if workloadUID == "" {
		return nil, nil, nil
	}
	return fetchLatestLossSummary(ctx, workloadUID, run.Cluster)
}

func buildSFTSnapshots(req CreateSftJobRequest) (string, string, error) {
	params, err := json.Marshal(req.TrainConfig)
	if err != nil {
		return "", "", err
	}
	resources, err := json.Marshal(map[string]interface{}{
		"nodeCount":        req.NodeCount,
		"gpuCount":         req.GpuCount,
		"cpu":              req.Cpu,
		"memory":           req.Memory,
		"sharedMemory":     req.SharedMemory,
		"ephemeralStorage": req.EphemeralStorage,
		"priority":         req.Priority,
		"timeout":          req.Timeout,
		"forceHostNetwork": req.ForceHostNetwork,
		"hostpath":         req.Hostpath,
	})
	if err != nil {
		return "", "", err
	}
	return string(params), string(resources), nil
}

func buildRLSnapshots(req CreateRlJobRequest) (string, string, error) {
	params, err := json.Marshal(req.TrainConfig)
	if err != nil {
		return "", "", err
	}
	resources, err := json.Marshal(map[string]interface{}{
		"nodeCount":        req.NodeCount,
		"gpuCount":         req.GpuCount,
		"cpu":              req.Cpu,
		"memory":           req.Memory,
		"sharedMemory":     req.SharedMemory,
		"ephemeralStorage": req.EphemeralStorage,
		"priority":         req.Priority,
		"timeout":          req.Timeout,
		"image":            req.Image,
	})
	if err != nil {
		return "", "", err
	}
	return string(params), string(resources), nil
}

func decodeJSONString(raw sql.NullString) map[string]interface{} {
	if !raw.Valid || raw.String == "" {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw.String), &out); err != nil {
		return map[string]interface{}{"raw": raw.String}
	}
	return out
}

func summarizeParameters(trainType, strategy string, algorithm sql.NullString, raw sql.NullString) string {
	snapshot := decodeJSONString(raw)
	if len(snapshot) == 0 {
		return ""
	}
	if trainType == "sft" {
		if strategy == "lora" {
			return fmt.Sprintf("lora | lr=%s | dim=%s | alpha=%s",
				formatMetricValue(snapshot["finetuneLr"]),
				formatMetricValue(snapshot["peftDim"]),
				formatMetricValue(snapshot["peftAlpha"]),
			)
		}
		return fmt.Sprintf("full | lr=%s | iters=%s | save=%s",
			formatMetricValue(snapshot["finetuneLr"]),
			formatMetricValue(snapshot["trainIters"]),
			formatMetricValue(snapshot["saveInterval"]),
		)
	}
	if strategy == "megatron" {
		return fmt.Sprintf("%s | tp=%s pp=%s cp=%s | batch=%s",
			defaultString(algorithm.String, "rl"),
			formatMetricValue(snapshot["megatronTpSize"]),
			formatMetricValue(snapshot["megatronPpSize"]),
			formatMetricValue(snapshot["megatronCpSize"]),
			formatMetricValue(snapshot["trainBatchSize"]),
		)
	}
	return fmt.Sprintf("%s | %s | batch=%s | epochs=%s",
		defaultString(algorithm.String, "rl"),
		strategy,
		formatMetricValue(snapshot["trainBatchSize"]),
		formatMetricValue(snapshot["totalEpochs"]),
	)
}

func formatMetricValue(v interface{}) string {
	switch value := v.(type) {
	case nil:
		return "-"
	case string:
		if value == "" {
			return "-"
		}
		return value
	case float64:
		if value == float64(int64(value)) {
			return fmt.Sprintf("%d", int64(value))
		}
		return fmt.Sprintf("%.4g", value)
	case bool:
		if value {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", value)
	}
}

func formatDuration(start, end, deleted pq.NullTime) string {
	if !start.Valid {
		return ""
	}
	finish := time.Now().UTC()
	if end.Valid {
		finish = end.Time
	} else if deleted.Valid {
		finish = deleted.Time
	}
	seconds := int64(finish.Sub(start.Time).Seconds())
	if seconds < 0 {
		return ""
	}
	return timeutil.FormatDuration(seconds)
}

func defaultStatus(status sql.NullString) string {
	if !status.Valid || status.String == "" {
		return "Pending"
	}
	return status.String
}

func nullStringValue(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func nullInt32Value(v sql.NullInt32) int {
	if !v.Valid {
		return 0
	}
	return int(v.Int32)
}

func nullTimeString(v pq.NullTime) string {
	if !v.Valid {
		return ""
	}
	return timeutil.FormatRFC3339(v.Time)
}

func nullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

func nullInt32(v int) sql.NullInt32 {
	if v == 0 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(v), Valid: true}
}

func sftStrategy(peft string) string {
	if peft == "lora" {
		return "lora"
	}
	return "full"
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func lossValue(loss *lossSummary) *float64 {
	if loss == nil {
		return nil
	}
	value := loss.Value
	return &value
}

func lossMetricName(loss *lossSummary) string {
	if loss == nil {
		return ""
	}
	return loss.MetricName
}

func lossDataSource(loss *lossSummary) string {
	if loss == nil {
		return ""
	}
	return loss.DataSource
}

func (h *Handler) resolvePosttrainDatasetName(ctx context.Context, datasetID string) string {
	if datasetID == "" {
		return ""
	}
	dataset, err := h.dbClient.GetDataset(ctx, datasetID)
	if err != nil || dataset == nil {
		return datasetID
	}
	if dataset.DisplayName != "" {
		return dataset.DisplayName
	}
	return datasetID
}

func (h *Handler) resolvePosttrainClusterID(ctx context.Context, workspaceID string) string {
	if workspaceID == "" {
		return ""
	}
	workspace := &v1.Workspace{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: workspaceID}, workspace); err != nil {
		return ""
	}
	return workspace.Spec.Cluster
}
