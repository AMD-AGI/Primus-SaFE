// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package logs

import (
	"context"
	"encoding/json"
	"net/http"

	advisorClient "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/client"
	advisorCommon "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/pods"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// WandBHandler WandB data reporting API handler
type WandBHandler struct {
	aiAdvisorClient *advisorClient.Client
	logProcessor    *WandBLogProcessor
}

// wandbHandler global handler instance
var wandbHandler *WandBHandler

// InitWandBHandlerWithClient initializes WandB Handler (using AI Advisor client)
func InitWandBHandlerWithClient(
	aiAdvisor *advisorClient.Client,
	logProcessor *WandBLogProcessor,
) {
	wandbHandler = &WandBHandler{
		aiAdvisorClient: aiAdvisor,
		logProcessor:    logProcessor,
	}
}

// WandBBatchRequest batch reporting request
type WandBBatchRequest struct {
	Detection *WandBDetectionRequest `json:"detection,omitempty"`
	Metrics   *WandBMetricsRequest   `json:"metrics,omitempty"`
	Logs      *WandBLogsRequest      `json:"logs,omitempty"`
}

// ReceiveWandBDetection handles framework detection reporting
// POST /wandb/detection
func ReceiveWandBDetection(ctx *gin.Context) {
	logrus.Info("====== [WandB Detection API] Received request ======")

	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBDetectionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logrus.Errorf("[WandB Detection API] Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	// Print request body details
	reqJSON, _ := json.MarshalIndent(req, "", "  ")
	logrus.Debugf("[WandB Detection API] Request body:\n%s", string(reqJSON))

	// Log output supporting dual-layer frameworks
	if len(req.Hints.WrapperFrameworks) > 0 || len(req.Hints.BaseFrameworks) > 0 {
		logrus.Infof("[WandB Detection API] Detection request (dual-layer frameworks) - WorkloadUID: %s, PodName: %s, RunID: %s, Wrapper: %v, Base: %v",
			req.WorkloadUID, req.PodName, req.Evidence.WandB.ID, req.Hints.WrapperFrameworks, req.Hints.BaseFrameworks)
	} else {
		// Backward compatibility: old format
		logrus.Infof("[WandB Detection API] Detection request - WorkloadUID: %s, PodName: %s, RunID: %s, PossibleFrameworks: %v",
			req.WorkloadUID, req.PodName, req.Evidence.WandB.ID, req.Hints.PossibleFrameworks)
	}

	// Validation: must provide workload_uid or pod_name
	if req.WorkloadUID == "" && req.PodName == "" {
		logrus.Warnf("[WandB Detection API] Validation failed: neither workload_uid nor pod_name provided")
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "either workload_uid or pod_name is required", nil))
		return
	}

	// Resolve workload UIDs using multi-level fallback (cache → DB)
	workloadUIDs := resolveWorkloadUIDs(req.WorkloadUID, req.PodName, req.PodUID, "WandB Detection API")
	if len(workloadUIDs) == 0 {
		logrus.Errorf("[WandB Detection API] No valid workload UIDs found")
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "no valid workload found", nil))
		return
	}

	// Process each workload
	logrus.Infof("[WandB Detection API] Processing detection for %d workload(s)...", len(workloadUIDs))
	successCount := 0
	var lastErr error

	for _, workloadUID := range workloadUIDs {
		// Create a copy of the request for each workload
		reqCopy := req
		reqCopy.WorkloadUID = workloadUID

		// Forward detection request to AI Advisor
		logrus.Infof("[WandB Detection API] Forwarding detection request to AI Advisor for WorkloadUID: %s...", workloadUID)

		// Convert request to AI Advisor format
		advisorReq := convertToAdvisorWandBRequest(&reqCopy)

		if err := wandbHandler.aiAdvisorClient.ReportWandBDetection(advisorReq); err != nil {
			logrus.Errorf("[WandB Detection API] Failed to forward detection to AI Advisor for WorkloadUID %s: %v", workloadUID, err)
			lastErr = err
		} else {
			successCount++
			// Success log supporting dual-layer frameworks
			if len(req.Hints.WrapperFrameworks) > 0 || len(req.Hints.BaseFrameworks) > 0 {
				logrus.Infof("[WandB Detection API] ✓ Detection processed successfully (dual-layer frameworks) - Wrapper: %v, Base: %v, WorkloadUID: %s",
					req.Hints.WrapperFrameworks, req.Hints.BaseFrameworks, workloadUID)
			} else {
				logrus.Infof("[WandB Detection API] ✓ Detection processed successfully - PossibleFrameworks: %v, WorkloadUID: %s",
					req.Hints.PossibleFrameworks, workloadUID)
			}
		}
	}

	if successCount == 0 {
		logrus.Errorf("[WandB Detection API] All workloads failed to process detection")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "failed to process detection", lastErr))
		return
	}

	logrus.Infof("[WandB Detection API] ✓ Detection processing completed - Success: %d/%d workloads", successCount, len(workloadUIDs))
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message":         "detection reported successfully",
		"workloads_count": len(workloadUIDs),
		"success_count":   successCount,
	}))
}

