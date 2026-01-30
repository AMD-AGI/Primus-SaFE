/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"testing"

	"gotest.tools/assert"
)

func TestIsValidCustomEvalType(t *testing.T) {
	tests := []struct {
		name     string
		evalType string
		expected bool
	}{
		{
			name:     "valid general_qa",
			evalType: "general_qa",
			expected: true,
		},
		{
			name:     "valid general_mcq",
			evalType: "general_mcq",
			expected: true,
		},
		{
			name:     "invalid type",
			evalType: "invalid_type",
			expected: false,
		},
		{
			name:     "empty type",
			evalType: "",
			expected: false,
		},
		{
			name:     "case sensitive - uppercase",
			evalType: "GENERAL_QA",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidCustomEvalType(tt.evalType)
			assert.Equal(t, result, tt.expected)
		})
	}
}

func TestCustomEvalTypeConstants(t *testing.T) {
	// Test CustomEvalType constants
	assert.Equal(t, string(GeneralQA), "general_qa")
	assert.Equal(t, string(GeneralMCQ), "general_mcq")
}

func TestValidCustomEvalTypes(t *testing.T) {
	// Test that ValidCustomEvalTypes contains expected values
	assert.Equal(t, len(ValidCustomEvalTypes), 2)
	assert.Equal(t, ValidCustomEvalTypes[0], GeneralQA)
	assert.Equal(t, ValidCustomEvalTypes[1], GeneralMCQ)
}

func TestEvalServiceTypeConstants(t *testing.T) {
	// Test EvalServiceType constants
	assert.Equal(t, string(EvalServiceTypeRemoteAPI), "remote_api")
	assert.Equal(t, string(EvalServiceTypeLocalWorkload), "local_workload")
}

func TestAvailableEvalService(t *testing.T) {
	// Test AvailableEvalService struct
	service := AvailableEvalService{
		ServiceId:   "model-123",
		ServiceType: EvalServiceTypeRemoteAPI,
		DisplayName: "GPT-4",
		ModelName:   "gpt-4-turbo",
		Status:      "Ready",
		Workspace:   "default",
		Endpoint:    "https://api.openai.com/v1",
	}

	assert.Equal(t, service.ServiceId, "model-123")
	assert.Equal(t, service.ServiceType, EvalServiceTypeRemoteAPI)
	assert.Equal(t, service.DisplayName, "GPT-4")
	assert.Equal(t, service.ModelName, "gpt-4-turbo")
	assert.Equal(t, service.Status, "Ready")
	assert.Equal(t, service.Workspace, "default")
	assert.Equal(t, service.Endpoint, "https://api.openai.com/v1")
}

func TestListAvailableServicesResponse(t *testing.T) {
	// Test ListAvailableServicesResponse struct
	response := ListAvailableServicesResponse{
		Items: []AvailableEvalService{
			{
				ServiceId:   "model-1",
				ServiceType: EvalServiceTypeRemoteAPI,
				DisplayName: "Model 1",
			},
			{
				ServiceId:   "workload-1",
				ServiceType: EvalServiceTypeLocalWorkload,
				DisplayName: "Workload 1",
			},
		},
	}

	assert.Equal(t, len(response.Items), 2)
	assert.Equal(t, response.Items[0].ServiceId, "model-1")
	assert.Equal(t, response.Items[1].ServiceType, EvalServiceTypeLocalWorkload)
}

func TestListEvaluationTasksRequest(t *testing.T) {
	// Test ListEvaluationTasksRequest struct with defaults
	req := ListEvaluationTasksRequest{
		Workspace: "production",
		Status:    "Running",
		ServiceId: "model-123",
		Limit:     100,
		Offset:    0,
	}

	assert.Equal(t, req.Workspace, "production")
	assert.Equal(t, req.Status, "Running")
	assert.Equal(t, req.ServiceId, "model-123")
	assert.Equal(t, req.Limit, 100)
	assert.Equal(t, req.Offset, 0)
}

