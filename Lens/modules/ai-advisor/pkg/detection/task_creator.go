package detection

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

const (
	// TaskTypeInferenceMetricsScrape is the task type for inference metrics scraping
	TaskTypeInferenceMetricsScrape = "inference_metrics_scrape"
)

// TaskCreator responsible for creating metadata collection tasks after framework detection completes
type TaskCreator struct {
	taskFacade     database.WorkloadTaskFacadeInterface
	podFacade      database.PodFacadeInterface
	workloadFacade database.WorkloadFacadeInterface
	instanceID     string
	autoCreateTask bool // whether to auto create tasks
}

// NewTaskCreator creates task creator
func NewTaskCreator(instanceID string) *TaskCreator {
	return &TaskCreator{
		taskFacade:     database.NewWorkloadTaskFacade(),
		podFacade:      database.GetFacade().GetPod(),
		workloadFacade: database.GetFacade().GetWorkload(),
		instanceID:     instanceID,
		autoCreateTask: true, // auto creation enabled by default
	}
}

// SetAutoCreateTask sets whether to auto create tasks
func (tc *TaskCreator) SetAutoCreateTask(auto bool) {
	tc.autoCreateTask = auto
}

// OnDetectionCompleted called when detection completes
// Creates metadata collection task and profiler collection task based on detection result
func (tc *TaskCreator) OnDetectionCompleted(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	if !tc.autoCreateTask {
		log.Debugf("Auto task creation disabled, skipping task creation for workload %s", workloadUID)
		return nil
	}

	log.Infof("Processing detection for workload %s (frameworks: %v, status: %s)",
		workloadUID, detection.Frameworks, detection.Status)

	// Check if this is an inference workload
	if tc.isInferenceWorkload(detection) {
		// Create inference metrics scrape task for inference workloads
		if detection.Status == coreModel.DetectionStatusConfirmed ||
			detection.Status == coreModel.DetectionStatusVerified {
			if err := tc.createInferenceMetricsScrapeTask(ctx, workloadUID, detection); err != nil {
				log.Warnf("Failed to create inference metrics scrape task for workload %s: %v", workloadUID, err)
			}
		}
		// Skip training-specific tasks for inference workloads
		return nil
	}

	// 1. Try to create profiler collection task early (less strict requirements)
	// Only requires: PyTorch framework + Training workload
	// Don't wait for detection status to be confirmed/verified to avoid missing early profiler files
	if err := tc.createProfilerCollectionTask(ctx, workloadUID, detection); err != nil {
		log.Warnf("Failed to create profiler collection task for workload %s: %v", workloadUID, err)
		// Continue - don't block metadata collection task creation
	}

	// 2. Create metadata collection task (requires confirmed/verified status)
	// Only create tasks for confirmed or verified detections
	if detection.Status != coreModel.DetectionStatusConfirmed &&
		detection.Status != coreModel.DetectionStatusVerified {
		log.Debugf("Detection status is %s (not confirmed/verified), skipping metadata collection task for workload %s",
			detection.Status, workloadUID)
		return nil
	}

	// Only create metadata collection tasks for training workloads
	if !tc.isTrainingWorkload(detection) {
		log.Debugf("Workload %s is not a training task, skipping metadata collection task", workloadUID)
		return nil
	}

	log.Infof("Creating metadata collection task for workload %s (frameworks: %v)",
		workloadUID, detection.Frameworks)

	if err := tc.createMetadataCollectionTask(ctx, workloadUID, detection); err != nil {
		return fmt.Errorf("failed to create metadata collection task: %w", err)
	}

	return nil
}

// createMetadataCollectionTask creates metadata collection task
func (tc *TaskCreator) createMetadataCollectionTask(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	// Note: workload-related specific info (pod, node, etc.) is stored in ai_workload_metadata table
	// ext here only stores task execution context
	task := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    constant.TaskTypeMetadataCollection,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			// Task execution configuration
			"auto_restart":        true,
			"priority":            100,
			"max_retries":         3,
			"retry_count":         0,
			"timeout":             30, // 30 second timeout
			"include_tensorboard": true,
			"include_metrics":     true,

			// Task metadata
			"created_by":   "detection_manager",
			"created_at":   time.Now().Format(time.RFC3339),
			"triggered_by": "framework_detection",

			// Detection summary info (for logging and debugging)
			"detection_frameworks": detection.Frameworks,
			"detection_confidence": detection.Confidence,
		},
	}

	if err := tc.taskFacade.UpsertTask(ctx, task); err != nil {
		return fmt.Errorf("failed to upsert task: %w", err)
	}

	log.Infof("Metadata collection task created for workload %s", workloadUID)
	return nil
}

