/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestIsEvaluationWorkload(t *testing.T) {
	tests := []struct {
		name     string
		workload *v1.Workload
		expected bool
	}{
		{
			name: "is evaluation workload",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						v1.OpsJobIdLabel:   "ops-job-123",
						v1.OpsJobTypeLabel: string(v1.OpsJobEvaluationType),
					},
				},
			},
			expected: true,
		},
		{
			name: "not evaluation workload - wrong type",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						v1.OpsJobIdLabel:   "ops-job-123",
						v1.OpsJobTypeLabel: "reboot",
					},
				},
			},
			expected: false,
		},
		{
			name: "not evaluation workload - no ops job id",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						v1.OpsJobTypeLabel: string(v1.OpsJobEvaluationType),
					},
				},
			},
			expected: false,
		},
		{
			name: "not evaluation workload - empty labels",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			expected: false,
		},
		{
			name: "not evaluation workload - nil labels",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEvaluationWorkload(tt.workload)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetParamValue(t *testing.T) {
	tests := []struct {
		name     string
		param    *v1.Parameter
		expected string
	}{
		{
			name:     "nil parameter",
			param:    nil,
			expected: "",
		},
		{
			name: "non-nil parameter with value",
			param: &v1.Parameter{
				Name:  "test",
				Value: "test_value",
			},
			expected: "test_value",
		},
		{
			name: "non-nil parameter with empty value",
			param: &v1.Parameter{
				Name:  "test",
				Value: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getParamValue(tt.param)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractParentDir(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "standard path",
			path:     "/wekafs/datasets/math_500",
			expected: "/wekafs/datasets",
		},
		{
			name:     "path with trailing slash",
			path:     "/wekafs/datasets/math_500/",
			expected: "/wekafs/datasets",
		},
		{
			name:     "deep nested path",
			path:     "/a/b/c/d/e",
			expected: "/a/b/c/d",
		},
		{
			name:     "root level path",
			path:     "/root",
			expected: "",
		},
		{
			name:     "single level",
			path:     "folder",
			expected: "folder",
		},
		{
			name:     "two level path",
			path:     "/folder/subfolder",
			expected: "/folder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractParentDir(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBenchmarkConfig(t *testing.T) {
	// Test BenchmarkConfig struct fields
	config := BenchmarkConfig{
		DatasetId:       "dataset-123",
		DatasetName:     "gsm8k",
		DatasetLocalDir: "/wekafs/datasets/gsm8k",
		EvalType:        "general_qa",
		Limit:           100,
	}

	assert.Equal(t, "dataset-123", config.DatasetId)
	assert.Equal(t, "gsm8k", config.DatasetName)
	assert.Equal(t, "/wekafs/datasets/gsm8k", config.DatasetLocalDir)
	assert.Equal(t, "general_qa", config.EvalType)
	assert.Equal(t, 100, config.Limit)
}

func TestDefaultEvalResources(t *testing.T) {
	// Test default resource constants
	assert.Equal(t, "16", DefaultEvalCPU)
	assert.Equal(t, "96Gi", DefaultEvalMemory)
}

func TestBuildSingleEvalCommand(t *testing.T) {
	r := &EvaluationJobReconciler{}

	tests := []struct {
		name            string
		modelEndpoint   string
		modelName       string
		modelApiKey     string
		datasetName     string
		datasetDir      string
		limit           int
		outputDir       string
		judgeModel      string
		judgeEndpoint   string
		judgeApiKey     string
		timeoutSecond   int
		concurrency     int
		expectedContain []string
	}{
		{
			name:            "basic evaluation command",
			modelEndpoint:   "http://localhost:8000/v1",
			modelName:       "llama-3-8b",
			modelApiKey:     "",
			datasetName:     "gsm8k",
			datasetDir:      "/wekafs/datasets/gsm8k",
			limit:           0,
			outputDir:       "/outputs/task-123",
			judgeModel:      "",
			judgeEndpoint:   "",
			judgeApiKey:     "",
			timeoutSecond:   7200,
			concurrency:     32,
			expectedContain: []string{"timeout", "7200", "evalscope", "eval", "--model", "llama-3-8b", "--api-url", "http://localhost:8000/v1", "--datasets", "gsm8k", "--dataset-dir", "/wekafs/datasets"},
		},
		{
			name:            "evaluation with limit",
			modelEndpoint:   "http://localhost:8000/v1",
			modelName:       "llama-3-8b",
			modelApiKey:     "",
			datasetName:     "math",
			datasetDir:      "/wekafs/datasets/math",
			limit:           100,
			outputDir:       "/outputs/task-456",
			judgeModel:      "",
			judgeEndpoint:   "",
			judgeApiKey:     "",
			timeoutSecond:   3600,
			concurrency:     16,
			expectedContain: []string{"--limit", "100", "--eval-batch-size", "16"},
		},
		{
			name:            "evaluation with api key",
			modelEndpoint:   "https://api.openai.com/v1",
			modelName:       "gpt-4",
			modelApiKey:     "sk-xxx",
			datasetName:     "humaneval",
			datasetDir:      "/wekafs/datasets/humaneval",
			limit:           0,
			outputDir:       "/outputs/task-789",
			judgeModel:      "",
			judgeEndpoint:   "",
			judgeApiKey:     "",
			timeoutSecond:   7200,
			concurrency:     32,
			expectedContain: []string{"--api-key", "sk-xxx"},
		},
		{
			name:            "evaluation with judge model",
			modelEndpoint:   "http://localhost:8000/v1",
			modelName:       "llama-3-8b",
			modelApiKey:     "",
			datasetName:     "alpaca_eval",
			datasetDir:      "/wekafs/datasets/alpaca_eval",
			limit:           0,
			outputDir:       "/outputs/task-judge",
			judgeModel:      "gpt-4",
			judgeEndpoint:   "https://api.openai.com/v1",
			judgeApiKey:     "sk-judge-key",
			timeoutSecond:   7200,
			concurrency:     32,
			expectedContain: []string{"--judge-model-args", "gpt-4", "--judge-worker-num", "32"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := r.buildSingleEvalCommand(
				tt.modelEndpoint,
				tt.modelName,
				tt.modelApiKey,
				tt.datasetName,
				tt.datasetDir,
				tt.limit,
				tt.outputDir,
				tt.judgeModel,
				tt.judgeEndpoint,
				tt.judgeApiKey,
				tt.timeoutSecond,
				tt.concurrency,
			)

			// Check that all expected strings are present in the result
			argsStr := ""
			for _, arg := range args {
				argsStr += arg + " "
			}

			for _, expected := range tt.expectedContain {
				assert.Contains(t, argsStr, expected)
			}
		})
	}
}

func TestEvaluationJobReconcilerFilter(t *testing.T) {
	r := &EvaluationJobReconciler{}

	tests := []struct {
		name     string
		job      *v1.OpsJob
		expected bool
	}{
		{
			name: "filter non-evaluation job",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Type: v1.OpsJobRebootType,
				},
			},
			expected: true,
		},
		{
			name: "do not filter evaluation job",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Type: v1.OpsJobEvaluationType,
				},
			},
			expected: false,
		},
		{
			name: "filter preflight job",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Type: v1.OpsJobPreflightType,
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.filter(context.TODO(), tt.job)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEvaluationJobReconcilerObserve(t *testing.T) {
	r := &EvaluationJobReconciler{}
	now := metav1.NewTime(time.Now())

	tests := []struct {
		name        string
		job         *v1.OpsJob
		expectedOk  bool
		expectedErr error
	}{
		{
			name: "job ended - succeeded with FinishedAt",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Phase:      v1.OpsJobSucceeded,
					FinishedAt: &now,
				},
			},
			expectedOk:  true,
			expectedErr: nil,
		},
		{
			name: "job ended - failed with FinishedAt",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Phase:      v1.OpsJobFailed,
					FinishedAt: &now,
				},
			},
			expectedOk:  true,
			expectedErr: nil,
		},
		{
			name: "job not ended - pending",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Phase: v1.OpsJobPending,
				},
			},
			expectedOk:  false,
			expectedErr: nil,
		},
		{
			name: "job not ended - running",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Phase: v1.OpsJobRunning,
				},
			},
			expectedOk:  false,
			expectedErr: nil,
		},
		{
			name: "job ended - with DeletionTimestamp",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &now,
				},
				Status: v1.OpsJobStatus{
					Phase: v1.OpsJobRunning,
				},
			},
			expectedOk:  true,
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := r.observe(context.TODO(), tt.job)
			assert.Equal(t, tt.expectedOk, ok)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}

