// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package logs

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/mapUtil"
	"github.com/sirupsen/logrus"
)

// WandBMetricsRequest WandB metrics reporting request
type WandBMetricsRequest struct {
	Source      string        `json:"source"`                 // "wandb"
	WorkloadUID string        `json:"workload_uid,omitempty"` // Optional (compatibility)
	PodUID      string        `json:"pod_uid,omitempty"`
	PodName     string        `json:"pod_name"` // Required: client gets from environment variable
	RunID       string        `json:"run_id"`   // WandB run id
	Metrics     []WandBMetric `json:"metrics"`  // Metrics data
	Timestamp   float64       `json:"timestamp"`
}

// WandBMetric single metrics data point
type WandBMetric struct {
	Name      string            `json:"name"`      // Metric name, e.g. "loss", "accuracy"
	Value     float64           `json:"value"`     // Metric value
	Step      int64             `json:"step"`      // Training step
	Timestamp float64           `json:"timestamp"` // Collection timestamp
	Tags      map[string]string `json:"tags"`      // Extra tags
}

// WandBLogsRequest WandB training data reporting request (from wandb.log())
// This is for structured training metrics logged via wandb.log(), not text logs
type WandBLogsRequest struct {
	Source      string     `json:"source"`                 // "wandb"
	WorkloadUID string     `json:"workload_uid,omitempty"` // Optional (backward compatibility)
	PodUID      string     `json:"pod_uid,omitempty"`
	PodName     string     `json:"pod_name"` // Required: obtained from environment variable by client
	RunID       string     `json:"run_id"`   // WandB run id
	Logs        []WandBLog `json:"logs"`     // Training data entries (from wandb.log())
	Timestamp   float64    `json:"timestamp"`
}

// WandBLog single training data entry from wandb.log()
// This represents structured metrics logged during training, not text logs
type WandBLog struct {
	Step      int64                  `json:"step"`      // Training step/iteration
	Timestamp float64                `json:"timestamp"` // Timestamp when logged
	Data      map[string]interface{} `json:"data"`      // Training metrics (loss, accuracy, lr, etc.)
}

// MetricsStorage metrics storage interface
type MetricsStorage interface {
	Store(ctx context.Context, metric *StoredMetric) error
	Query(ctx context.Context, workloadUID string, metricName string) ([]*StoredMetric, error)
}

// StoredMetric stored metric format
type StoredMetric struct{
	WorkloadUID string
	PodUID      string
	Source      string
	RunID       string
	Name        string
	Value       float64
	Step        int64
	Timestamp   time.Time
	Tags        map[string]string
}

// WandBLogProcessor WandB log and metrics processor
type WandBLogProcessor struct {
	metricsStorage  MetricsStorage                   // Metrics storage interface
	trainingFacade  database.TrainingFacadeInterface // Training facade (optional, for testing)
}

// NewWandBLogProcessor creates processor
func NewWandBLogProcessor(
	metricsStorage MetricsStorage,
) *WandBLogProcessor {
	return &WandBLogProcessor{
		metricsStorage:  metricsStorage,
		trainingFacade:  nil, // Use default facade from database.GetFacade()
	}
}

// NewWandBLogProcessorWithFacade creates processor with custom training facade (for testing)
func NewWandBLogProcessorWithFacade(
	metricsStorage MetricsStorage,
	trainingFacade database.TrainingFacadeInterface,
) *WandBLogProcessor {
	return &WandBLogProcessor{
		metricsStorage:  metricsStorage,
		trainingFacade:  trainingFacade,
	}
}