// createProfilerCollectionTask creates profiler collection task
// Requirements: PyTorch framework + Training workload (no detection status check)
// This allows early profiler file collection to avoid missing training startup files
func (tc *TaskCreator) createProfilerCollectionTask(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	// Check 1: Must be PyTorch framework
	if !tc.isPyTorchFramework(detection) {
		log.Debugf("Workload %s is not PyTorch, skipping profiler collection", workloadUID)
		return nil
	}

	// Check 2: Must be training workload
	if !tc.isTrainingWorkload(detection) {
		log.Debugf("Workload %s is not a training task, skipping profiler collection", workloadUID)
		return nil
	}

	log.Infof("Creating profiler collection task for PyTorch training workload %s (early trigger, detection status: %s)",
		workloadUID, detection.Status)

	task := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    constant.TaskTypeProfilerCollection,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			// Task execution configuration
			"auto_restart":        true,
			"priority":            50, // Lower than metadata collection
			"max_retries":         3,
			"retry_count":         0,
			"timeout":             600, // 10 minutes
			"collection_interval": 300, // 5 minutes
			"max_executions":      0,   // Unlimited, until training stops
			"execution_count":     0,

			// Collection configuration
			"auto_collect":   true,
			"min_confidence": "medium",
			"max_file_size":  1073741824, // 1GB

			// Task metadata
			"created_by":   "detection_manager",
			"created_at":   time.Now().Format(time.RFC3339),
			"triggered_by": "framework_detection_early", // Early trigger (no status check)

			// Detection summary info (for logging and debugging)
			"detection_frameworks": detection.Frameworks,
			"detection_confidence": detection.Confidence,
			"detection_status":     string(detection.Status), // Record detection status for reference

			// Note: Task created early (before detection confirmed) to avoid missing startup profiler files
			"early_trigger": true,
		},
	}

	if err := tc.taskFacade.UpsertTask(ctx, task); err != nil {
		return fmt.Errorf("failed to upsert task: %w", err)
	}

	log.Infof("Profiler collection task created successfully for workload %s", workloadUID)
	return nil
}

// createInferenceMetricsScrapeTask creates inference metrics scrape task
// This task is picked up by inference-metrics-exporter to scrape metrics from inference services
func (tc *TaskCreator) createInferenceMetricsScrapeTask(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	// Get primary framework name
	frameworkName := tc.getPrimaryInferenceFramework(detection)
	if frameworkName == "" {
		log.Debugf("No inference framework detected for workload %s, skipping metrics scrape task", workloadUID)
		return nil
	}

	// Get pod info for this workload
	podInfo, err := tc.getInferencePodInfo(ctx, workloadUID)
	if err != nil {
		return fmt.Errorf("failed to get pod info: %w", err)
	}

	if podInfo == nil || podInfo.IP == "" {
		log.Debugf("No pod with IP found for workload %s, skipping metrics scrape task", workloadUID)
		return nil
	}

	// Get metrics port from framework config
	metricsPort, metricsPath := tc.getInferenceMetricsEndpoint(frameworkName)

	log.Infof("Creating inference metrics scrape task for workload %s (framework=%s, pod=%s/%s, ip=%s, port=%d)",
		workloadUID, frameworkName, podInfo.Namespace, podInfo.Name, podInfo.IP, metricsPort)

	// Build ext data for the task
	extData := map[string]interface{}{
		"framework":       frameworkName,
		"pod_ip":          podInfo.IP,
		"metrics_port":    metricsPort,
		"metrics_path":    metricsPath,
		"scrape_interval": 15, // default 15 seconds
		"labels": map[string]string{
			"namespace":     podInfo.Namespace,
			"pod_name":      podInfo.Name,
			"workload_uid":  workloadUID,
			"workload_name": podInfo.WorkloadName,
			"framework":     frameworkName,
		},
		// Task metadata
		"created_by":   "detection_manager",
		"created_at":   time.Now().Format(time.RFC3339),
		"triggered_by": "framework_detection",
		// Detection info
		"detection_confidence": detection.Confidence,
	}

	task := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    TaskTypeInferenceMetricsScrape,
		Status:      constant.TaskStatusPending,
		Ext:         extData,
	}

	if err := tc.taskFacade.UpsertTask(ctx, task); err != nil {
		return fmt.Errorf("failed to upsert inference metrics scrape task: %w", err)
	}

	log.Infof("Inference metrics scrape task created for workload %s (framework=%s, endpoint=http://%s:%d%s)",
		workloadUID, frameworkName, podInfo.IP, metricsPort, metricsPath)
	return nil
}

// InferencePodInfo contains pod information for inference services
type InferencePodInfo struct {
	Name         string
	Namespace    string
	IP           string
	WorkloadName string
}

