// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/alerts"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/log_alert_engine"
)

func executeWorkloadLog(ctx context.Context, workloadLog *PodLog) error {
	if workloadLog.Kubernetes == nil {
		return nil
	}
	if strings.Contains(workloadLog.Kubernetes.PodName, "primus-lens-telemetry-processor") {
		return nil
	}

	// Process workload log
	err := WorkloadLog(ctx,
		workloadLog.Kubernetes.PodId,
		workloadLog.Message,
		workloadLog.Time)
	if err != nil {
		return err
	}

	// Evaluate log against alert rules
	engine := log_alert_engine.GetGlobalEngine()
	if engine != nil {
		// Convert to PodLogData
		logData := convertToPodLogData(workloadLog)
		results := engine.EvaluateLog(logData)
		if len(results) > 0 {
			// Process evaluation results asynchronously
			go processAlertResults(context.Background(), results)
		}
	}

	return nil
}

// convertToPodLogData converts PodLog to PodLogData for the alert engine
func convertToPodLogData(podLog *PodLog) *log_alert_engine.PodLogData {
	logData := &log_alert_engine.PodLogData{
		Time:    podLog.Time,
		Message: podLog.Message,
		Labels:  make(map[string]string),
	}

	if podLog.Kubernetes != nil {
		logData.PodName = podLog.Kubernetes.PodName
		logData.PodId = podLog.Kubernetes.PodId
		logData.Namespace = podLog.Kubernetes.NamespaceName
		logData.Host = podLog.Kubernetes.Host

		// Convert label struct to map
		if podLog.Kubernetes.Labels != nil {
			if podLog.Kubernetes.Labels.App != "" {
				logData.Labels["app"] = podLog.Kubernetes.Labels.App
			}
			if podLog.Kubernetes.Labels.Component != "" {
				logData.Labels["component"] = podLog.Kubernetes.Labels.Component
			}
			if podLog.Kubernetes.Labels.TrainingKubeflowOrgJobName != "" {
				logData.Labels["training.kubeflow.org/job-name"] = podLog.Kubernetes.Labels.TrainingKubeflowOrgJobName
			}
			if podLog.Kubernetes.Labels.TrainingKubeflowOrgReplicaType != "" {
				logData.Labels["training.kubeflow.org/replica-type"] = podLog.Kubernetes.Labels.TrainingKubeflowOrgReplicaType
			}
			if podLog.Kubernetes.Labels.TrainingKubeflowOrgReplicaIndex != "" {
				logData.Labels["training.kubeflow.org/replica-index"] = podLog.Kubernetes.Labels.TrainingKubeflowOrgReplicaIndex
			}
			if podLog.Kubernetes.Labels.TrainingKubeflowOrgOperatorName != "" {
				logData.Labels["training.kubeflow.org/operator-name"] = podLog.Kubernetes.Labels.TrainingKubeflowOrgOperatorName
			}
		}
	}

	return logData
}

// processAlertResults processes evaluation results from rule engine
func processAlertResults(ctx context.Context, results []*log_alert_engine.EvaluationResult) {
	for _, result := range results {
		if err := processAlertResult(ctx, result); err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf(
				"Failed to process alert result for rule %d: %v", result.RuleID, err)
		}
	}
}

// processAlertResult processes a single evaluation result
func processAlertResult(ctx context.Context, result *log_alert_engine.EvaluationResult) error {
	// Build alert from evaluation result
	alert := buildUnifiedAlert(result)

	// Send to alert processor (existing alert system)
	if err := alerts.ProcessAlertFromLog(ctx, alert); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf(
			"Failed to process log alert for rule %s: %v", result.RuleName, err)
		return err
	}

	// Update rule trigger information
	facade := database.GetFacade().GetLogAlertRule()
	if err := facade.UpdateRuleTriggerInfo(ctx, result.RuleID); err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf(
			"Failed to update rule trigger info for rule %d: %v", result.RuleID, err)
	}

	// Update rule statistics
	go updateRuleStatistics(context.Background(), result)

	log.GlobalLogger().WithContext(ctx).Infof(
		"Log alert triggered: rule=%s, severity=%s, workload=%s, pod=%s",
		result.RuleName, result.Severity, result.Context.WorkloadID, result.Context.PodName)

	return nil
}