// ReceiveWandBMetrics handles metrics reporting
// POST /wandb/metrics
func ReceiveWandBMetrics(ctx *gin.Context) {
	logrus.Info("====== [WandB Metrics API] Received request ======")

	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBMetricsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logrus.Errorf("[WandB Metrics API] Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	// Print request body summary (avoid excessive logging)
	logrus.Infof("[WandB Metrics API] Metrics request - WorkloadUID: %s, PodName: %s, RunID: %s, MetricsCount: %d",
		req.WorkloadUID, req.PodName, req.RunID, len(req.Metrics))

	// Detailed logs (only at Debug level)
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		reqJSON, _ := json.MarshalIndent(req, "", "  ")
		logrus.Debugf("[WandB Metrics API] Request body:\n%s", string(reqJSON))

		// Print a few metric samples
		sampleSize := 3
		if len(req.Metrics) < sampleSize {
			sampleSize = len(req.Metrics)
		}
		for i := 0; i < sampleSize; i++ {
			m := req.Metrics[i]
			logrus.Debugf("[WandB Metrics API] Metric[%d]: name=%s, value=%v, step=%d",
				i, m.Name, m.Value, m.Step)
		}
	}

	// Validation: must provide workload_uid or pod_name
	if req.WorkloadUID == "" && req.PodName == "" {
		logrus.Warnf("[WandB Metrics API] Validation failed: neither workload_uid nor pod_name provided")
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "either workload_uid or pod_name is required", nil))
		return
	}

	// Resolve workload UIDs using multi-level fallback (cache → DB)
	workloadUIDs := resolveWorkloadUIDs(req.WorkloadUID, req.PodName, req.PodUID, "WandB Metrics API")
	if len(workloadUIDs) == 0 {
		logrus.Errorf("[WandB Metrics API] No valid workload UIDs found")
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "no valid workload found", nil))
		return
	}

	// Process each workload
	logrus.Infof("[WandB Metrics API] Processing metrics for %d workload(s), %d metrics per workload...",
		len(workloadUIDs), len(req.Metrics))
	successCount := 0
	var lastErr error

	for _, workloadUID := range workloadUIDs {
		// Create a copy of the request for each workload
		reqCopy := req
		reqCopy.WorkloadUID = workloadUID

		logrus.Infof("[WandB Metrics API] Starting metrics processing for WorkloadUID: %s (%d metrics)...",
			workloadUID, len(req.Metrics))
		if err := wandbHandler.logProcessor.ProcessMetrics(ctx.Request.Context(), &reqCopy); err != nil {
			logrus.Errorf("[WandB Metrics API] Failed to process metrics for WorkloadUID %s: %v", workloadUID, err)
			lastErr = err
		} else {
			successCount++
			logrus.Infof("[WandB Metrics API] ✓ Metrics processed successfully - Count: %d, WorkloadUID: %s",
				len(req.Metrics), workloadUID)
		}
	}

	if successCount == 0 {
		logrus.Errorf("[WandB Metrics API] All workloads failed to process metrics")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "failed to process metrics", lastErr))
		return
	}

	logrus.Infof("[WandB Metrics API] ✓ Metrics processing completed - Success: %d/%d workloads", successCount, len(workloadUIDs))
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message":         "metrics reported successfully",
		"count":           len(req.Metrics),
		"workloads_count": len(workloadUIDs),
		"success_count":   successCount,
	}))
}

