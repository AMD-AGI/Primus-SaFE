/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"time"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// ==================== Evaluation Types ====================

// CustomEvalType represents the evaluation type for custom datasets
type CustomEvalType string

const (
	GeneralQA  CustomEvalType = "general_qa"
	GeneralMCQ CustomEvalType = "general_mcq"
)

// ValidCustomEvalTypes contains all valid custom evaluation types
var ValidCustomEvalTypes = []CustomEvalType{GeneralQA, GeneralMCQ}

// IsValidCustomEvalType checks if an eval type is valid for custom datasets
func IsValidCustomEvalType(evalType string) bool {
	for _, t := range ValidCustomEvalTypes {
		if string(t) == evalType {
			return true
		}
	}
	return false
}

// ==================== API Request/Response Types ====================

// EvalServiceType represents the type of model service
type EvalServiceType string

const (
	EvalServiceTypeRemoteAPI     EvalServiceType = "remote_api"
	EvalServiceTypeLocalWorkload EvalServiceType = "local_workload"
)

// BenchmarkConfig represents a benchmark/dataset configuration in the request
// All benchmarks are now stored in the dataset table (including system benchmarks)
type BenchmarkConfig struct {
	DatasetId       string `json:"datasetId" binding:"required"` // Dataset ID from dataset table
	DatasetName     string `json:"datasetName,omitempty"`        // Dataset displayName, used as evalscope benchmark name
	DatasetLocalDir string `json:"datasetLocalDir,omitempty"`    // Full local path to dataset, e.g. /apps/datasets/math_500
	EvalType        string `json:"evalType,omitempty"`           // Optional: "general_qa" or "general_mcq" for custom datasets
	Limit           *int   `json:"limit,omitempty"`              // Optional sample limit
}

// EvalParams represents evaluation parameters
type EvalParams struct {
	FewShot     int     `json:"fewShot,omitempty"`     // Number of few-shot examples (0-5)
	Temperature float64 `json:"temperature,omitempty"` // Generation temperature (recommended: 0)
	MaxTokens   int     `json:"maxTokens,omitempty"`   // Maximum generation length
}

// CreateEvaluationTaskRequest represents the request to create an evaluation task
type CreateEvaluationTaskRequest struct {
	Name          string            `json:"name" binding:"required"`
	Description   string            `json:"description,omitempty"`
	ServiceId     string            `json:"serviceId" binding:"required"`
	ServiceType   EvalServiceType   `json:"serviceType" binding:"required"`
	Benchmarks    []BenchmarkConfig `json:"benchmarks" binding:"required,min=1"`
	EvalParams    *EvalParams       `json:"evalParams,omitempty"`
	WorkspaceId   string            `json:"workspaceId,omitempty"`
	TimeoutSecond int               `json:"timeoutSecond,omitempty"` // Default: 7200 (2 hours)
}

// CreateEvaluationTaskResponse represents the response after creating an evaluation task
type CreateEvaluationTaskResponse struct {
	TaskId   string `json:"taskId"`
	OpsJobId string `json:"opsJobId,omitempty"`
}

// AvailableEvalService represents a model/service available for evaluation
type AvailableEvalService struct {
	ServiceId   string          `json:"serviceId"`
	ServiceType EvalServiceType `json:"serviceType"`
	DisplayName string          `json:"displayName"`
	ModelName   string          `json:"modelName,omitempty"`
	Status      string          `json:"status"`
	Workspace   string          `json:"workspace,omitempty"`
	Endpoint    string          `json:"endpoint,omitempty"`
}

// ListAvailableServicesResponse represents the response for listing available services
type ListAvailableServicesResponse struct {
	Items []AvailableEvalService `json:"items"`
}

// EvaluationTaskView represents the view of an evaluation task
type EvaluationTaskView struct {
	TaskId        string                        `json:"taskId"`
	TaskName      string                        `json:"taskName"`
	Description   string                        `json:"description,omitempty"`
	ServiceId     string                        `json:"serviceId"`
	ServiceType   EvalServiceType               `json:"serviceType"`
	ServiceName   string                        `json:"serviceName,omitempty"`
	Benchmarks    []BenchmarkConfig             `json:"benchmarks"`
	EvalParams    *EvalParams                   `json:"evalParams,omitempty"`
	OpsJobId      string                        `json:"opsJobId,omitempty"`
	Status        dbclient.EvaluationTaskStatus `json:"status"`
	Progress      int                           `json:"progress"`
	ResultSummary map[string]interface{}        `json:"resultSummary,omitempty"`
	ReportS3Path  string                        `json:"reportS3Path,omitempty"`
	Workspace     string                        `json:"workspace,omitempty"`
	UserId        string                        `json:"userId"`
	UserName      string                        `json:"userName,omitempty"`
	CreationTime  *time.Time                    `json:"creationTime,omitempty"`
	StartTime     *time.Time                    `json:"startTime,omitempty"`
	EndTime       *time.Time                    `json:"endTime,omitempty"`
}

// ListEvaluationTasksRequest represents query parameters for listing tasks
type ListEvaluationTasksRequest struct {
	Workspace string `form:"workspace"`
	Status    string `form:"status"`
	ServiceId string `form:"serviceId"`
	Limit     int    `form:"limit,default=50"`
	Offset    int    `form:"offset,default=0"`
}

// ListEvaluationTasksResponse represents the response for listing evaluation tasks
type ListEvaluationTasksResponse struct {
	Items      []EvaluationTaskView `json:"items"`
	TotalCount int                  `json:"totalCount"`
}

// EvaluationReportResponse represents the evaluation report
type EvaluationReportResponse struct {
	TaskId       string                 `json:"taskId"`
	TaskName     string                 `json:"taskName"`
	ServiceName  string                 `json:"serviceName"`
	Status       string                 `json:"status"`
	Results      map[string]interface{} `json:"results,omitempty"`
	ReportS3Path string                 `json:"reportS3Path,omitempty"`
	StartTime    *time.Time             `json:"startTime,omitempty"`
	EndTime      *time.Time             `json:"endTime,omitempty"`
	Duration     string                 `json:"duration,omitempty"`
}

// ==================== Report Types ====================

// BenchmarkResult represents the result of a single benchmark evaluation
type BenchmarkResult struct {
	BenchmarkID   string                 `json:"benchmarkId"`
	BenchmarkName string                 `json:"benchmarkName"`
	Metrics       map[string]float64     `json:"metrics"`
	Details       map[string]interface{} `json:"details,omitempty"`
}

// EvaluationSummary represents a summary of the entire evaluation
type EvaluationSummary struct {
	TotalBenchmarks   int               `json:"totalBenchmarks"`
	CompletedCount    int               `json:"completedCount"`
	FailedCount       int               `json:"failedCount"`
	OverallScore      float64           `json:"overallScore,omitempty"`
	BenchmarkResults  []BenchmarkResult `json:"benchmarkResults"`
	ModelName         string            `json:"modelName"`
	EvaluationVersion string            `json:"evaluationVersion,omitempty"`
}