// buildUnifiedAlert builds a unified alert from evaluation result
func buildUnifiedAlert(result *log_alert_engine.EvaluationResult) *alerts.UnifiedAlert {
	now := time.Now()

	// Generate alert ID
	alertID := fmt.Sprintf("log-%d-%d", result.RuleID, now.Unix())

	// Build labels
	labels := make(map[string]string)
	labels["alertname"] = result.RuleName
	labels["severity"] = result.Severity
	labels["source"] = "log"
	labels["rule_id"] = fmt.Sprintf("%d", result.RuleID)

	if result.Context.WorkloadID != "" {
		labels["workload_id"] = result.Context.WorkloadID
	}
	if result.Context.PodName != "" {
		labels["pod_name"] = result.Context.PodName
	}
	if result.Context.NodeName != "" {
		labels["node_name"] = result.Context.NodeName
	}
	if result.Context.Namespace != "" {
		labels["namespace"] = result.Context.Namespace
	}
	if result.Context.ClusterName != "" {
		labels["cluster_name"] = result.Context.ClusterName
	}

	// Add template labels
	for k, v := range result.AlertTemplate.Labels {
		labels[k] = v
	}

	// Build annotations
	annotations := make(map[string]string)
	annotations["summary"] = renderTemplate(result.AlertTemplate.Summary, result.Context)
	annotations["description"] = renderTemplate(result.AlertTemplate.Description, result.Context)
	annotations["log_message"] = truncateString(result.Context.Message, 500)
	annotations["match_reason"] = result.MatchReason

	// Add template annotations
	for k, v := range result.AlertTemplate.Annotations {
		annotations[k] = renderTemplate(v, result.Context)
	}

	// Build raw data
	rawData := map[string]interface{}{
		"rule_id":      result.RuleID,
		"rule_name":    result.RuleName,
		"log_time":     result.Context.LogTime,
		"eval_time_ms": result.EvalTimeMs,
		"match_reason": result.MatchReason,
	}
	rawDataBytes, _ := json.Marshal(rawData)

	alert := &alerts.UnifiedAlert{
		ID:          alertID,
		Source:      "log",
		AlertName:   result.RuleName,
		Severity:    result.Severity,
		Status:      "firing",
		StartsAt:    now,
		Labels:      labels,
		Annotations: annotations,
		WorkloadID:  result.Context.WorkloadID,
		PodName:     result.Context.PodName,
		PodID:       result.Context.PodID,
		NodeName:    result.Context.NodeName,
		ClusterName: result.Context.ClusterName,
		RawData:     rawDataBytes,
	}

	return alert
}

// renderTemplate renders a template string with context values
func renderTemplate(template string, ctx *log_alert_engine.EvaluationContext) string {
	result := template

	// Replace placeholders
	replacements := map[string]string{
		"{{.WorkloadID}}":   ctx.WorkloadID,
		"{{.WorkloadName}}": ctx.WorkloadID, // Use workload ID as name for now
		"{{.PodName}}":      ctx.PodName,
		"{{.NodeName}}":     ctx.NodeName,
		"{{.Namespace}}":    ctx.Namespace,
		"{{.ClusterName}}":  ctx.ClusterName,
		"{{.LogMessage}}":   truncateString(ctx.Message, 200),
	}

	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// updateRuleStatistics updates statistics for a rule
func updateRuleStatistics(ctx context.Context, result *log_alert_engine.EvaluationResult) {
	facade := database.GetFacade().GetLogAlertRule()

	date := result.Context.LogTime.Truncate(24 * time.Hour)
	hour := result.Context.LogTime.Hour()

	stat := &model.LogAlertRuleStatistics{
		RuleID:         result.RuleID,
		Date:           date,
		Hour:           int32(hour),
		ClusterName:    result.Context.ClusterName,
		EvaluatedCount: 1,
		MatchedCount:   1,
		FiredCount:     1,
		AvgEvalTimeMs:  result.EvalTimeMs,
		MaxEvalTimeMs:  result.EvalTimeMs,
	}

	if err := facade.CreateOrUpdateRuleStatistic(ctx, stat); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf(
			"Failed to update rule statistics for rule %d: %v", result.RuleID, err)
	}

	// Also update daily statistics
	dailyStat := &model.LogAlertRuleStatistics{
		RuleID:         result.RuleID,
		Date:           date,
		Hour:           0, // 0 means daily aggregate
		ClusterName:    result.Context.ClusterName,
		EvaluatedCount: 1,
		MatchedCount:   1,
		FiredCount:     1,
		AvgEvalTimeMs:  result.EvalTimeMs,
		MaxEvalTimeMs:  result.EvalTimeMs,
	}

	if err := facade.CreateOrUpdateRuleStatistic(ctx, dailyStat); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf(
			"Failed to update daily rule statistics for rule %d: %v", result.RuleID, err)
	}
}