// getInferencePodInfo retrieves pod information for an inference workload
func (tc *TaskCreator) getInferencePodInfo(ctx context.Context, workloadUID string) (*InferencePodInfo, error) {
	// Try to find pods through workload_pod_reference table
	podRefs, err := tc.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to query workload_pod_reference for workload %s: %v", workloadUID, err)
	}

	var pods []*model.GpuPods
	if len(podRefs) > 0 {
		// Query pod details through pod UID list
		podUIDs := make([]string, 0, len(podRefs))
		for _, ref := range podRefs {
			podUIDs = append(podUIDs, ref.PodUID)
		}

		// Get pod details from gpu_pods table
		db := database.GetFacade().GetSystemConfig().GetDB()
		err = db.WithContext(ctx).
			Where("uid IN ? AND deleted = ? AND running = ?", podUIDs, false, true).
			Find(&pods).Error
		if err != nil {
			return nil, fmt.Errorf("failed to query pods by references: %w", err)
		}
	}

	// If no pods found through references, try child workloads
	if len(pods) == 0 {
		childWorkloads, err := tc.workloadFacade.ListChildrenWorkloadByParentUid(ctx, workloadUID)
		if err != nil {
			log.Warnf("Failed to query child workloads for %s: %v", workloadUID, err)
		} else if len(childWorkloads) > 0 {
			for _, child := range childWorkloads {
				childPodInfo, err := tc.getInferencePodInfo(ctx, child.UID)
				if err == nil && childPodInfo != nil && childPodInfo.IP != "" {
					return childPodInfo, nil
				}
			}
		}
	}

	if len(pods) == 0 {
		return nil, nil
	}

	// Get workload name
	workloadName := ""
	workload, err := tc.workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err == nil && workload != nil {
		workloadName = workload.Name
	}

	// Return the first running pod with an IP
	for _, pod := range pods {
		if pod.IP != "" && pod.Running {
			return &InferencePodInfo{
				Name:         pod.Name,
				Namespace:    pod.Namespace,
				IP:           pod.IP,
				WorkloadName: workloadName,
			}, nil
		}
	}

	// If no running pod with IP, return first pod
	if len(pods) > 0 {
		return &InferencePodInfo{
			Name:         pods[0].Name,
			Namespace:    pods[0].Namespace,
			IP:           pods[0].IP,
			WorkloadName: workloadName,
		}, nil
	}

	return nil, nil
}

// getPrimaryInferenceFramework returns the primary inference framework from detection
func (tc *TaskCreator) getPrimaryInferenceFramework(detection *coreModel.FrameworkDetection) string {
	if detection == nil {
		return ""
	}

	// Check detection type
	if detection.Type != FrameworkTypeInference {
		return ""
	}

	// Return the first framework
	if len(detection.Frameworks) > 0 {
		return detection.Frameworks[0]
	}

	// Check sources for inference frameworks
	for _, source := range detection.Sources {
		if source.Type == FrameworkTypeInference && len(source.Frameworks) > 0 {
			return source.Frameworks[0]
		}
	}

	return ""
}

// getInferenceMetricsEndpoint returns the metrics port and path for an inference framework
func (tc *TaskCreator) getInferenceMetricsEndpoint(frameworkName string) (port int, path string) {
	// Default values
	port = 8000
	path = "/metrics"

	configMgr := GetConfigManager()
	if configMgr == nil {
		return port, path
	}

	// Try to get framework config from cache first
	fwConfig := configMgr.GetFramework(frameworkName)
	if fwConfig == nil {
		// Try to load it
		var err error
		fwConfig, err = configMgr.LoadFrameworkConfig(context.Background(), frameworkName)
		if err != nil {
			log.Debugf("Failed to get framework config for %s: %v, using defaults", frameworkName, err)
			return port, path
		}
	}

	// Check extensions for metrics_port
	if fwConfig.Extensions != nil {
		if metricsPort, ok := fwConfig.Extensions["metrics_port"]; ok {
			if p, ok := metricsPort.(float64); ok {
				port = int(p)
			}
		} else if defaultPort, ok := fwConfig.Extensions["default_port"]; ok {
			if p, ok := defaultPort.(float64); ok {
				port = int(p)
			}
		}
	}

	// Check inference patterns for ports
	if fwConfig.InferencePatterns != nil && len(fwConfig.InferencePatterns.Ports) > 0 {
		// Use the first port as default metrics port
		port = fwConfig.InferencePatterns.Ports[0]
	}

	// Get health endpoint (often same as metrics path pattern)
	// For most frameworks, metrics are at /metrics
	// But some frameworks have specific endpoints

	return port, path
}

