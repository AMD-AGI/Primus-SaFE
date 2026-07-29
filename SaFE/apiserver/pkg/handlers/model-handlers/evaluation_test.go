/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	testifyassert "github.com/stretchr/testify/assert"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

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
			taskId:   "eval-job-12345",
			expected: "evaluations/eval-job-12345/summary.json",
		},
		{
			name:     "uuid task id",
			taskId:   "eval-job-abcd1234-5678-90ef",
			expected: "evaluations/eval-job-abcd1234-5678-90ef/summary.json",
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
					BenchmarkName: "gsm8k",
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
					BenchmarkName: "gsm8k",
					Metrics: map[string]float64{
						"accuracy": 0.80,
					},
				},
				{
					BenchmarkName: "math",
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
					BenchmarkName: "alpaca_eval",
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
					BenchmarkName: "humaneval",
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
					BenchmarkName: "custom",
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
			// Use tolerance for float comparison to handle precision issues
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			assert.Assert(t, diff < 0.0001, "expected %v but got %v", tt.expected, result)
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
	limit := 100
	config := BenchmarkConfig{
		DatasetId:       "dataset-123",
		DatasetName:     "gsm8k",
		DatasetLocalDir: "/apps/datasets/gsm8k",
		Limit:           &limit,
	}

	assert.Equal(t, config.DatasetId, "dataset-123")
	assert.Equal(t, config.DatasetName, "gsm8k")
	assert.Equal(t, config.DatasetLocalDir, "/apps/datasets/gsm8k")
	assert.Equal(t, *config.Limit, 100)
}

func TestEvaluationTaskView(t *testing.T) {
	// Test EvaluationTaskView struct initialization
	view := EvaluationTaskView{
		TaskId:         "eval-job-test",
		TaskName:       "Test Evaluation",
		ServiceId:      "model-123",
		ServiceType:    EvalServiceTypeRemoteAPI,
		ServiceName:    "gpt-4",
		Status:         "Succeeded",
		EvaluationType: "normal",
		Timeout:        7200,
		Concurrency:    32,
	}

	assert.Equal(t, view.TaskId, "eval-job-test")
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
		ServiceType: EvalServiceTypeRemoteAPI,
	}

	assert.Equal(t, config.ServiceId, "model-judge")
	assert.Equal(t, config.ServiceType, EvalServiceTypeRemoteAPI)
}

// --- merged from evaluation_crud_test.go ---

// evalCtx builds a gin context (with recorder) carrying the given params.
func evalCtx(t *testing.T, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = params
	return c, w
}

func TestGetEvaluationTaskHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{TaskId: "t1", TaskName: "task", Status: dbclient.EvaluationTaskStatusRunning}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.GetEvaluationTask(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetEvaluationTaskHandlerEmptyID(t *testing.T) {
	h := &Handler{}
	c, w := evalCtx(t, nil)
	h.GetEvaluationTask(c)
	testifyassert.NotEqual(t, http.StatusOK, w.Code)
}

func TestListEvaluationTasksHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().CountEvaluationTasks(gomock.Any(), gomock.Any()).Return(1, nil)
	m.EXPECT().SelectEvaluationTasks(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.EvaluationTask{{TaskId: "t1", TaskName: "task"}}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, nil)
	h.ListEvaluationTasks(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteEvaluationTaskHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	// OpsJobId invalid -> no k8s delete.
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{TaskId: "t1"}, nil)
	m.EXPECT().SetEvaluationTaskDeleted(gomock.Any(), "t1").Return(nil)

	h := &Handler{dbClient: m, k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.DeleteEvaluationTask(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestStopEvaluationTaskHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{TaskId: "t1", Status: dbclient.EvaluationTaskStatusRunning}, nil)
	m.EXPECT().UpdateEvaluationTaskStatus(gomock.Any(), "t1", dbclient.EvaluationTaskStatusCancelled).Return(nil)

	h := &Handler{dbClient: m, k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.StopEvaluationTask(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestStopEvaluationTaskHandlerNotRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{TaskId: "t1", Status: dbclient.EvaluationTaskStatusSucceeded}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.StopEvaluationTask(c)
	testifyassert.NotEqual(t, http.StatusOK, w.Code)
}

func TestGetEvaluationReportHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	// No report S3 path / nil s3 client -> returns base response.
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{TaskId: "t1", TaskName: "task"}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.GetEvaluationReport(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestListAvailableEvalServicesHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListModels(gomock.Any(), "remote_api", "", false).
		Return([]*dbclient.Model{{ID: "m1", DisplayName: "M", ModelName: "gpt"}}, nil)
	m.EXPECT().SelectWorkloads(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.Workload{}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, nil)
	h.ListAvailableEvalServices(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- merged from evaluation_more_test.go ---

var assertErr = errors.New("db error")

// TestListAvailableEvalServices verifies remote_api models and local workloads are aggregated.
func TestListAvailableEvalServices(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListModels(gomock.Any(), "remote_api", "", false).
		Return([]*dbclient.Model{{ID: "m1", DisplayName: "GPT", ModelName: "gpt", SourceURL: "http://api"}}, nil)
	m.EXPECT().SelectWorkloads(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.Workload{{WorkloadId: "w1", DisplayName: "infer"}}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, nil)
	h.ListAvailableEvalServices(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestListAvailableEvalServicesDBErrors verifies the handler still responds when DB calls fail.
func TestListAvailableEvalServicesDBErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListModels(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, assertErr)
	m.EXPECT().SelectWorkloads(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, assertErr)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, nil)
	h.ListAvailableEvalServices(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetEvaluationReport verifies a task report is rendered (no S3 client -> metadata only).
func TestGetEvaluationReport(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{
			TaskId:      "t1",
			TaskName:    "task",
			ServiceName: "svc",
			Status:      dbclient.EvaluationTaskStatusSucceeded,
		}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.GetEvaluationReport(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetEvaluationReportEmptyID verifies the empty-id branch.
func TestGetEvaluationReportEmptyID(t *testing.T) {
	h := &Handler{}
	c, w := evalCtx(t, nil)
	h.GetEvaluationReport(c)
	testifyassert.NotEqual(t, http.StatusOK, w.Code)
}

// --- merged from evaluation_types_test.go ---

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
		TaskId:      "eval-job-123",
		TaskName:    "Test Evaluation",
		ServiceName: "gpt-4",
		Status:      "Succeeded",
		Results: map[string]interface{}{
			"accuracy": 0.95,
			"f1_score": 0.92,
		},
		Duration: "1h30m",
	}

	assert.Equal(t, response.TaskId, "eval-job-123")
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
