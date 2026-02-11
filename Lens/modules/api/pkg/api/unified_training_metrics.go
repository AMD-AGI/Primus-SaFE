// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// ===== Training Metrics Definitions =====

// TrainingMetricDef defines a single training metric with its PromQL template and metadata.
type TrainingMetricDef struct {
	Name        string `json:"name"`        // Unique metric key
	DisplayName string `json:"display_name"` // Human-readable name for display
	Description string `json:"description"` // Metric description
	Category    string `json:"category"`    // Category grouping (gpu, pod, rdma, network)
	Unit        string `json:"unit"`        // Unit of measurement (%, W, bytes, bytes/s, pkts/s, errors)
	PromQL      string `json:"promql"`      // PromQL template with $workload_uid placeholder
	AggLevel    string `json:"agg_level"`   // Aggregation level: node or device
}

// trainingMetricsDefs holds all training metrics extracted from the Grafana dashboards.
// PromQL templates use $workload_uid as placeholder for the actual workload UID.
var trainingMetricsDefs = []TrainingMetricDef{
	// === GPU Metrics (Node Level) ===
	{
		Name:        "gpu_utilization_by_node",
		DisplayName: "GPU Utilization (Node Avg)",
		Description: "Average GPU utilization percentage per node",
		Category:    "gpu",
		Unit:        "%",
		PromQL:      `avg(workload_gpu_utilization{workload_uid="$workload_uid"}) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "gpu_memory_util_by_node",
		DisplayName: "GPU Memory Utilization (Node Avg)",
		Description: "Average GPU memory utilization percentage per node, calculated as (total - free) / total * 100",
		Category:    "gpu",
		Unit:        "%",
		PromQL:      `100 - ((avg(workload_gpu_free_vram{workload_uid="$workload_uid"}) by (primus_lens_node_name))/(avg(workload_gpu_total_vram{workload_uid="$workload_uid"}) by (primus_lens_node_name)) * 100)`,
		AggLevel:    "node",
	},
	{
		Name:        "gpu_power_by_node",
		DisplayName: "GPU Power (Node Avg)",
		Description: "Average GPU socket power consumption in watts per node",
		Category:    "gpu",
		Unit:        "W",
		PromQL:      `avg(workload_gpu_socket_power_watts{workload_uid="$workload_uid"}) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "gpu_memory_usage_by_node",
		DisplayName: "GPU Memory Usage (Node Avg)",
		Description: "Average GPU memory usage in bytes per node (total - free)",
		Category:    "gpu",
		Unit:        "bytes",
		PromQL:      `(avg(workload_gpu_total_vram{workload_uid="$workload_uid"}) by (primus_lens_node_name)) - (avg(workload_gpu_free_vram{workload_uid="$workload_uid"}) by (primus_lens_node_name))`,
		AggLevel:    "node",
	},

	// === GPU Metrics (Device Level) ===
	{
		Name:        "gpu_utilization_by_device",
		DisplayName: "GPU Utilization (Per Device)",
		Description: "GPU utilization percentage per device",
		Category:    "gpu",
		Unit:        "%",
		PromQL:      `workload_gpu_utilization{workload_uid="$workload_uid"}`,
		AggLevel:    "device",
	},
	{
		Name:        "gpu_memory_util_by_device",
		DisplayName: "GPU Memory Utilization (Per Device)",
		Description: "GPU memory utilization percentage per device",
		Category:    "gpu",
		Unit:        "%",
		PromQL:      `100 - (workload_gpu_free_vram{workload_uid="$workload_uid"}/workload_gpu_total_vram{workload_uid="$workload_uid"} * 100)`,
		AggLevel:    "device",
	},
	{
		Name:        "gpu_power_by_device",
		DisplayName: "GPU Power (Per Device)",
		Description: "GPU socket power consumption in watts per device",
		Category:    "gpu",
		Unit:        "W",
		PromQL:      `workload_gpu_socket_power_watts{workload_uid="$workload_uid"}`,
		AggLevel:    "device",
	},
	{
		Name:        "gpu_memory_usage_by_device",
		DisplayName: "GPU Memory Usage (Per Device)",
		Description: "GPU memory usage in bytes per device (total - free)",
		Category:    "gpu",
		Unit:        "bytes",
		PromQL:      `workload_gpu_total_vram{workload_uid="$workload_uid"} - workload_gpu_free_vram{workload_uid="$workload_uid"}`,
		AggLevel:    "device",
	},

	// === Pod Metrics ===
	{
		Name:        "pod_cpu_usage",
		DisplayName: "Pod CPU Usage",
		Description: "Pod CPU usage in cores (rate over 5m window)",
		Category:    "pod",
		Unit:        "cores",
		PromQL:      `sum(rate(workload_container_cpu_usage_seconds_total{workload_uid="$workload_uid"}[5m])) by (pod)`,
		AggLevel:    "node",
	},
	{
		Name:        "pod_memory_usage",
		DisplayName: "Pod Memory Usage",
		Description: "Pod working set memory usage in bytes",
		Category:    "pod",
		Unit:        "bytes",
		PromQL:      `sum by (pod) (workload_container_memory_working_set_bytes{container!="",pod!="",workload_uid="$workload_uid"})`,
		AggLevel:    "node",
	},
	{
		Name:        "pod_memory_utilization",
		DisplayName: "Pod Memory Utilization",
		Description: "Pod memory utilization as percentage of requested memory",
		Category:    "pod",
		Unit:        "%",
		PromQL:      `(sum by (pod) (workload_container_memory_working_set_bytes{container!="",pod!="",workload_uid="$workload_uid"})) / (sum by (pod) (workload_kube_pod_container_resource_requests{resource="memory",workload_uid="$workload_uid"})) * 100`,
		AggLevel:    "node",
	},
	{
		Name:        "pod_ephemeral_storage",
		DisplayName: "Pod Ephemeral Storage",
		Description: "Pod ephemeral storage usage in bytes",
		Category:    "pod",
		Unit:        "bytes",
		PromQL:      `sum by (pod) (workload_pod_ephemeral_storage_usage_bytes{workload_uid="$workload_uid"})`,
		AggLevel:    "node",
	},
	{
		Name:        "pod_write_bytes",
		DisplayName: "Pod Write Bytes/s",
		Description: "Pod filesystem write throughput in bytes per second",
		Category:    "pod",
		Unit:        "bytes/s",
		PromQL:      `sum by (pod,container) (rate(workload_container_fs_writes_bytes_total{container!="",pod!="",workload_uid="$workload_uid"}[5m]))`,
		AggLevel:    "node",
	},
	{
		Name:        "pod_read_bytes",
		DisplayName: "Pod Read Bytes/s",
		Description: "Pod filesystem read throughput in bytes per second",
		Category:    "pod",
		Unit:        "bytes/s",
		PromQL:      `sum by (pod,container) (rate(workload_container_fs_reads_bytes_total{container!="",pod!="",workload_uid="$workload_uid"}[5m]))`,
		AggLevel:    "node",
	},

	// === RDMA Metrics (Node Level) ===
	{
		Name:        "rdma_tx_bandwidth_by_node",
		DisplayName: "RDMA TX Bandwidth (Node)",
		Description: "RDMA transmit RoCE bandwidth in bytes per second per node",
		Category:    "rdma",
		Unit:        "bytes/s",
		PromQL:      `sum(rate(workload_rdma_stat_tx_roce_only_bytes{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_rx_bandwidth_by_node",
		DisplayName: "RDMA RX Bandwidth (Node)",
		Description: "RDMA receive RoCE bandwidth in bytes per second per node",
		Category:    "rdma",
		Unit:        "bytes/s",
		PromQL:      `sum(rate(workload_rdma_stat_rx_roce_only_bytes{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_tx_pkts_by_node",
		DisplayName: "RDMA TX Packets (Node)",
		Description: "RDMA transmit RoCE packet rate per node",
		Category:    "rdma",
		Unit:        "pkts/s",
		PromQL:      `sum(rate(workload_rdma_stat_tx_roce_only_pkts{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_rx_pkts_by_node",
		DisplayName: "RDMA RX Packets (Node)",
		Description: "RDMA receive RoCE packet rate per node",
		Category:    "rdma",
		Unit:        "pkts/s",
		PromQL:      `sum(rate(workload_rdma_stat_rx_roce_only_pkts{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},

	// === RDMA Metrics (Device Level) ===
	{
		Name:        "rdma_tx_bandwidth_by_device",
		DisplayName: "RDMA TX Bandwidth (Device)",
		Description: "RDMA transmit RoCE bandwidth in bytes per second per device",
		Category:    "rdma",
		Unit:        "bytes/s",
		PromQL:      `sum(rate(workload_rdma_stat_tx_roce_only_bytes{workload_uid="$workload_uid"}[1m])) by (gpu_id,primus_lens_node_name)`,
		AggLevel:    "device",
	},
	{
		Name:        "rdma_rx_bandwidth_by_device",
		DisplayName: "RDMA RX Bandwidth (Device)",
		Description: "RDMA receive RoCE bandwidth in bytes per second per device",
		Category:    "rdma",
		Unit:        "bytes/s",
		PromQL:      `sum(rate(workload_rdma_stat_rx_roce_only_bytes{workload_uid="$workload_uid"}[1m])) by (gpu_id,primus_lens_node_name)`,
		AggLevel:    "device",
	},
	{
		Name:        "rdma_tx_pkts_by_device",
		DisplayName: "RDMA TX Packets (Device)",
		Description: "RDMA transmit RoCE packet rate per device",
		Category:    "rdma",
		Unit:        "pkts/s",
		PromQL:      `sum(rate(workload_rdma_stat_tx_roce_only_pkts{workload_uid="$workload_uid"}[1m])) by (gpu_id,primus_lens_node_name)`,
		AggLevel:    "device",
	},
	{
		Name:        "rdma_rx_pkts_by_device",
		DisplayName: "RDMA RX Packets (Device)",
		Description: "RDMA receive RoCE packet rate per device",
		Category:    "rdma",
		Unit:        "pkts/s",
		PromQL:      `sum(rate(workload_rdma_stat_rx_roce_only_pkts{workload_uid="$workload_uid"}[1m])) by (gpu_id,primus_lens_node_name)`,
		AggLevel:    "device",
	},

	// === RDMA Error Metrics (Node Level) ===
	{
		Name:        "rdma_rx_roce_discards",
		DisplayName: "RDMA RX RoCE Discards",
		Description: "Increase in RDMA RX RoCE discards per node",
		Category:    "rdma_error",
		Unit:        "errors",
		PromQL:      `sum(increase(workload_rdma_stat_rx_roce_discards{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_tx_roce_discards",
		DisplayName: "RDMA TX RoCE Discards",
		Description: "Increase in RDMA TX RoCE discards per node",
		Category:    "rdma_error",
		Unit:        "errors",
		PromQL:      `sum(increase(workload_rdma_stat_tx_roce_discards{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_tx_roce_errors",
		DisplayName: "RDMA TX RoCE Errors",
		Description: "Increase in RDMA TX RoCE errors per node",
		Category:    "rdma_error",
		Unit:        "errors",
		PromQL:      `sum(increase(workload_rdma_stat_tx_roce_errors{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_rx_roce_errors",
		DisplayName: "RDMA RX RoCE Errors",
		Description: "Increase in RDMA RX RoCE errors per node",
		Category:    "rdma_error",
		Unit:        "errors",
		PromQL:      `sum(increase(workload_rdma_stat_rx_roce_errors{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_seq_err_naks_rcvd",
		DisplayName: "RDMA Seq Error NAKs Received",
		Description: "Increase in RDMA sequence error NAKs received per node",
		Category:    "rdma_error",
		Unit:        "errors",
		PromQL:      `sum(increase(workload_rdma_stat_seq_err_naks_rcvd{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_rnr_naks_rcvd",
		DisplayName: "RDMA RNR NAKs Received",
		Description: "Increase in RDMA receiver-not-ready NAKs received per node",
		Category:    "rdma_error",
		Unit:        "errors",
		PromQL:      `sum(increase(workload_rdma_stat_rnr_naks_rcvd{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_to_retransmits",
		DisplayName: "RDMA Timeout Retransmits",
		Description: "Increase in RDMA timeout retransmissions per node",
		Category:    "rdma_error",
		Unit:        "errors",
		PromQL:      `sum(increase(workload_rdma_stat_to_retransmits{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_max_retry_exceeded",
		DisplayName: "RDMA Max Retry Exceeded",
		Description: "Increase in RDMA max retry exceeded errors per node",
		Category:    "rdma_error",
		Unit:        "errors",
		PromQL:      `sum(increase(workload_rdma_stat_max_retry_exceeded{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_missing_resp",
		DisplayName: "RDMA Missing Response",
		Description: "Increase in RDMA missing responses per node",
		Category:    "rdma_error",
		Unit:        "errors",
		PromQL:      `sum(increase(workload_rdma_stat_missing_resp{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_unrecoverable_err",
		DisplayName: "RDMA Unrecoverable Errors",
		Description: "Increase in RDMA unrecoverable errors per node",
		Category:    "rdma_error",
		Unit:        "errors",
		PromQL:      `sum(increase(workload_rdma_stat_unrecoverable_err{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "rdma_local_qp_op_err",
		DisplayName: "RDMA Local QP Operation Errors",
		Description: "Increase in RDMA local QP operation errors per node",
		Category:    "rdma_error",
		Unit:        "errors",
		PromQL:      `sum(increase(workload_rdma_stat_local_qp_op_err{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},

	// === XGMI / PCIe Metrics ===
	{
		Name:        "xgmi_tx_bandwidth_by_node",
		DisplayName: "XGMI TX Bandwidth (Node)",
		Description: "XGMI link transmit bandwidth rate per node",
		Category:    "network",
		Unit:        "bytes/s",
		PromQL:      `sum(rate(workload_gpu_xgmi_link_tx{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "xgmi_rx_bandwidth_by_node",
		DisplayName: "XGMI RX Bandwidth (Node)",
		Description: "XGMI link receive bandwidth rate per node",
		Category:    "network",
		Unit:        "bytes/s",
		PromQL:      `sum(rate(gpu_xgmi_link_rx{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "pcie_bandwidth_by_node",
		DisplayName: "PCIe Bandwidth (Node)",
		Description: "PCIe bandwidth rate per node in MB/s",
		Category:    "network",
		Unit:        "MB/s",
		PromQL:      `sum(rate(workload_gpu_pcie_bandwidth_mbs{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name)`,
		AggLevel:    "node",
	},
	{
		Name:        "xgmi_tx_bandwidth_by_device",
		DisplayName: "XGMI TX Bandwidth (Device)",
		Description: "XGMI link transmit bandwidth rate per device",
		Category:    "network",
		Unit:        "bytes/s",
		PromQL:      `sum(rate(workload_gpu_xgmi_link_tx{workload_uid="$workload_uid"}[1m])) by (gpu_id,primus_lens_node_name)`,
		AggLevel:    "device",
	},
	{
		Name:        "xgmi_rx_bandwidth_by_device",
		DisplayName: "XGMI RX Bandwidth (Device)",
		Description: "XGMI link receive bandwidth rate per device",
		Category:    "network",
		Unit:        "bytes/s",
		PromQL:      `sum(rate(gpu_xgmi_link_rx{workload_uid="$workload_uid"}[1m])) by (gpu_id,primus_lens_node_name)`,
		AggLevel:    "device",
	},
	{
		Name:        "pcie_bandwidth_by_device",
		DisplayName: "PCIe Bandwidth (Device)",
		Description: "PCIe bandwidth rate per device in MB/s",
		Category:    "network",
		Unit:        "MB/s",
		PromQL:      `sum(rate(workload_gpu_pcie_bandwidth_mbs{workload_uid="$workload_uid"}[1m])) by (primus_lens_node_name,gpu_id)`,
		AggLevel:    "device",
	},
}

// trainingMetricsIndex is a lookup map by metric name for quick access.
var trainingMetricsIndex map[string]*TrainingMetricDef

func init() {
	// Build index
	trainingMetricsIndex = make(map[string]*TrainingMetricDef, len(trainingMetricsDefs))
	for i := range trainingMetricsDefs {
		trainingMetricsIndex[trainingMetricsDefs[i].Name] = &trainingMetricsDefs[i]
	}
}

// ===== API 1: Training Metrics List =====

// TrainingMetricsListRequest represents the request for listing available training metrics.
type TrainingMetricsListRequest struct {
	Category string `json:"category" query:"category" mcp:"category,description=Filter by category (gpu/pod/rdma/rdma_error/network). Leave empty for all."`
	AggLevel string `json:"agg_level" query:"agg_level" mcp:"agg_level,description=Filter by aggregation level (node/device). Leave empty for all."`
}

// TrainingMetricsListResponse represents the response for training metrics list.
type TrainingMetricsListResponse struct {
	Metrics    []TrainingMetricDef `json:"metrics"`
	TotalCount int                 `json:"total_count"`
	Categories []string            `json:"categories"`
}

// ===== API 2: Training Metrics Data Query =====

// TrainingMetricsDataRequest represents the request for querying training metrics data.
type TrainingMetricsDataRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	WorkloadUID string `json:"workload_uid" query:"workload_uid" mcp:"workload_uid,description=Workload UID,required"`
	Metrics     string `json:"metrics" query:"metrics" mcp:"metrics,description=Comma-separated metric names from the training metrics list (e.g. gpu_utilization_by_node or rdma_tx_bandwidth_by_node). Use 'all' for all metrics.,required"`
	Start       string `json:"start" query:"start" mcp:"start,description=Start timestamp (unix seconds),required"`
	End         string `json:"end" query:"end" mcp:"end,description=End timestamp (unix seconds),required"`
	Step        string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

// TrainingMetricResult holds the query result for a single metric.
type TrainingMetricResult struct {
	Name        string               `json:"name"`
	DisplayName string               `json:"display_name"`
	Category    string               `json:"category"`
	Unit        string               `json:"unit"`
	AggLevel    string               `json:"agg_level"`
	Series      []model.MetricsSeries `json:"series"`
}

// TrainingMetricsDataResponse represents the response for training metrics data query.
type TrainingMetricsDataResponse struct {
	WorkloadUID string                 `json:"workload_uid"`
	Start       int64                  `json:"start"`
	End         int64                  `json:"end"`
	Step        int                    `json:"step"`
	Results     []TrainingMetricResult `json:"results"`
	TotalCount  int                    `json:"total_count"`
}

// ===== Register Training Metrics Endpoints =====

func init() {
	// API 1: Training Metrics List
	unified.Register(&unified.EndpointDef[TrainingMetricsListRequest, TrainingMetricsListResponse]{
		Name:        "training_metrics_list",
		Description: "List all available training metrics with their descriptions and PromQL templates. Metrics are organized by category: gpu (GPU utilization/memory/power), pod (CPU/memory/storage/IO), rdma (bandwidth/packets), rdma_error (RDMA errors), network (XGMI/PCIe). Each metric includes its PromQL template that uses $workload_uid as a placeholder.",
		HTTPMethod:  "GET",
		HTTPPath:    "/training-metrics",
		MCPToolName: "lens_training_metrics_list",
		Handler:     handleTrainingMetricsList,
	})

	// API 2: Training Metrics Data Query
	unified.Register(&unified.EndpointDef[TrainingMetricsDataRequest, TrainingMetricsDataResponse]{
		Name:        "training_metrics_data",
		Description: "Query training metrics data for a workload. Select specific metrics from the training metrics list, specify a time range and step. Returns Prometheus time series data for each requested metric. Use the training_metrics_list endpoint first to discover available metrics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/training-metrics/data",
		MCPToolName: "lens_training_metrics_data",
		Handler:     handleTrainingMetricsData,
	})
}

// ===== Handler Implementations =====

// handleTrainingMetricsList returns the list of available training metrics.
func handleTrainingMetricsList(_ context.Context, req *TrainingMetricsListRequest) (*TrainingMetricsListResponse, error) {
	var filtered []TrainingMetricDef
	categorySet := make(map[string]struct{})

	for _, m := range trainingMetricsDefs {
		// Apply category filter
		if req.Category != "" && m.Category != req.Category {
			continue
		}
		// Apply aggregation level filter
		if req.AggLevel != "" && m.AggLevel != req.AggLevel {
			continue
		}
		filtered = append(filtered, m)
		categorySet[m.Category] = struct{}{}
	}

	categories := make([]string, 0, len(categorySet))
	for cat := range categorySet {
		categories = append(categories, cat)
	}

	return &TrainingMetricsListResponse{
		Metrics:    filtered,
		TotalCount: len(filtered),
		Categories: categories,
	}, nil
}

// handleTrainingMetricsData queries Prometheus for the requested training metrics.
func handleTrainingMetricsData(ctx context.Context, req *TrainingMetricsDataRequest) (*TrainingMetricsDataResponse, error) {
	// Validate required fields
	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}
	if req.Metrics == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("metrics is required")
	}
	if req.Start == "" || req.End == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("start and end timestamps are required")
	}

	// Parse timestamps
	startUnix, err := strconv.ParseInt(req.Start, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid start timestamp")
	}
	endUnix, err := strconv.ParseInt(req.End, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid end timestamp")
	}
	startTime := time.Unix(startUnix, 0)
	endTime := time.Unix(endUnix, 0)

	step := 60
	if req.Step != "" {
		step, err = strconv.Atoi(req.Step)
		if err != nil || step <= 0 {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid step value, must be positive integer")
		}
	}

	// Resolve metrics to query
	var metricsToQuery []*TrainingMetricDef
	if req.Metrics == "all" {
		for i := range trainingMetricsDefs {
			metricsToQuery = append(metricsToQuery, &trainingMetricsDefs[i])
		}
	} else {
		metricNames := strings.Split(req.Metrics, ",")
		for _, name := range metricNames {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			def, ok := trainingMetricsIndex[name]
			if !ok {
				return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).
					WithMessagef("unknown metric: %s. Use the training_metrics_list endpoint to discover available metrics.", name)
			}
			metricsToQuery = append(metricsToQuery, def)
		}
	}

	if len(metricsToQuery) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("no valid metrics specified")
	}

	// Resolve cluster and get storage client
	clients, err := getClusterClientsForWorkload(ctx, req.WorkloadUID, req.Cluster)
	if err != nil {
		return nil, err
	}
	storageClient := clients.StorageClientSet

	// Fallback to current cluster if storage not available
	if storageClient == nil {
		log.Warnf("[TrainingMetricsData] Cluster '%s' has no storage configuration, falling back to current cluster",
			clients.ClusterName)
		cm := clientsets.GetClusterManager()
		currentClients := cm.GetCurrentClusterClients()
		if currentClients == nil || currentClients.StorageClientSet == nil {
			return nil, errors.NewError().WithCode(errors.InternalError).
				WithMessage("No storage configuration available for metrics query")
		}
		storageClient = currentClients.StorageClientSet
	}

	// Query each metric
	results := make([]TrainingMetricResult, 0, len(metricsToQuery))
	for _, metricDef := range metricsToQuery {
		// Replace $workload_uid placeholder with actual UID
		query := strings.ReplaceAll(metricDef.PromQL, "$workload_uid", req.WorkloadUID)

		series, err := prom.QueryRange(ctx, storageClient, query, startTime, endTime, step, nil)
		if err != nil {
			log.Warnf("[TrainingMetricsData] Failed to query metric %s: %v", metricDef.Name, err)
			// Add empty result instead of failing the entire request
			results = append(results, TrainingMetricResult{
				Name:        metricDef.Name,
				DisplayName: metricDef.DisplayName,
				Category:    metricDef.Category,
				Unit:        metricDef.Unit,
				AggLevel:    metricDef.AggLevel,
				Series:      []model.MetricsSeries{},
			})
			continue
		}

		results = append(results, TrainingMetricResult{
			Name:        metricDef.Name,
			DisplayName: metricDef.DisplayName,
			Category:    metricDef.Category,
			Unit:        metricDef.Unit,
			AggLevel:    metricDef.AggLevel,
			Series:      series,
		})
	}

	return &TrainingMetricsDataResponse{
		WorkloadUID: req.WorkloadUID,
		Start:       startUnix,
		End:         endUnix,
		Step:        step,
		Results:     results,
		TotalCount:  len(results),
	}, nil
}