// ReceiveWandBLogs handles training data reporting from wandb.log()
// POST /wandb/logs
// Note: Despite the name, this endpoint receives structured training metrics
// from wandb.log(), not text logs. Data is stored in training_performance table.
func ReceiveWandBLogs(ctx *gin.Context) {
	logrus.Info("====== [WandB Logs/Training API] Received request ======")

	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBLogsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logrus.Errorf("[WandB Logs/Training API] Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	// Print request body summary
	logrus.Infof("[WandB Logs/Training API] Training data request - WorkloadUID: %s, PodName: %s, RunID: %s, LogsCount: %d",
		req.WorkloadUID, req.PodName, req.RunID, len(req.Logs))

	// Detailed logs (only at Debug level)
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		reqJSON, _ := json.MarshalIndent(req, "", "  ")
		logrus.Debugf("[WandB Logs/Training API] Request body:\n%s", string(reqJSON))

		// Print a few training data entry samples
		sampleSize := 3
		if len(req.Logs) < sampleSize {
			sampleSize = len(req.Logs)
		}
		for i := 0; i < sampleSize; i++ {
			l := req.Logs[i]
			logrus.Debugf("[WandB Logs/Training API] Log[%d]: step=%d, dataKeys=%v",
				i, l.Step, getMapKeys(l.Data))
		}
	}

	// Validation: must provide workload_uid or pod_name
	if req.WorkloadUID == "" && req.PodName == "" {
		logrus.Warnf("[WandB Logs/Training API] Validation failed: neither workload_uid nor pod_name provided")
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "either workload_uid or pod_name is required", nil))
		return
	}

	// Resolve workload UIDs using multi-level fallback (cache → DB)
	workloadUIDs := resolveWorkloadUIDs(req.WorkloadUID, req.PodName, req.PodUID, "WandB Logs/Training API")
	if len(workloadUIDs) == 0 {
		logrus.Errorf("[WandB Logs/Training API] No valid workload UIDs found")
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "no valid workload found", nil))
		return
	}

	// Process each workload
	logrus.Infof("[WandB Logs/Training API] Processing training data for %d workload(s), %d entries per workload...",
		len(workloadUIDs), len(req.Logs))
	successCount := 0
	var lastErr error

	for _, workloadUID := range workloadUIDs {
		// Create a copy of the request for each workload
		reqCopy := req
		reqCopy.WorkloadUID = workloadUID

		logrus.Infof("[WandB Logs/Training API] Starting training data processing for WorkloadUID: %s (%d entries)...",
			workloadUID, len(req.Logs))
		if err := wandbHandler.logProcessor.ProcessLogs(ctx.Request.Context(), &reqCopy); err != nil {
			logrus.Errorf("[WandB Logs/Training API] Failed to process training data for WorkloadUID %s: %v", workloadUID, err)
			lastErr = err
		} else {
			successCount++
			logrus.Infof("[WandB Logs/Training API] ✓ Training data processed successfully - Count: %d, WorkloadUID: %s",
				len(req.Logs), workloadUID)
		}
	}

	if successCount == 0 {
		logrus.Errorf("[WandB Logs/Training API] All workloads failed to process training data")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "failed to process logs", lastErr))
		return
	}

	logrus.Infof("[WandB Logs/Training API] ✓ Training data processing completed - Success: %d/%d workloads", successCount, len(workloadUIDs))
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message":         "training data reported successfully",
		"count":           len(req.Logs),
		"workloads_count": len(workloadUIDs),
		"success_count":   successCount,
	}))
}

