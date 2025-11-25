package logs

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// WandBMetricsRequest WandB 指标上报请求
type WandBMetricsRequest struct {
	Source      string        `json:"source"`       // "wandb"
	WorkloadUID string        `json:"workload_uid"` // 必需
	PodUID      string        `json:"pod_uid"`
	RunID       string        `json:"run_id"` // WandB run id
	Metrics     []WandBMetric `json:"metrics"`      // 指标数据
	Timestamp   float64       `json:"timestamp"`
}

// WandBMetric 单个指标数据点
type WandBMetric struct {
	Name      string            `json:"name"`      // 指标名称，如 "loss", "accuracy"
	Value     float64           `json:"value"`     // 指标值
	Step      int64             `json:"step"`      // 训练步数
	Timestamp float64           `json:"timestamp"` // 采集时间戳
	Tags      map[string]string `json:"tags"`      // 额外标签
}

// WandBLogsRequest WandB 日志上报请求
type WandBLogsRequest struct {
	Source      string      `json:"source"`       // "wandb"
	WorkloadUID string      `json:"workload_uid"` // 必需
	PodUID      string      `json:"pod_uid"`
	RunID       string      `json:"run_id"` // WandB run id
	Logs        []WandBLog  `json:"logs"`         // 日志条目
	Timestamp   float64     `json:"timestamp"`
}

// WandBLog 单条日志
type WandBLog struct {
	Level     string                 `json:"level"`     // 日志级别: info, warning, error
	Message   string                 `json:"message"`   // 日志内容
	Timestamp float64                `json:"timestamp"` // 日志时间戳
	Source    string                 `json:"source"`    // 日志来源，如 "stdout", "stderr"
	Extra     map[string]interface{} `json:"extra"`     // 额外字段
}

// MetricsStorage 指标存储接口
type MetricsStorage interface {
	Store(ctx context.Context, metric *StoredMetric) error
	Query(ctx context.Context, workloadUID string, metricName string) ([]*StoredMetric, error)
}

// StoredMetric 存储的指标格式
type StoredMetric struct {
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

// WandBLogProcessor WandB 日志和指标处理器
type WandBLogProcessor struct {
	metricsStorage MetricsStorage // 指标存储接口
}

// NewWandBLogProcessor 创建处理器
func NewWandBLogProcessor(
	metricsStorage MetricsStorage,
) *WandBLogProcessor {
	return &WandBLogProcessor{
		metricsStorage: metricsStorage,
	}
}

// ProcessMetrics 处理 WandB 指标数据
func (p *WandBLogProcessor) ProcessMetrics(
	ctx context.Context,
	req *WandBMetricsRequest,
) error {
	logrus.Infof("Processing WandB metrics for workload %s, %d metrics",
		req.WorkloadUID, len(req.Metrics))

	// 1. 验证请求
	if req.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}

	if len(req.Metrics) == 0 {
		logrus.Debug("No metrics to process")
		return nil
	}

	// 2. 转换为内部格式并存储
	successCount := 0
	errorCount := 0

	for _, metric := range req.Metrics {
		// 构造存储格式
		storedMetric := &StoredMetric{
			WorkloadUID: req.WorkloadUID,
			PodUID:      req.PodUID,
			Source:      "wandb",
			RunID:       req.RunID,
			Name:        metric.Name,
			Value:       metric.Value,
			Step:        metric.Step,
			Timestamp:   time.Unix(0, int64(metric.Timestamp*1e9)),
			Tags:        metric.Tags,
		}

		// 存储到时序数据库或指标存储
		if err := p.metricsStorage.Store(ctx, storedMetric); err != nil {
			logrus.Errorf("Failed to store metric %s: %v", metric.Name, err)
			errorCount++
			// 继续处理其他指标
			continue
		}
		successCount++
	}

	logrus.Infof("✓ WandB metrics processed: %d success, %d errors (workload: %s)",
		successCount, errorCount, req.WorkloadUID)

	if errorCount > 0 {
		return fmt.Errorf("failed to store %d metrics", errorCount)
	}

	return nil
}

// ProcessLogs 处理 WandB 日志数据
func (p *WandBLogProcessor) ProcessLogs(
	ctx context.Context,
	req *WandBLogsRequest,
) error {
	logrus.Infof("Processing WandB logs for workload %s, %d logs",
		req.WorkloadUID, len(req.Logs))

	// 1. 验证请求
	if req.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}

	if len(req.Logs) == 0 {
		logrus.Debug("No logs to process")
		return nil
	}

	// 2. 处理日志
	successCount := 0
	errorCount := 0

	for _, log := range req.Logs {
		// 转换时间戳
		logTime := time.Unix(0, int64(log.Timestamp*1e9))

		// 使用已有的 WorkloadLog 函数处理
		// 这样可以复用日志模式匹配、框架检测等逻辑
		if err := WorkloadLog(ctx, req.PodUID, log.Message, logTime); err != nil {
			logrus.Errorf("Failed to process log: %v", err)
			errorCount++
			// 继续处理其他日志
			continue
		}
		successCount++
	}

	logrus.Infof("✓ WandB logs processed: %d success, %d errors (workload: %s)",
		successCount, errorCount, req.WorkloadUID)

	if errorCount > 0 {
		return fmt.Errorf("failed to process %d logs", errorCount)
	}

	return nil
}