func TestEvaluationReportResponse(t *testing.T) {
	// Test EvaluationReportResponse struct
	response := EvaluationReportResponse{
		TaskId:      "eval-task-123",
		TaskName:    "Test Evaluation",
		ServiceName: "gpt-4",
		Status:      "Succeeded",
		Results: map[string]interface{}{
			"accuracy": 0.95,
			"f1_score": 0.92,
		},
		Duration: "1h30m",
	}

	assert.Equal(t, response.TaskId, "eval-task-123")
	assert.Equal(t, response.TaskName, "Test Evaluation")
	assert.Equal(t, response.ServiceName, "gpt-4")
	assert.Equal(t, response.Status, "Succeeded")
	assert.Equal(t, response.Duration, "1h30m")
	assert.Equal(t, response.Results["accuracy"], 0.95)
}

func TestBenchmarkResult(t *testing.T) {
	// Test BenchmarkResult struct
	result := BenchmarkResult{
		BenchmarkID:   "benchmark-123",
		BenchmarkName: "GSM8K",
		Metrics: map[string]float64{
			"accuracy":   0.85,
			"completion": 0.92,
		},
		Details: map[string]interface{}{
			"total_questions":   1000,
			"correct_answers":   850,
			"processing_time_s": 3600,
		},
	}

	assert.Equal(t, result.BenchmarkID, "benchmark-123")
	assert.Equal(t, result.BenchmarkName, "GSM8K")
	assert.Equal(t, result.Metrics["accuracy"], 0.85)
	assert.Equal(t, result.Metrics["completion"], 0.92)
	assert.Equal(t, result.Details["total_questions"], 1000)
}

func TestEvaluationSummary(t *testing.T) {
	// Test EvaluationSummary struct
	summary := EvaluationSummary{
		TotalBenchmarks: 5,
		CompletedCount:  4,
		FailedCount:     1,
		OverallScore:    0.88,
		BenchmarkResults: []BenchmarkResult{
			{
				BenchmarkID:   "b1",
				BenchmarkName: "GSM8K",
				Metrics:       map[string]float64{"accuracy": 0.85},
			},
		},
		ModelName:         "llama-3-70b",
		EvaluationVersion: "1.0.0",
	}

	assert.Equal(t, summary.TotalBenchmarks, 5)
	assert.Equal(t, summary.CompletedCount, 4)
	assert.Equal(t, summary.FailedCount, 1)
	assert.Equal(t, summary.OverallScore, 0.88)
	assert.Equal(t, len(summary.BenchmarkResults), 1)
	assert.Equal(t, summary.ModelName, "llama-3-70b")
	assert.Equal(t, summary.EvaluationVersion, "1.0.0")
}

func TestBenchmarkConfigWithPointer(t *testing.T) {
	// Test BenchmarkConfig struct with Limit pointer
	limit := 50
	config := BenchmarkConfig{
		DatasetId:       "ds-123",
		DatasetName:     "math_500",
		DatasetLocalDir: "/apps/datasets/math_500",
		EvalType:        "general_qa",
		Limit:           &limit,
	}

	assert.Equal(t, config.DatasetId, "ds-123")
	assert.Equal(t, config.DatasetName, "math_500")
	assert.Equal(t, config.DatasetLocalDir, "/apps/datasets/math_500")
	assert.Equal(t, config.EvalType, "general_qa")
	assert.Assert(t, config.Limit != nil)
	assert.Equal(t, *config.Limit, 50)
}

func TestBenchmarkConfigNilLimit(t *testing.T) {
	// Test BenchmarkConfig with nil Limit
	config := BenchmarkConfig{
		DatasetId:   "ds-456",
		DatasetName: "humaneval",
	}

	assert.Equal(t, config.DatasetId, "ds-456")
	assert.Assert(t, config.Limit == nil)
}

func TestJudgeConfigStruct(t *testing.T) {
	// Test JudgeConfig struct initialization and values
	config := JudgeConfig{
		ServiceId:   "model-judge-001",
		ServiceType: EvalServiceTypeRemoteAPI,
	}

	assert.Equal(t, config.ServiceId, "model-judge-001")
	assert.Equal(t, config.ServiceType, EvalServiceTypeRemoteAPI)

	// Test with local workload
	configLocal := JudgeConfig{
		ServiceId:   "workload-judge-001",
		ServiceType: EvalServiceTypeLocalWorkload,
	}

	assert.Equal(t, configLocal.ServiceId, "workload-judge-001")
	assert.Equal(t, configLocal.ServiceType, EvalServiceTypeLocalWorkload)
}

