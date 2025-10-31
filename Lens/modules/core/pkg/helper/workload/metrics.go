package workload

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/constant"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	promModel "github.com/prometheus/common/model"
)

func GetWorkloadGpuUtilMetrics(ctx context.Context, workloadUid string, start, end time.Time, step int, clientSets *clientsets.StorageClientSet) (*model.MetricsGraph, error) {
	query := fmt.Sprintf(`avg (workload_gpu_utilization{workload_uid="%s"}) by (primus_lens_node_name)`, workloadUid)
	data, err := prom.QueryRange(ctx, clientSets, query, start, end, step, map[string]struct{}{
		"__name__":                       {},
		constant.PrimusLensNodeLabelName: {},
	})
	if err != nil {
		return nil, err
	}
	return &model.MetricsGraph{
		Series: data,
		Config: model.MetricsGraphConfig{
			YAxisUnit: "%",
		},
	}, nil
}

func GetWorkloadGpuMemoryUtilMetrics(ctx context.Context, workloadUid string, start, end time.Time, step int, clientSets *clientsets.StorageClientSet) (*model.MetricsGraph, error) {
	query := fmt.Sprintf(
		`avg by (%s) (workload_gpu_used_vram{workload_uid="%s"}/workload_gpu_total_vram{workload_uid="%s"}) * 100`,
		constant.PrimusLensNodeLabelName,
		workloadUid,
		workloadUid)
	data, err := prom.QueryRange(ctx, clientSets, query, start, end, step, map[string]struct{}{
		"__name__":                       {},
		constant.PrimusLensNodeLabelName: {},
	})
	if err != nil {
		return nil, err
	}
	return &model.MetricsGraph{
		Series: data,
		Config: model.MetricsGraphConfig{
			YAxisUnit: "%",
		},
	}, nil
}

func GetWorkloadGpuPowerMetrics(ctx context.Context, workloadUid string, start, end time.Time, step int, clientSets *clientsets.StorageClientSet) (*model.MetricsGraph, error) {
	query := fmt.Sprintf(`avg by (%s) (workload_gpu_package_power{workload_uid="%s"})`,
		constant.PrimusLensNodeLabelName,
		workloadUid)
	data, err := prom.QueryRange(ctx, clientSets, query, start, end, step, map[string]struct{}{
		"__name__":                       {},
		constant.PrimusLensNodeLabelName: {},
	})
	if err != nil {
		return nil, err
	}
	return &model.MetricsGraph{
		Series: data,
		Config: model.MetricsGraphConfig{YAxisUnit: "W"},
	}, nil
}

func GetWorkloadRdmaErrorMetrics(ctx context.Context, workloadUid string, start, end time.Time, step int, clientSets *clientsets.StorageClientSet) (*model.MetricsGraph, error) {
	query := `avg (gpu_clock{clock_type="GPU_CLOCK_TYPE_SYSTEM"}) by (primus_lens_node_name)`
	data, err := prom.QueryRange(ctx, clientSets, query, start, end, step, map[string]struct{}{
		"__name__":                       {},
		constant.PrimusLensNodeLabelName: {},
	})
	if err != nil {
		return nil, err
	}
	return &model.MetricsGraph{
		Series: data,
		Config: model.MetricsGraphConfig{
			YAxisUnit: "%",
		},
	}, nil
}

func GetTFLOPSMetrics(ctx context.Context, workloadUid string, start, end time.Time, step int, clientSets *clientsets.StorageClientSet) (*model.MetricsGraph, error) {
	perfs, err := database.ListWorkloadPerformanceByWorkloadIdAndTimeRange(ctx, workloadUid, start, end)
	if err != nil {
		return nil, err
	}
	iterationSeries := model.MetricsSeries{
		Labels: map[promModel.LabelName]promModel.LabelValue{
			promModel.LabelName(constant.PrimusLensNodeLabelName): promModel.LabelValue("iteration"),
		},
		Values: []model.TimePoint{},
	}
	tflopsSeries := model.MetricsSeries{
		Labels: map[promModel.LabelName]promModel.LabelValue{
			promModel.LabelName(constant.PrimusLensNodeLabelName): promModel.LabelValue("tflops"),
		},
		Values: []model.TimePoint{},
	}
	for _, perf := range perfs {
		datas := perf.Performance
		tflops := datas["tflops"].(float64)
		iteration := perf.Iteration
		iterationSeries.Values = append(iterationSeries.Values, model.TimePoint{
			Timestamp: perf.CreatedAt.Unix(),
			Value:     float64(iteration),
		})
		tflopsSeries.Values = append(tflopsSeries.Values, model.TimePoint{
			Timestamp: perf.CreatedAt.Unix(),
			Value:     tflops,
		})
	}
	return &model.MetricsGraph{
		Serial: 0,
		Series: []model.MetricsSeries{
			tflopsSeries,
			iterationSeries,
		},
		Config: model.MetricsGraphConfig{
			YAxisUnit: " ",
		},
	}, nil
}