// ReceiveWandBBatch batch reporting
// POST /wandb/batch
func ReceiveWandBBatch(ctx *gin.Context) {
	logrus.Info("====== [WandB Batch API] Received request ======")

	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBBatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logrus.Errorf("[WandB Batch API] Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	// Print batch request summary
	detectionCount := 0
	if req.Detection != nil {
		detectionCount = 1
	}
	metricsCount := 0
	if req.Metrics != nil {
		metricsCount = len(req.Metrics.Metrics)
	}
	logsCount := 0
	if req.Logs != nil {
		logsCount = len(req.Logs.Logs)
	}

	logrus.Infof("[WandB Batch API] Batch request summary - Detection: %d, Metrics: %d, Logs: %d",
		detectionCount, metricsCount, logsCount)

	// Detailed logs (only at Debug level)
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		reqJSON, _ := json.MarshalIndent(req, "", "  ")
		logrus.Debugf("[WandB Batch API] Request body:\n%s", string(reqJSON))
	}

	result := gin.H{
		"success": true,
		"results": gin.H{},
	}

	// Process framework detection
	if req.Detection != nil {
		// Resolve workload UIDs using multi-level fallback (cache → DB)
		workloadUIDs := resolveWorkloadUIDs(req.Detection.WorkloadUID, req.Detection.PodName, req.Detection.PodUID, "WandB Batch API - Detection")

		if len(workloadUIDs) == 0 {
			logrus.Errorf("[WandB Batch API] No valid workload UIDs found for detection")
			result["results"].(gin.H)["detection"] = gin.H{
				"success": false,
				"error":   "no valid workload found",
			}
		} else {
			// Process each workload
			logrus.Infof("[WandB Batch API] Processing detection for %d workload(s)...", len(workloadUIDs))
			detectionSuccessCount := 0
			var detectionLastErr error

			for _, workloadUID := range workloadUIDs {
				// Create a copy of the request for each workload
				detectionReq := *req.Detection
				detectionReq.WorkloadUID = workloadUID

				// Log output supporting dual-layer frameworks
				if len(detectionReq.Hints.WrapperFrameworks) > 0 || len(detectionReq.Hints.BaseFrameworks) > 0 {
					logrus.Infof("[WandB Batch API] Processing detection (dual-layer frameworks) - Wrapper: %v, Base: %v, WorkloadUID: %s",
						detectionReq.Hints.WrapperFrameworks, detectionReq.Hints.BaseFrameworks, workloadUID)
				} else {
					logrus.Infof("[WandB Batch API] Processing detection - PossibleFrameworks: %v, WorkloadUID: %s",
						detectionReq.Hints.PossibleFrameworks, workloadUID)
				}

				// Forward to AI Advisor
				advisorReq := convertToAdvisorWandBRequest(&detectionReq)
				if err := wandbHandler.aiAdvisorClient.ReportWandBDetection(advisorReq); err != nil {
					logrus.Errorf("[WandB Batch API] Failed to forward detection to AI Advisor for WorkloadUID %s: %v", workloadUID, err)
					detectionLastErr = err
				} else {
					detectionSuccessCount++
					logrus.Infof("[WandB Batch API] ✓ Detection forwarded to AI Advisor successfully for WorkloadUID: %s", workloadUID)
				}
			}

			if detectionSuccessCount == 0 {
				result["results"].(gin.H)["detection"] = gin.H{
					"success": false,
					"error":   detectionLastErr.Error(),
				}
			} else {
				logrus.Infof("[WandB Batch API] ✓ Detection processing completed - Success: %d/%d workloads", detectionSuccessCount, len(workloadUIDs))
				result["results"].(gin.H)["detection"] = gin.H{
					"success":         true,
					"workloads_count": len(workloadUIDs),
					"success_count":   detectionSuccessCount,
				}
			}
		}
	}

	// Process metrics
	if req.Metrics != nil {
		// Resolve workload UIDs using multi-level fallback (cache → DB)
		workloadUIDs := resolveWorkloadUIDs(req.Metrics.WorkloadUID, req.Metrics.PodName, req.Metrics.PodUID, "WandB Batch API - Metrics")

		if len(workloadUIDs) == 0 {
			logrus.Errorf("[WandB Batch API] No valid workload UIDs found for metrics")
			result["results"].(gin.H)["metrics"] = gin.H{
				"success": false,
				"error":   "no valid workload found",
			}
		} else {
			// Process each workload
			logrus.Infof("[WandB Batch API] Processing metrics for %d workload(s), %d metrics per workload...",
				len(workloadUIDs), len(req.Metrics.Metrics))
			metricsSuccessCount := 0
			var metricsLastErr error

			for _, workloadUID := range workloadUIDs {
				// Create a copy of the request for each workload
				metricsReq := *req.Metrics
				metricsReq.WorkloadUID = workloadUID

				logrus.Infof("[WandB Batch API] Processing metrics - Count: %d, WorkloadUID: %s",
					len(metricsReq.Metrics), workloadUID)
				if err := wandbHandler.logProcessor.ProcessMetrics(ctx.Request.Context(), &metricsReq); err != nil {
					logrus.Errorf("[WandB Batch API] Failed to process metrics for WorkloadUID %s: %v", workloadUID, err)
					metricsLastErr = err
				} else {
					metricsSuccessCount++
					logrus.Infof("[WandB Batch API] ✓ Metrics processed successfully - Count: %d, WorkloadUID: %s",
						len(metricsReq.Metrics), workloadUID)
				}
			}

			if metricsSuccessCount == 0 {
				result["results"].(gin.H)["metrics"] = gin.H{
					"success": false,
					"error":   metricsLastErr.Error(),
				}
			} else {
				logrus.Infof("[WandB Batch API] ✓ Metrics processing completed - Success: %d/%d workloads", metricsSuccessCount, len(workloadUIDs))
				result["results"].(gin.H)["metrics"] = gin.H{
					"success":         true,
					"count":           len(req.Metrics.Metrics),
					"workloads_count": len(workloadUIDs),
					"success_count":   metricsSuccessCount,
				}
			}
		}
	}

	// Process logs
	if req.Logs != nil {
		// Resolve workload UIDs using multi-level fallback (cache → DB)
		workloadUIDs := resolveWorkloadUIDs(req.Logs.WorkloadUID, req.Logs.PodName, req.Logs.PodUID, "WandB Batch API - Logs")

		if len(workloadUIDs) == 0 {
			logrus.Errorf("[WandB Batch API] No valid workload UIDs found for logs")
			result["results"].(gin.H)["logs"] = gin.H{
				"success": false,
				"error":   "no valid workload found",
			}
		} else {
			// Process each workload
			logrus.Infof("[WandB Batch API] Processing training data for %d workload(s), %d entries per workload...",
				len(workloadUIDs), len(req.Logs.Logs))
			logsSuccessCount := 0
			var logsLastErr error

			for _, workloadUID := range workloadUIDs {
				// Create a copy of the request for each workload
				logsReq := *req.Logs
				logsReq.WorkloadUID = workloadUID

				logrus.Infof("[WandB Batch API] Processing training data - Count: %d, WorkloadUID: %s",
					len(logsReq.Logs), workloadUID)
				if err := wandbHandler.logProcessor.ProcessLogs(ctx.Request.Context(), &logsReq); err != nil {
					logrus.Errorf("[WandB Batch API] Failed to process training data for WorkloadUID %s: %v", workloadUID, err)
					logsLastErr = err
				} else {
					logsSuccessCount++
					logrus.Infof("[WandB Batch API] ✓ Training data processed successfully - Count: %d, WorkloadUID: %s",
						len(logsReq.Logs), workloadUID)
				}
			}

			if logsSuccessCount == 0 {
				result["results"].(gin.H)["logs"] = gin.H{
					"success": false,
					"error":   logsLastErr.Error(),
				}
			} else {
				logrus.Infof("[WandB Batch API] ✓ Training data processing completed - Success: %d/%d workloads", logsSuccessCount, len(workloadUIDs))
				result["results"].(gin.H)["logs"] = gin.H{
					"success":         true,
					"count":           len(req.Logs.Logs),
					"workloads_count": len(workloadUIDs),
					"success_count":   logsSuccessCount,
				}
			}
		}
	}

	logrus.Infof("[WandB Batch API] ✓ Batch request completed - Detection: %v, Metrics: %v, Logs: %v",
		req.Detection != nil, req.Metrics != nil, req.Logs != nil)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), result))
}

