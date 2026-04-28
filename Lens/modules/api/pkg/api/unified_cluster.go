// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for cluster operations.
// All data-plane queries are delegated to the Robust API.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/robust"
)

// ===== Cluster Overview =====

type ClusterOverviewRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional - uses default if not specified)"`
}

type ClusterOverviewResponse = model.GpuClusterOverview

// ===== Cluster Consumers =====

type ClusterConsumersRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number for pagination (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Number of items per page (default 10)"`
}

type ClusterConsumersResponse struct {
	Data  []model.TopLevelGpuResource `json:"data"`
	Total int                         `json:"total"`
}

// ===== GPU Heatmap =====

type ClusterGPUHeatmapRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

type ClusterGPUHeatmapResponse struct {
	Power       model.Heatmap `json:"power"`
	Temperature model.Heatmap `json:"temperature"`
	Utilization model.Heatmap `json:"utilization"`
}

// ===== Register Cluster Endpoints =====

func init() {
	unified.Register(&unified.EndpointDef[ClusterOverviewRequest, ClusterOverviewResponse]{
		Name:        "cluster_overview",
		Description: "Get comprehensive GPU cluster overview including node counts, health status, allocation rate, utilization, storage and RDMA statistics. Returns total/healthy/faulty nodes, idle/busy breakdown, and resource utilization metrics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/clusters/overview",
		MCPToolName: "lens_cluster_overview",
		Handler:     handleClusterOverview,
	})

	unified.Register(&unified.EndpointDef[ClusterConsumersRequest, ClusterConsumersResponse]{
		Name:        "cluster_consumers",
		Description: "List GPU resource consumers (workloads) in the cluster with their GPU allocation and utilization. Shows which workloads are using GPU resources and their current utilization percentage.",
		HTTPMethod:  "GET",
		HTTPPath:    "/clusters/consumers",
		MCPToolName: "lens_cluster_consumers",
		Handler:     handleClusterConsumers,
	})

	unified.Register(&unified.EndpointDef[ClusterGPUHeatmapRequest, ClusterGPUHeatmapResponse]{
		Name:        "cluster_gpu_heatmap",
		Description: "Get GPU heatmap data showing power, temperature and utilization for top K GPUs. Useful for visualizing cluster-wide GPU health.",
		HTTPMethod:  "GET",
		HTTPPath:    "/clusters/gpuHeatmap",
		MCPToolName: "lens_gpu_heatmap",
		Handler:     handleClusterGPUHeatmap,
	})
}

// ===== Handler Implementations =====

func handleClusterOverview(ctx context.Context, req *ClusterOverviewRequest) (*ClusterOverviewResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	resp, err := rc.GetClusterOverview(ctx)
	if err != nil {
		return nil, fmt.Errorf("robust cluster overview: %w", err)
	}

	return &model.GpuClusterOverview{
		TotalNodes:     resp.TotalNodes,
		HealthyNodes:   resp.HealthyNodes,
		FaultyNodes:    resp.FaultedNodes,
		AllocationRate: resp.AllocationRate,
		Utilization:    resp.AvgUtilization,
	}, nil
}

func handleClusterConsumers(ctx context.Context, req *ClusterConsumersRequest) (*ClusterConsumersResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	raw, err := rc.GetRaw(ctx, "/workloads/ranking", map[string][]string{
		"limit": {strconv.Itoa(pageNum * pageSize)},
	})
	if err != nil {
		return nil, fmt.Errorf("robust workload ranking: %w", err)
	}

	var rankResp struct {
		Ranking []struct {
			WorkloadID     string  `json:"workload_id"`
			AvgUtilization float64 `json:"avg_utilization"`
			P90Utilization float64 `json:"p90_utilization"`
			GPUHours       float64 `json:"gpu_hours"`
		} `json:"ranking"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(raw, &rankResp); err != nil {
		return nil, fmt.Errorf("decode ranking: %w", err)
	}

	start := (pageNum - 1) * pageSize
	end := start + pageSize
	if start > len(rankResp.Ranking) {
		start = len(rankResp.Ranking)
	}
	if end > len(rankResp.Ranking) {
		end = len(rankResp.Ranking)
	}
	page := rankResp.Ranking[start:end]

	data := make([]model.TopLevelGpuResource, 0, len(page))
	for _, r := range page {
		data = append(data, model.TopLevelGpuResource{
			Uid: r.WorkloadID,
			Stat: model.GpuStat{
				GpuUtilization: r.AvgUtilization,
			},
		})
	}

	return &ClusterConsumersResponse{
		Data:  data,
		Total: rankResp.Count,
	}, nil
}

func handleClusterGPUHeatmap(ctx context.Context, req *ClusterGPUHeatmapRequest) (*ClusterGPUHeatmapResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	heatmap, err := rc.GetClusterGPUHeatmap(ctx)
	if err != nil {
		return nil, fmt.Errorf("robust gpu heatmap: %w", err)
	}

	return convertRobustHeatmap(heatmap), nil
}

func convertRobustHeatmap(src *robust.ClusterGPUHeatmapResp) *ClusterGPUHeatmapResponse {
	var utilData, powerData, tempData []model.ClusterOverviewHeatmapItem
	for _, node := range src.Nodes {
		for _, g := range node.GPUs {
			utilData = append(utilData, model.ClusterOverviewHeatmapItem{
				NodeName: g.NodeName,
				GpuId:    gpuIDToInt(g.GpuID),
				Value:    g.Utilization,
			})
			powerData = append(powerData, model.ClusterOverviewHeatmapItem{
				NodeName: g.NodeName,
				GpuId:    gpuIDToInt(g.GpuID),
				Value:    g.Power,
			})
		}
	}
	return &ClusterGPUHeatmapResponse{
		Power: model.Heatmap{
			Serial: 2, Unit: "W", YAxisMax: 850, YAxisMin: 0,
			Data: powerData,
		},
		Temperature: model.Heatmap{
			Serial: 3, Unit: "℃", YAxisMax: 110, YAxisMin: 20,
			Data: tempData,
		},
		Utilization: model.Heatmap{
			Serial: 1, Unit: "%", YAxisMax: 100, YAxisMin: 0,
			Data: utilData,
		},
	}
}

func gpuIDToInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// getRobustClient resolves the Robust API client for the given cluster name.
func getRobustClient(cluster string) (*robust.Client, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(cluster)
	if err != nil {
		return nil, err
	}
	if clients.RobustClient == nil {
		return nil, fmt.Errorf("no Robust client configured for cluster %q", clients.ClusterName)
	}
	return clients.RobustClient, nil
}
