/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"testing"

	"gotest.tools/assert"
)

func TestParseS3Path(t *testing.T) {
	tests := []struct {
		name       string
		s3Path     string
		wantBucket string
		wantKey    string
		wantErr    bool
	}{
		{
			name:       "standard s3 path",
			s3Path:     "s3://my-bucket/path/to/file.json",
			wantBucket: "my-bucket",
			wantKey:    "path/to/file.json",
			wantErr:    false,
		},
		{
			name:       "without s3 prefix",
			s3Path:     "my-bucket/path/to/file.json",
			wantBucket: "my-bucket",
			wantKey:    "path/to/file.json",
			wantErr:    false,
		},
		{
			name:       "simple path",
			s3Path:     "bucket/key",
			wantBucket: "bucket",
			wantKey:    "key",
			wantErr:    false,
		},
		{
			name:       "invalid path - no slash",
			s3Path:     "invalid",
			wantBucket: "",
			wantKey:    "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, key, err := parseS3Path(tt.s3Path)
			if tt.wantErr {
				assert.Assert(t, err != nil, "expected error but got none")
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, bucket, tt.wantBucket)
			assert.Equal(t, key, tt.wantKey)
		})
	}
}

func TestGenerateS3ReportPath(t *testing.T) {
	tests := []struct {
		name     string
		taskId   string
		expected string
	}{
		{
			name:     "standard task id",
			taskId:   "eval-task-12345",
			expected: "evaluations/eval-task-12345/summary.json",
		},
		{
			name:     "uuid task id",
			taskId:   "eval-task-abcd1234-5678-90ef",
			expected: "evaluations/eval-task-abcd1234-5678-90ef/summary.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateS3ReportPath(tt.taskId)
			assert.Equal(t, result, tt.expected)
		})
	}
}

func TestCalculateOverallScore(t *testing.T) {
	tests := []struct {
		name     string
		results  []BenchmarkResult
		expected float64
	}{
		{
			name:     "empty results",
			results:  []BenchmarkResult{},
			expected: 0,
		},
		{
			name: "single result with accuracy",
			results: []BenchmarkResult{
				{
					DatasetName: "gsm8k",
					Metrics: map[string]float64{
						"accuracy": 0.85,
					},
				},
			},
			expected: 0.85,
		},
		{
			name: "multiple results with accuracy",
			results: []BenchmarkResult{
				{
					DatasetName: "gsm8k",
					Metrics: map[string]float64{
						"accuracy": 0.80,
					},
				},
				{
					DatasetName: "math",
					Metrics: map[string]float64{
						"accuracy": 0.90,
					},
				},
			},
			expected: 0.85,
		},
		{
			name: "results with score metric",
			results: []BenchmarkResult{
				{
					DatasetName: "alpaca_eval",
					Metrics: map[string]float64{
						"winrate_score": 0.70,
					},
				},
			},
			expected: 0.70,
		},
		{
			name: "results with pass@1 metric",
			results: []BenchmarkResult{
				{
					DatasetName: "humaneval",
					Metrics: map[string]float64{
						"pass@1": 0.65,
					},
				},
			},
			expected: 0.65,
		},
		{
			name: "results without recognized metrics",
			results: []BenchmarkResult{
				{
					DatasetName: "custom",
					Metrics: map[string]float64{
						"bleu": 0.45,
					},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateOverallScore(tt.results)
			assert.Equal(t, result, tt.expected)
		})
	}
}

func TestEvalServiceType(t *testing.T) {
	// Test service type constants
	assert.Equal(t, string(EvalServiceTypeRemoteAPI), "remote_api")
	assert.Equal(t, string(EvalServiceTypeLocalWorkload), "local_workload")
}

func TestBenchmarkConfig(t *testing.T) {
	// Test BenchmarkConfig struct fields
	config := BenchmarkConfig{
		DatasetId:       "dataset-123",
		DatasetName:     "gsm8k",
		DatasetLocalDir: "/apps/datasets/gsm8k",
		Limit:           100,
	}

	assert.Equal(t, config.DatasetId, "dataset-123")
	assert.Equal(t, config.DatasetName, "gsm8k")
	assert.Equal(t, config.DatasetLocalDir, "/apps/datasets/gsm8k")
	assert.Equal(t, config.Limit, 100)
}

func TestEvaluationTaskView(t *testing.T) {
	// Test EvaluationTaskView struct initialization
	view := EvaluationTaskView{
		TaskId:         "eval-task-test",
		TaskName:       "Test Evaluation",
		ServiceId:      "model-123",
		ServiceType:    EvalServiceTypeRemoteAPI,
		ServiceName:    "gpt-4",
		Status:         "Succeeded",
		EvaluationType: "normal",
		Timeout:        7200,
		Concurrency:    32,
	}

	assert.Equal(t, view.TaskId, "eval-task-test")
	assert.Equal(t, view.TaskName, "Test Evaluation")
	assert.Equal(t, view.ServiceType, EvalServiceTypeRemoteAPI)
	assert.Equal(t, view.EvaluationType, "normal")
	assert.Equal(t, view.Timeout, 7200)
	assert.Equal(t, view.Concurrency, 32)
}

func TestJudgeConfig(t *testing.T) {
	// Test JudgeConfig struct
	config := JudgeConfig{
		ServiceId:   "model-judge",
		ServiceType: "remote_api",
	}

	assert.Equal(t, config.ServiceId, "model-judge")
	assert.Equal(t, config.ServiceType, "remote_api")
}