// getMapKeys gets all keys from a map (for log output)
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// resolveWorkloadUIDs resolves WorkloadUIDs using a multi-level fallback strategy:
//  1. Use workload_uid directly if provided
//  2. Look up by pod_name in cache
//  3. Look up by pod_uid in cache
//  4. Query database by pod_uid as final fallback (handles cache-miss on newly started pods)
func resolveWorkloadUIDs(workloadUID string, podName string, podUID string, apiName string) []string {
	if workloadUID != "" {
		logrus.Infof("[%s] WorkloadUID provided: %s", apiName, workloadUID)
		return []string{workloadUID}
	}

	// Try pod_name cache lookup
	if podName != "" {
		workloads := pods.GetWorkloadsByPodName(podName)
		if len(workloads) > 0 {
			result := extractWorkloadUIDs(workloads, apiName, "podName cache", podName)
			return result
		}
		logrus.Debugf("[%s] PodName %s not in cache, trying next strategy", apiName, podName)
	}

	// Try pod_uid cache lookup
	if podUID != "" {
		workloads := pods.GetWorkloadsByPodUid(podUID)
		if len(workloads) > 0 {
			result := extractWorkloadUIDs(workloads, apiName, "podUID cache", podUID)
			return result
		}
		logrus.Debugf("[%s] PodUID %s not in cache, falling back to DB query", apiName, podUID)
	}

	// DB fallback: query workload_pod_reference by pod_uid
	if podUID != "" {
		clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
		facade := database.GetFacadeForCluster(clusterName).GetWorkload()
		uids, err := facade.ListWorkloadUidsByPodUids(context.Background(), []string{podUID})
		if err != nil {
			logrus.Warnf("[%s] DB fallback query failed for PodUID %s: %v", apiName, podUID, err)
		} else if len(uids) > 0 {
			logrus.Infof("[%s] ✓ DB fallback found %d workload(s) for PodUID: %s", apiName, len(uids), podUID)
			for _, uid := range uids {
				logrus.Infof("[%s]   WorkloadUID: %s", apiName, uid)
			}
			return uids
		}
	}

	identifier := podName
	if identifier == "" {
		identifier = podUID
	}
	logrus.Warnf("[%s] No workload found for pod (name=%s, uid=%s)", apiName, podName, podUID)
	return []string{}
}

