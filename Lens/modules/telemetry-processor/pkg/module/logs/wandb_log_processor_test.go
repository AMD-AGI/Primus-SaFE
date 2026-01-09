// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package logs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInMemoryMetricsStorage_StoreAndQuery(t *testing.T) {
	storage := NewInMemoryMetricsStorage(100)
	ctx := context.Background()

	// Store metrics
	metric1 := &StoredMetric{
		WorkloadUID: "workload-123",
		PodUID:      "pod-456",
		Source:      "wandb",
		Name:        "loss",
		Value:       2.5,
		Step:        0,
		Timestamp:   time.Now(),
	}

	metric2 := &StoredMetric{
		WorkloadUID: "workload-123",
		PodUID:      "pod-456",
		Source:      "wandb",
		Name:        "loss",
		Value:       2.0,
		Step:        100,
		Timestamp:   time.Now(),
	}

	metric3 := &StoredMetric{
		WorkloadUID: "workload-123",
		PodUID:      "pod-456",
		Source:      "wandb",
		Name:        "accuracy",
		Value:       0.85,
		Step:        100,
		Timestamp:   time.Now(),
	}

	// Store
	err := storage.Store(ctx, metric1)
	assert.NoError(t, err)
	err = storage.Store(ctx, metric2)
	assert.NoError(t, err)
	err = storage.Store(ctx, metric3)
	assert.NoError(t, err)

	// Query all metrics
	allMetrics, err := storage.Query(ctx, "workload-123", "")
	assert.NoError(t, err)
	assert.Equal(t, 3, len(allMetrics))

	// Query specific metric
	lossMetrics, err := storage.Query(ctx, "workload-123", "loss")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(lossMetrics))

	accMetrics, err := storage.Query(ctx, "workload-123", "accuracy")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(accMetrics))

	// Query non-existent workload
	noMetrics, err := storage.Query(ctx, "workload-999", "")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(noMetrics))
}

func TestInMemoryMetricsStorage_MaxSize(t *testing.T) {
	storage := NewInMemoryMetricsStorage(10) // Maximum 10 entries
	ctx := context.Background()

	// Store 15 metrics
	for i := 0; i < 15; i++ {
		metric := &StoredMetric{
			WorkloadUID: "workload-123",
			PodUID:      "pod-456",
			Source:      "wandb",
			Name:        "loss",
			Value:       float64(i),
			Step:        int64(i),
			Timestamp:   time.Now(),
		}
		err := storage.Store(ctx, metric)
		assert.NoError(t, err)
	}

	// Should keep only the latest 10 entries
	metrics, err := storage.Query(ctx, "workload-123", "")
	assert.NoError(t, err)
	assert.Equal(t, 10, len(metrics))

	// Verify these are the latest 10 entries (step 5-14)
	assert.Equal(t, float64(5), metrics[0].Value)
	assert.Equal(t, float64(14), metrics[9].Value)
}

func TestInMemoryMetricsStorage_CleanupOldMetrics(t *testing.T) {
	storage := NewInMemoryMetricsStorage(100)
	ctx := context.Background()

	// Store some old metrics
	oldMetric := &StoredMetric{
		WorkloadUID: "workload-123",
		PodUID:      "pod-456",
		Source:      "wandb",
		Name:        "loss",
		Value:       2.5,
		Step:        0,
		Timestamp:   time.Now().Add(-2 * time.Hour), // 2 hours ago
	}

	// Store some new metrics
	newMetric := &StoredMetric{
		WorkloadUID: "workload-123",
		PodUID:      "pod-456",
		Source:      "wandb",
		Name:        "loss",
		Value:       1.5,
		Step:        100,
		Timestamp:   time.Now(),
	}

	err := storage.Store(ctx, oldMetric)
	assert.NoError(t, err)
	err = storage.Store(ctx, newMetric)
	assert.NoError(t, err)

	// Cleanup metrics older than 1 hour
	deleted := storage.CleanupOldMetrics(1 * time.Hour)
	assert.Equal(t, 1, deleted)

	// Verify only new metrics remain
	metrics, err := storage.Query(ctx, "workload-123", "")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(metrics))
	assert.Equal(t, float64(1.5), metrics[0].Value)
}