// ProcessMetrics processes WandB metrics data
func (p *WandBLogProcessor) ProcessMetrics(
	ctx context.Context,
	req *WandBMetricsRequest,
) error {
	// Record metrics: request count and duration
	startTime := time.Now()
	IncWandBRequestCount("metrics")
	defer func() {
		ObserveWandBRequestDuration("metrics", time.Since(startTime).Seconds())
	}()

	// 1. Parse WorkloadUID from PodName
	workloadUID, err := resolveWorkloadUID(req.WorkloadUID, req.PodName)
	if err != nil {
		IncWandBRequestErrorCount("metrics", "validation")
		return err
	}

	logrus.Infof("Processing WandB metrics for pod %s -> workload %s, %d metrics",
		req.PodName, workloadUID, len(req.Metrics))

	// Record data point count
	ObserveWandBMetricsDataPointCount(workloadUID, len(req.Metrics))

	if len(req.Metrics) == 0 {
		logrus.Debug("No metrics to process")
		return nil
	}

	// 2. Convert to internal format and store to MetricsStorage
	successCount := 0
	errorCount := 0

	for _, metric := range req.Metrics {
		// Construct storage format
		storedMetric := &StoredMetric{
			WorkloadUID: workloadUID,
			PodUID:      req.PodUID,
			Source:      constant.DataSourceWandB, // Use constant from constant package
			RunID:       req.RunID,
			Name:        metric.Name,
			Value:       metric.Value,
			Step:        metric.Step,
			Timestamp:   time.Unix(0, int64(metric.Timestamp*1e9)),
			Tags:        metric.Tags,
		}

		// Store to time-series database or metrics storage
		if err := p.metricsStorage.Store(ctx, storedMetric); err != nil {
			logrus.Errorf("Failed to store metric %s: %v", metric.Name, err)
			IncWandBMetricsStoreErrors(workloadUID)
			errorCount++
			// Continue processing other metrics
			continue
		}
		IncWandBMetricsStoreCount(workloadUID)
		successCount++
	}

	logrus.Infof("✓ WandB metrics stored to MetricsStorage: %d success, %d errors (workload: %s)",
		successCount, errorCount, workloadUID)

	// 3. Aggregate metrics by step, store to training_performance table
	stepMetrics := make(map[int64]map[string]interface{})
	stepTimestamps := make(map[int64]time.Time)

	for _, metric := range req.Metrics {
		step := metric.Step
		if stepMetrics[step] == nil {
			stepMetrics[step] = make(map[string]interface{})
		}
		stepMetrics[step][metric.Name] = metric.Value

		// Record timestamp for each step (use first metric's timestamp for that step)
		if _, exists := stepTimestamps[step]; !exists {
			stepTimestamps[step] = time.Unix(0, int64(metric.Timestamp*1e9))
		}
	}

	logrus.Infof("Aggregated metrics into %d step(s) for training_performance storage", len(stepMetrics))

	// 4. Store metrics for each step to training_performance table
	trainingSuccessCount := 0
	trainingErrorCount := 0

	for step, data := range stepMetrics {
		// Construct data in WandBLog format
		logData := &WandBLog{
			Step:      step,
			Timestamp: float64(stepTimestamps[step].UnixNano()) / 1e9,
			Data:      data,
		}

		// Store to training_performance table
		if err := p.storeTrainingData(ctx, workloadUID, req.PodUID, req.RunID, logData, stepTimestamps[step]); err != nil {
			logrus.Errorf("Failed to store training data for step %d: %v", step, err)
			IncTrainingPerformanceSaveErrors(workloadUID, constant.DataSourceWandB, "db_error")
			trainingErrorCount++
			continue
		}
		IncTrainingPerformanceSaveCount(workloadUID, constant.DataSourceWandB)
		trainingSuccessCount++
	}

	logrus.Infof("✓ WandB metrics stored to training_performance: %d steps success, %d errors (workload: %s)",
		trainingSuccessCount, trainingErrorCount, workloadUID)

	if errorCount > 0 || trainingErrorCount > 0 {
		IncWandBRequestErrorCount("metrics", "storage")
		return fmt.Errorf("failed to store %d metrics to MetricsStorage, %d steps to training_performance",
			errorCount, trainingErrorCount)
	}

	return nil
}

// ProcessLogs processes WandB training data (from wandb.log())
// Stores structured training metrics to training_performance table
func (p *WandBLogProcessor) ProcessLogs(
	ctx context.Context,
	req *WandBLogsRequest,
) error {
	// Record metrics: request count and duration
	startTime := time.Now()
	IncWandBRequestCount("logs")
	defer func() {
		ObserveWandBRequestDuration("logs", time.Since(startTime).Seconds())
	}()

	if len(req.Logs) == 0 {
		logrus.Debug("No training data to process")
		return nil
	}

	// 1. Resolve WorkloadUID from PodName
	workloadUID, err := resolveWorkloadUID(req.WorkloadUID, req.PodName)
	if err != nil {
		IncWandBRequestErrorCount("logs", "validation")
		return err
	}

	logrus.Infof("Processing WandB training data for pod %s -> workload %s, %d entries",
		req.PodName, workloadUID, len(req.Logs))

	// Record data point count
	ObserveWandBLogsDataPointCount(workloadUID, len(req.Logs))

	// 2. Store training data to training_performance table
	successCount := 0
	errorCount := 0

	for _, log := range req.Logs {
		// Convert timestamp
		logTime := time.Unix(0, int64(log.Timestamp*1e9))

		// Store each training data entry as training performance
		if err := p.storeTrainingData(ctx, workloadUID, req.PodUID, req.RunID, &log, logTime); err != nil {
			logrus.Errorf("Failed to store training data at step %d: %v", log.Step, err)
			IncTrainingPerformanceSaveErrors(workloadUID, constant.DataSourceWandB, "db_error")
			errorCount++
			// Continue processing other entries
			continue
		}
		IncTrainingPerformanceSaveCount(workloadUID, constant.DataSourceWandB)
		successCount++
	}

	logrus.Infof("✓ WandB training data processed: %d success, %d errors (workload: %s)",
		successCount, errorCount, workloadUID)

	if errorCount > 0 {
		IncWandBRequestErrorCount("logs", "storage")
		return fmt.Errorf("failed to store %d training data entries", errorCount)
	}

	return nil
}

