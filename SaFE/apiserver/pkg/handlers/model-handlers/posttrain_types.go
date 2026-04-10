/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

type ListPosttrainRunQuery struct {
	Workspace      string `form:"workspace"`
	TrainType      string `form:"trainType"`
	Strategy       string `form:"strategy"`
	Status         string `form:"status"`
	Search         string `form:"search"`
	UserID         string `form:"userId"`
	Limit          int    `form:"limit" binding:"omitempty,min=1"`
	Offset         int    `form:"offset" binding:"omitempty,min=0"`
	SortBy         string `form:"sortBy"`
	Order          string `form:"order"`
	IncludeMetrics bool   `form:"includeMetrics"`
}

type ListPosttrainRunResponse struct {
	Total int                `json:"total"`
	Items []PosttrainRunItem `json:"items"`
}

type PosttrainRunItem struct {
	RunID            string   `json:"runId"`
	WorkloadID       string   `json:"workloadId"`
	WorkloadUID      string   `json:"workloadUid,omitempty"`
	DisplayName      string   `json:"displayName"`
	TrainType        string   `json:"trainType"`
	Strategy         string   `json:"strategy"`
	Algorithm        string   `json:"algorithm,omitempty"`
	Workspace        string   `json:"workspace"`
	Cluster          string   `json:"cluster"`
	UserID           string   `json:"userId,omitempty"`
	UserName         string   `json:"userName,omitempty"`
	BaseModelID      string   `json:"baseModelId"`
	BaseModelName    string   `json:"baseModelName"`
	DatasetID        string   `json:"datasetId"`
	DatasetName      string   `json:"datasetName,omitempty"`
	Image            string   `json:"image,omitempty"`
	NodeCount        int      `json:"nodeCount,omitempty"`
	GpuPerNode       int      `json:"gpuPerNode,omitempty"`
	Cpu              string   `json:"cpu,omitempty"`
	Memory           string   `json:"memory,omitempty"`
	SharedMemory     string   `json:"sharedMemory,omitempty"`
	EphemeralStorage string   `json:"ephemeralStorage,omitempty"`
	Priority         int      `json:"priority,omitempty"`
	Timeout          int      `json:"timeout,omitempty"`
	ExportModel      bool     `json:"exportModel"`
	OutputPath       string   `json:"outputPath,omitempty"`
	Status           string   `json:"status"`
	Message          string   `json:"message,omitempty"`
	CreatedAt        string   `json:"createdAt,omitempty"`
	StartTime        string   `json:"startTime,omitempty"`
	EndTime          string   `json:"endTime,omitempty"`
	Duration         string   `json:"duration,omitempty"`
	ModelID          string   `json:"modelId,omitempty"`
	ModelDisplayName string   `json:"modelDisplayName,omitempty"`
	ModelPhase       string   `json:"modelPhase,omitempty"`
	ModelOrigin      string   `json:"modelOrigin,omitempty"`
	ParameterSummary string   `json:"parameterSummary,omitempty"`
	AvailableMetrics []string `json:"availableMetrics,omitempty"`
	LatestLoss       *float64 `json:"latestLoss,omitempty"`
	LossMetricName   string   `json:"lossMetricName,omitempty"`
	LossDataSource   string   `json:"lossDataSource,omitempty"`
}

type PosttrainRunDetailResponse struct {
	PosttrainRunItem
	ParameterSnapshot map[string]interface{} `json:"parameterSnapshot,omitempty"`
	ResourceSnapshot  map[string]interface{} `json:"resourceSnapshot,omitempty"`
}

type GetPosttrainMetricsQuery struct {
	Metrics    string `form:"metrics"`
	DataSource string `form:"dataSource"`
	Start      string `form:"start"`
	End        string `form:"end"`
}

type PosttrainMetricPoint struct {
	MetricName string  `json:"metricName"`
	Value      float64 `json:"value"`
	Timestamp  int64   `json:"timestamp"`
	Iteration  int32   `json:"iteration"`
	DataSource string  `json:"dataSource,omitempty"`
}

type PosttrainMetricsResponse struct {
	RunID            string                 `json:"runId"`
	WorkloadUID      string                 `json:"workloadUid"`
	AvailableMetrics []string               `json:"availableMetrics,omitempty"`
	Data             []PosttrainMetricPoint `json:"data"`
	LatestLoss       *float64               `json:"latestLoss,omitempty"`
	LossMetricName   string                 `json:"lossMetricName,omitempty"`
	LossDataSource   string                 `json:"lossDataSource,omitempty"`
}