func TestWandBLogProcessor_ProcessMetrics(t *testing.T) {
	storage := NewInMemoryMetricsStorage(100)
	mockTrainingFacade := NewMockTrainingFacade()
	processor := NewWandBLogProcessorWithFacade(storage, mockTrainingFacade)
	ctx := context.Background()

	tests := []struct {
		name    string
		req     *WandBMetricsRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid metrics request",
			req: &WandBMetricsRequest{
				WorkloadUID: "workload-123",
				PodUID:      "pod-456",
				RunID:       "run-789",
				Metrics: []WandBMetric{
					{
						Name:      "loss",
						Value:     2.5,
						Step:      0,
						Timestamp: float64(time.Now().Unix()),
					},
					{
						Name:      "accuracy",
						Value:     0.85,
						Step:      0,
						Timestamp: float64(time.Now().Unix()),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Missing workload_uid",
			req: &WandBMetricsRequest{
				Metrics: []WandBMetric{{Name: "loss", Value: 2.5}},
			},
			wantErr: true,
			errMsg:  "either workload_uid or pod_name is required",
		},
		{
			name: "Empty metrics",
			req: &WandBMetricsRequest{
				WorkloadUID: "workload-123",
				Metrics:     []WandBMetric{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.ProcessMetrics(ctx, tt.req)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)

				// Verify metrics are stored
				if len(tt.req.Metrics) > 0 {
					metrics, err := storage.Query(ctx, tt.req.WorkloadUID, "")
					assert.NoError(t, err)
					assert.Equal(t, len(tt.req.Metrics), len(metrics))
				}
			}
		})
	}
}

func TestWandBLogProcessor_ProcessLogs(t *testing.T) {
	storage := NewInMemoryMetricsStorage(100)
	mockTrainingFacade := NewMockTrainingFacade()
	processor := NewWandBLogProcessorWithFacade(storage, mockTrainingFacade)
	ctx := context.Background()

	tests := []struct {
		name    string
		req     *WandBLogsRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid training data request",
			req: &WandBLogsRequest{
				WorkloadUID: "workload-123",
				PodUID:      "pod-456",
				RunID:       "run-789",
				Logs: []WandBLog{
					{
						Step:      100,
						Timestamp: float64(time.Now().Unix()),
						Data: map[string]interface{}{
							"loss":     0.5,
							"accuracy": 0.92,
							"lr":       0.001,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Missing workload_uid and pod_name",
			req: &WandBLogsRequest{
				Logs: []WandBLog{{
					Step:      50,
					Timestamp: float64(time.Now().Unix()),
					Data:      map[string]interface{}{"loss": 0.8},
				}},
			},
			wantErr: true,
			errMsg:  "either workload_uid or pod_name is required",
		},
		{
			name: "Empty logs",
			req: &WandBLogsRequest{
				WorkloadUID: "workload-123",
				Logs:        []WandBLog{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: Since ProcessLogs calls WorkloadLog which requires database,
			// this test may fail without complete test environment
			// Here we mainly test parameter validation logic
			err := processor.ProcessLogs(ctx, tt.req)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			}
			// Note: Success cases not verified due to database dependency
		})
	}
}

func TestInMemoryMetricsStorage_GetMetricsCount(t *testing.T) {
	storage := NewInMemoryMetricsStorage(100)
	ctx := context.Background()

	assert.Equal(t, 0, storage.GetMetricsCount())

	// Add some metrics
	for i := 0; i < 5; i++ {
		metric := &StoredMetric{
			WorkloadUID: "workload-123",
			Name:        "loss",
			Value:       float64(i),
			Timestamp:   time.Now(),
		}
		err := storage.Store(ctx, metric)
		assert.NoError(t, err)
	}

	assert.Equal(t, 5, storage.GetMetricsCount())

	// Add metrics for another workload
	for i := 0; i < 3; i++ {
		metric := &StoredMetric{
			WorkloadUID: "workload-456",
			Name:        "accuracy",
			Value:       float64(i),
			Timestamp:   time.Now(),
		}
		err := storage.Store(ctx, metric)
		assert.NoError(t, err)
	}

	assert.Equal(t, 8, storage.GetMetricsCount())
}
