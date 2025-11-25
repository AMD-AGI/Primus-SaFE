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

	// 存储指标
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

	// 存储
	err := storage.Store(ctx, metric1)
	assert.NoError(t, err)
	err = storage.Store(ctx, metric2)
	assert.NoError(t, err)
	err = storage.Store(ctx, metric3)
	assert.NoError(t, err)

	// 查询所有指标
	allMetrics, err := storage.Query(ctx, "workload-123", "")
	assert.NoError(t, err)
	assert.Equal(t, 3, len(allMetrics))

	// 查询特定指标
	lossMetrics, err := storage.Query(ctx, "workload-123", "loss")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(lossMetrics))

	accMetrics, err := storage.Query(ctx, "workload-123", "accuracy")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(accMetrics))

	// 查询不存在的 workload
	noMetrics, err := storage.Query(ctx, "workload-999", "")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(noMetrics))
}

func TestInMemoryMetricsStorage_MaxSize(t *testing.T) {
	storage := NewInMemoryMetricsStorage(10) // 最多存储 10 条
	ctx := context.Background()

	// 存储 15 条指标
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

	// 应该只保留最新的 10 条
	metrics, err := storage.Query(ctx, "workload-123", "")
	assert.NoError(t, err)
	assert.Equal(t, 10, len(metrics))

	// 验证是最新的 10 条（step 5-14）
	assert.Equal(t, float64(5), metrics[0].Value)
	assert.Equal(t, float64(14), metrics[9].Value)
}

func TestInMemoryMetricsStorage_CleanupOldMetrics(t *testing.T) {
	storage := NewInMemoryMetricsStorage(100)
	ctx := context.Background()

	// 存储一些旧指标
	oldMetric := &StoredMetric{
		WorkloadUID: "workload-123",
		PodUID:      "pod-456",
		Source:      "wandb",
		Name:        "loss",
		Value:       2.5,
		Step:        0,
		Timestamp:   time.Now().Add(-2 * time.Hour), // 2 小时前
	}

	// 存储一些新指标
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

	// 清理超过 1 小时的指标
	deleted := storage.CleanupOldMetrics(1 * time.Hour)
	assert.Equal(t, 1, deleted)

	// 验证只剩下新指标
	metrics, err := storage.Query(ctx, "workload-123", "")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(metrics))
	assert.Equal(t, float64(1.5), metrics[0].Value)
}

func TestWandBLogProcessor_ProcessMetrics(t *testing.T) {
	storage := NewInMemoryMetricsStorage(100)
	processor := NewWandBLogProcessor(storage)
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
			errMsg:  "workload_uid is required",
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

				// 验证指标已存储
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
	processor := NewWandBLogProcessor(storage)
	ctx := context.Background()

	tests := []struct {
		name    string
		req     *WandBLogsRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid logs request",
			req: &WandBLogsRequest{
				WorkloadUID: "workload-123",
				PodUID:      "pod-456",
				RunID:       "run-789",
				Logs: []WandBLog{
					{
						Level:     "info",
						Message:   "[Primus] Training started",
						Timestamp: float64(time.Now().Unix()),
						Source:    "stdout",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Missing workload_uid",
			req: &WandBLogsRequest{
				Logs: []WandBLog{{Message: "test"}},
			},
			wantErr: true,
			errMsg:  "workload_uid is required",
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
			// 注意：由于 ProcessLogs 调用 WorkloadLog，而 WorkloadLog 需要数据库，
			// 在没有完整测试环境的情况下，这个测试可能会失败
			// 这里主要测试参数验证逻辑
			err := processor.ProcessLogs(ctx, tt.req)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			}
			// 注意：由于依赖数据库，这里不验证成功情况
		})
	}
}

func TestInMemoryMetricsStorage_GetMetricsCount(t *testing.T) {
	storage := NewInMemoryMetricsStorage(100)
	ctx := context.Background()

	assert.Equal(t, 0, storage.GetMetricsCount())

	// 添加一些指标
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

	// 添加另一个 workload 的指标
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