// storeTrainingData stores WandB training data to training_performance table
// If a record already exists for this step, it merges the old data into history
// and updates with new data (new data takes precedence)
func (p *WandBLogProcessor) storeTrainingData(
	ctx context.Context,
	workloadUID, podUID, runID string,
	data *WandBLog,
	timestamp time.Time,
) error {
	// Prepare new performance data with WandB metrics
	newPerformanceData := make(map[string]interface{})

	// Add WandB metadata
	newPerformanceData["source"] = constant.DataSourceWandB // Use constant from constant package
	newPerformanceData["run_id"] = runID
	newPerformanceData["step"] = data.Step

	// Merge all logged metrics from wandb.log()
	for key, value := range data.Data {
		newPerformanceData[key] = value
	}

	// Use fixed serial=1 for WandB data (similar to other sources)
	serial := 1
	iteration := int(data.Step)

	// Get training facade (use injected one or default)
	trainingFacade := p.trainingFacade
	if trainingFacade == nil {
		trainingFacade = database.GetFacade().GetTraining()
	}

	// Check if performance data already exists for this step
	existingPerf, err := trainingFacade.GetTrainingPerformanceByWorkloadIdSerialAndIteration(
		ctx, workloadUID, serial, iteration)
	if err != nil {
		return fmt.Errorf("failed to check existing performance: %w", err)
	}

	var finalPerformanceData map[string]interface{}
	var recordID int32

	if existingPerf != nil {
		// Record exists - merge old data into history
		logrus.Debugf("Training performance exists for workload %s, step %d, merging with new data",
			workloadUID, data.Step)

		// Decode existing performance data
		existingData := make(map[string]interface{})
		if existingPerf.Performance != nil {
			// Performance is already a map[string]interface{} (ExtType)
			existingData = existingPerf.Performance
		}

		// Create history entry from existing data
		historyEntry := make(map[string]interface{})
		for k, v := range existingData {
			// Skip the history field itself to avoid nested histories
			if k != "history" {
				historyEntry[k] = v
			}
		}
		// Add timestamp to history entry
		historyEntry["updated_at"] = existingPerf.CreatedAt.Format(time.RFC3339)

		// Get existing history if any
		var history []interface{}
		if existingData["history"] != nil {
			if h, ok := existingData["history"].([]interface{}); ok {
				history = h
			}
		}

		// Append old data to history
		history = append(history, historyEntry)

		// Start with existing data
		finalPerformanceData = make(map[string]interface{})
		for k, v := range existingData {
			finalPerformanceData[k] = v
		}

		// Merge new data (new data overwrites old)
		for key, value := range newPerformanceData {
			finalPerformanceData[key] = value
		}

		// Add updated history
		finalPerformanceData["history"] = history
		finalPerformanceData["updated_at"] = timestamp.Format(time.RFC3339)

		// Keep existing record ID for update
		recordID = existingPerf.ID

		logrus.Infof("Merged training data for workload %s, step %d (history entries: %d)",
			workloadUID, data.Step, len(history))
	} else {
		// New record - no merge needed
		finalPerformanceData = newPerformanceData
		finalPerformanceData["created_at"] = timestamp.Format(time.RFC3339)
		recordID = 0 // Will be auto-generated

		logrus.Debugf("Creating new training performance for workload %s, step %d",
			workloadUID, data.Step)
	}

	// Encode final performance data as ExtType (JSON)
	encoded, err := mapUtil.EncodeMap(finalPerformanceData)
	if err != nil {
		return fmt.Errorf("failed to encode performance data: %w", err)
	}

	// Create training performance record
	// For updates, we keep the original CreatedAt timestamp
	createdAt := timestamp
	if existingPerf != nil {
		createdAt = existingPerf.CreatedAt
	}

	perfRecord := &dbModel.TrainingPerformance{
		ID:          recordID,
		PodUUID:     podUID,
		Performance: encoded,
		Iteration:   int32(iteration),
		CreatedAt:   createdAt,
		Serial:      int32(serial),
		WorkloadUID: workloadUID,
		DataSource:  constant.DataSourceWandB, // Use constant from constant package
	}

	// Save training performance (creates if ID=0, updates if ID>0)
	// trainingFacade is already obtained above, reuse it

	if recordID > 0 {
		// Update existing record (merges old data into history)
		if err := trainingFacade.UpdateTrainingPerformance(ctx, perfRecord); err != nil {
			return fmt.Errorf("failed to update training performance: %w", err)
		}

		logrus.Infof("✓ Updated WandB training data for workload %s, step %d (merged with history)",
			workloadUID, data.Step)
	} else {
		// Create new record
		if err := trainingFacade.CreateTrainingPerformance(ctx, perfRecord); err != nil {
			return fmt.Errorf("failed to create training performance: %w", err)
		}

		logrus.Debugf("✓ Created WandB training data for workload %s, step %d with %d metrics",
			workloadUID, data.Step, len(data.Data))
	}

	return nil
}