// getFrameworkConfigFromDB retrieves framework config from database
func (tc *TaskCreator) getFrameworkConfigFromDB(ctx context.Context, frameworkName string) (*FrameworkLogPatterns, error) {
	sysConfigFacade := database.GetFacade().GetSystemConfig()
	key := ConfigKeyPrefix + "." + frameworkName

	sysConfig, err := sysConfigFacade.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}

	if sysConfig == nil {
		return nil, fmt.Errorf("framework config not found: %s", frameworkName)
	}

	// Marshal the map to JSON bytes first, then unmarshal to struct
	valueBytes, err := json.Marshal(sysConfig.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal framework config value: %w", err)
	}

	var config FrameworkLogPatterns
	if err := json.Unmarshal(valueBytes, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal framework config: %w", err)
	}

	return &config, nil
}

// isTrainingWorkload determines if it's a training workload
func (tc *TaskCreator) isTrainingWorkload(detection *coreModel.FrameworkDetection) bool {
	if detection == nil {
		return true // Default to training for safety
	}

	// Check 1: Explicit type field on detection (highest priority)
	if detection.Type == FrameworkTypeInference {
		return false
	}
	if detection.Type == FrameworkTypeTraining {
		return true
	}

	// Check 2: Analyze sources for type information
	hasInference := false
	hasTraining := false

	for _, source := range detection.Sources {
		switch source.Type {
		case FrameworkTypeInference:
			hasInference = true
		case FrameworkTypeTraining:
			hasTraining = true
		case "": // Empty type counts as training for backward compatibility
			hasTraining = true
		}
	}

	// If any source is explicitly marked as inference and none as training
	if hasInference && !hasTraining {
		return false
	}

	// Default to training task (backward compatible)
	return true
}

// isInferenceWorkload determines if it's an inference workload
func (tc *TaskCreator) isInferenceWorkload(detection *coreModel.FrameworkDetection) bool {
	return !tc.isTrainingWorkload(detection)
}

// isPyTorchFramework checks if detection contains PyTorch framework
// Checks multiple indicators:
// 1. Framework name contains "pytorch" or "torch"
// 2. Detection sources contain PyTorch indicators
// 3. Base framework or wrapper framework uses PyTorch (megatron is PyTorch-based)
func (tc *TaskCreator) isPyTorchFramework(detection *coreModel.FrameworkDetection) bool {
	if detection == nil {
		return false
	}

	// Check 1: Framework list contains "pytorch" or "torch"
	for _, framework := range detection.Frameworks {
		fw := strings.ToLower(framework)
		if fw == "pytorch" || strings.Contains(fw, "torch") {
			return true
		}
	}

	// Check 2: Megatron is PyTorch-based, so if megatron is detected, consider it PyTorch
	for _, framework := range detection.Frameworks {
		fw := strings.ToLower(framework)
		if fw == "megatron" || strings.Contains(fw, "megatron") {
			log.Debugf("Detected Megatron framework (PyTorch-based), treating as PyTorch workload")
			return true
		}
	}

	// Check 3: Check detection sources for PyTorch evidence
	for _, source := range detection.Sources {
		for _, fw := range source.Frameworks {
			fwLower := strings.ToLower(fw)
			if fwLower == "pytorch" || strings.Contains(fwLower, "torch") {
				return true
			}
			// Megatron is also PyTorch-based
			if fwLower == "megatron" || strings.Contains(fwLower, "megatron") {
				return true
			}
		}
	}

	return false
}

// extractSourceNames extracts detection source names
func (tc *TaskCreator) extractSourceNames(detection *coreModel.FrameworkDetection) []string {
	sources := []string{}
	seen := make(map[string]bool)

	for _, source := range detection.Sources {
		if !seen[source.Source] {
			sources = append(sources, source.Source)
			seen[source.Source] = true
		}
	}

	return sources
}

// RegisterWithDetectionManager registers TaskCreator with DetectionManager
// as a detection event listener
func RegisterTaskCreatorWithDetectionManager(
	detectionMgr *framework.FrameworkDetectionManager,
	instanceID string,
) *TaskCreator {
	taskCreator := NewTaskCreator(instanceID)

	// Create an adapter to convert DetectionEvent to TaskCreator calls
	listener := &detectionEventAdapter{
		taskCreator: taskCreator,
	}

	detectionMgr.RegisterListener(listener)

	log.Info("TaskCreator registered with DetectionManager as event listener")
	return taskCreator
}

// detectionEventAdapter adapts DetectionEvent to TaskCreator
type detectionEventAdapter struct {
	taskCreator *TaskCreator
}

// OnDetectionEvent implements DetectionEventListener interface
func (a *detectionEventAdapter) OnDetectionEvent(
	ctx context.Context,
	event *framework.DetectionEvent,
) error {
	// Only handle completed and updated events
	if event.Type != framework.DetectionEventTypeUpdated &&
		event.Type != framework.DetectionEventTypeCompleted {
		return nil
	}

	if event.Detection == nil {
		return nil
	}

	// Call TaskCreator to create task
	return a.taskCreator.OnDetectionCompleted(ctx, event.WorkloadUID, event.Detection)
}