func extractWorkloadUIDs(workloads [][]string, apiName string, source string, key string) []string {
	result := make([]string, 0, len(workloads))
	for _, w := range workloads {
		result = append(result, w[1])
		logrus.Infof("[%s] ✓ Found workload via %s - WorkloadName: %s, WorkloadUID: %s, Key: %s",
			apiName, source, w[0], w[1], key)
	}
	return result
}

// getWorkloadUIDsFromPodName is kept for backward compatibility, delegates to resolveWorkloadUIDs.
func getWorkloadUIDsFromPodName(workloadUID string, podName string, apiName string) []string {
	return resolveWorkloadUIDs(workloadUID, podName, "", apiName)
}

// convertToAdvisorWandBRequest converts telemetry-processor WandBDetectionRequest
// to ai-advisor WandBDetectionRequest format
// Note: Since structures are slightly different, we use JSON marshaling for safe conversion
func convertToAdvisorWandBRequest(req *WandBDetectionRequest) *advisorCommon.WandBDetectionRequest {
	if req == nil {
		return nil
	}

	// Build evidence with all fields
	evidence := advisorCommon.WandBEvidence{
		WandB: advisorCommon.WandBInfo{
			ID:      req.Evidence.WandB.ID,
			Name:    req.Evidence.WandB.Name,
			Project: req.Evidence.WandB.Project,
			Config:  req.Evidence.WandB.Config,
		},
		Environment:       req.Evidence.Environment,
		System:            req.Evidence.System,
		WrapperFrameworks: req.Evidence.WrapperFrameworks,
		BaseFrameworks:    req.Evidence.BaseFrameworks,
	}

	// PyTorch info
	if req.Evidence.PyTorch != nil {
		evidence.PyTorch = &advisorCommon.PyTorchInfo{
			Available:       req.Evidence.PyTorch.Available,
			Version:         req.Evidence.PyTorch.Version,
			CudaAvailable:   req.Evidence.PyTorch.CudaAvailable,
			DetectedModules: req.Evidence.PyTorch.DetectedModules,
		}
	}

	// Hardware info
	if req.Evidence.Hardware != nil {
		evidence.Hardware = &advisorCommon.HardwareInfo{
			GPUArch:     req.Evidence.Hardware.GPUArch,
			GPUCount:    req.Evidence.Hardware.GPUCount,
			GPUMemoryGB: req.Evidence.Hardware.GPUMemoryGB,
			GPUName:     req.Evidence.Hardware.GPUName,
			ROCmVersion: req.Evidence.Hardware.ROCmVersion,
			CUDAVersion: req.Evidence.Hardware.CUDAVersion,
		}
	}

	// Software info
	if req.Evidence.Software != nil {
		evidence.Software = &advisorCommon.SoftwareInfo{
			ROCmVersion: req.Evidence.Software.ROCmVersion,
			Packages:    req.Evidence.Software.Packages,
		}
	}

	// Build info
	if req.Evidence.Build != nil {
		evidence.Build = &advisorCommon.BuildInfo{
			BuildURL:      req.Evidence.Build.BuildURL,
			DockerfileURL: req.Evidence.Build.DockerfileURL,
			ImageTag:      req.Evidence.Build.ImageTag,
			BuildDate:     req.Evidence.Build.BuildDate,
			GitCommit:     req.Evidence.Build.GitCommit,
			GitBranch:     req.Evidence.Build.GitBranch,
			GitRepo:       req.Evidence.Build.GitRepo,
			CIPipelineID:  req.Evidence.Build.CIPipelineID,
		}
	}

	return &advisorCommon.WandBDetectionRequest{
		Source:      req.Source,
		Type:        req.Type,
		Version:     req.Version,
		WorkloadUID: req.WorkloadUID,
		PodUID:      req.PodUID,
		PodName:     req.PodName,
		Namespace:   req.Namespace,
		Timestamp:   req.Timestamp,
		Evidence:    evidence,
		Hints: advisorCommon.WandBHints{
			PossibleFrameworks: req.Hints.PossibleFrameworks,
			WrapperFrameworks:  req.Hints.WrapperFrameworks,
			BaseFrameworks:     req.Hints.BaseFrameworks,
		},
	}
}
