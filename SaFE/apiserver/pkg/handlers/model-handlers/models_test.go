/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newMockModelHandler creates a mock Handler for testing
func newMockModelHandler(k8sClient client.Client) *Handler {
	return &Handler{
		k8sClient:        k8sClient,
		dbClient:         nil, // No database client for unit tests
		accessController: adminModelAC(),
	}
}

func newMockModelHandlerWithDB(k8sClient client.Client, dbClient dbclient.Interface) *Handler {
	return &Handler{
		k8sClient:        k8sClient,
		dbClient:         dbClient,
		accessController: adminModelAC(),
	}
}

// genMockK8sModel generates a mock K8s Model for testing
func genMockK8sModel(name string, accessMode v1.AccessMode, workspace string) *v1.Model {
	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test Model " + name,
			Description: "Test model for unit tests",
			Icon:        "https://example.com/icon.png",
			Label:       "test-org",
			Tags:        []string{"llm", "text-generation"},
			MaxTokens:   4096,
			Workspace:   workspace,
			Source: v1.ModelSource{
				URL:        "https://huggingface.co/test/model",
				AccessMode: accessMode,
				ModelName:  "test-model",
			},
		},
		Status: v1.ModelStatus{
			Phase:   v1.ModelPhaseReady,
			Message: "Model is ready",
		},
	}
	return model
}

// genMockRemoteAPIK8sModel generates a mock remote API Model
func genMockRemoteAPIK8sModel(name string) *v1.Model {
	model := genMockK8sModel(name, v1.AccessModeRemoteAPI, "")
	model.Spec.Source.URL = "https://api.openai.com"
	model.Spec.Source.ModelName = "gpt-4"
	return model
}

// genMockLocalK8sModel generates a mock local Model
func genMockLocalK8sModel(name string, workspace string) *v1.Model {
	model := genMockK8sModel(name, v1.AccessModeLocal, workspace)
	model.Spec.Source.URL = "https://huggingface.co/meta-llama/Llama-2-7b"
	model.Status.S3Path = "models/meta-llama-Llama-2-7b"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: workspace,
			Path:      "/apps/models/meta-llama-Llama-2-7b",
			Status:    v1.LocalPathStatusReady,
		},
	}
	return model
}

func genMockWorkspace(name, mountPath string) *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.WorkspaceSpec{
			Volumes: []v1.WorkspaceVolume{
				{
					Type:      v1.PFS,
					MountPath: mountPath,
				},
			},
		},
	}
}

// genMockWorkloadForModel generates a mock Workload associated with a model
func genMockWorkloadForModel(name, modelId, workspace string, phase v1.WorkloadPhase) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1.WorkloadSpec{
			Workspace: workspace,
			Env: map[string]string{
				"PRIMUS_SOURCE_MODEL": modelId,
				"MODEL_PATH":          "/apps/models/test-model",
			},
		},
		Status: v1.WorkloadStatus{
			Phase: phase,
		},
	}
}

// TestIsFullURL tests the isFullURL function
func TestIsFullURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "HTTPS URL",
			input:    "https://huggingface.co/model",
			expected: true,
		},
		{
			name:     "HTTP URL",
			input:    "http://huggingface.co/model",
			expected: true,
		},
		{
			name:     "Repo ID only",
			input:    "meta-llama/Llama-2-7b",
			expected: false,
		},
		{
			name:     "Short string",
			input:    "short",
			expected: false,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFullURL(tt.input)
			assert.Equal(t, result, tt.expected)
		})
	}
}

// TestGetModel tests the getModel handler
func TestGetModel(t *testing.T) {
	model := genMockRemoteAPIK8sModel("test-model-1")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	// Create test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "test-model-1"}}
	c.Request, _ = http.NewRequest("GET", "/models/test-model-1", nil)

	// Call handler
	result, err := h.getModel(c)
	assert.NilError(t, err)
	assert.Assert(t, result != nil)

	modelInfo := result.(ModelInfo)
	assert.Equal(t, modelInfo.ID, "test-model-1")
	assert.Equal(t, modelInfo.AccessMode, string(v1.AccessModeRemoteAPI))
}

// TestGetModel_NotFound tests getModel when model doesn't exist
func TestGetModel_NotFound(t *testing.T) {
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "non-existent"}}
	c.Request, _ = http.NewRequest("GET", "/models/non-existent", nil)

	_, err := h.getModel(c)
	assert.ErrorContains(t, err, "not found")
}

// TestListModels tests the listModels handler
func TestListModels(t *testing.T) {
	model1 := genMockRemoteAPIK8sModel("model-1")
	model2 := genMockLocalK8sModel("model-2", "ws1")
	model3 := genMockLocalK8sModel("model-3", "ws2")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model1, model2, model3).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	t.Run("List all models", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/models?limit=10&offset=0", nil)
		c.Set(common.UserId, adminModelUserID)

		result, err := h.listModels(c)
		assert.NilError(t, err)

		resp := result.(*ListModelResponse)
		assert.Equal(t, resp.Total, int64(3))
	})

	t.Run("Filter by accessMode", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/models?accessMode=remote_api", nil)
		c.Set(common.UserId, adminModelUserID)

		result, err := h.listModels(c)
		assert.NilError(t, err)

		resp := result.(*ListModelResponse)
		assert.Equal(t, resp.Total, int64(1))
		assert.Equal(t, resp.Items[0].ID, "model-1")
	})

	t.Run("Filter by workspace", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/models?workspace=ws1", nil)
		c.Set(common.UserId, adminModelUserID)

		result, err := h.listModels(c)
		assert.NilError(t, err)

		resp := result.(*ListModelResponse)
		// Should include ws1 model and remote_api model (public)
		assert.Assert(t, resp.Total >= 1)
	})
}

// TestDeleteModel tests the deleteModel handler
func TestDeleteModel(t *testing.T) {
	model := genMockRemoteAPIK8sModel("model-to-delete")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-to-delete"}}
	c.Request, _ = http.NewRequest("DELETE", "/models/model-to-delete", nil)
	c.Set(common.UserId, adminModelUserID)

	result, err := h.deleteModel(c)
	assert.NilError(t, err)
	assert.Assert(t, result != nil)

	// Verify model is deleted
	deletedModel := &v1.Model{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "model-to-delete"}, deletedModel)
	assert.ErrorContains(t, err, "not found")
}

// TestDeleteModel_WithRunningWorkloads tests deletion is blocked with running workloads
func TestDeleteModel_WithRunningWorkloads(t *testing.T) {
	model := genMockLocalK8sModel("model-with-workloads", "ws1")
	workload := genMockWorkloadForModel("workload-1", "model-with-workloads", "ws1", v1.WorkloadRunning)

	k8sClient := fake.NewClientBuilder().
		WithObjects(model, workload).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-with-workloads"}}
	c.Request, _ = http.NewRequest("DELETE", "/models/model-with-workloads", nil)
	c.Set(common.UserId, adminModelUserID)

	_, err := h.deleteModel(c)
	assert.ErrorContains(t, err, "running/pending workloads exist")
}

// TestDeleteModel_NotFound tests deletion of non-existent model
func TestDeleteModel_NotFound(t *testing.T) {
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "non-existent"}}
	c.Request, _ = http.NewRequest("DELETE", "/models/non-existent", nil)

	_, err := h.deleteModel(c)
	assert.ErrorContains(t, err, "not found")
}

// TestRetryModel tests the retryModel handler
func TestRetryModel(t *testing.T) {
	model := genMockLocalK8sModel("failed-model", "ws1")
	model.Status.Phase = v1.ModelPhaseFailed
	model.Status.Message = "Download failed"

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithStatusSubresource(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "failed-model"}}
	c.Request, _ = http.NewRequest("POST", "/models/failed-model/retry", nil)
	c.Set(common.UserId, adminModelUserID)

	result, err := h.retryModel(c)
	assert.NilError(t, err)
	assert.Assert(t, result != nil)

	// Verify model status is reset to Pending
	updatedModel := &v1.Model{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "failed-model"}, updatedModel)
	assert.NilError(t, err)
	assert.Equal(t, updatedModel.Status.Phase, v1.ModelPhasePending)
}

// TestRetryModel_NotFailed tests retry when model is not in failed state
func TestRetryModel_NotFailed(t *testing.T) {
	model := genMockLocalK8sModel("ready-model", "ws1")
	model.Status.Phase = v1.ModelPhaseReady

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "ready-model"}}
	c.Request, _ = http.NewRequest("POST", "/models/ready-model/retry", nil)
	c.Set(common.UserId, adminModelUserID)

	_, err := h.retryModel(c)
	assert.ErrorContains(t, err, "not in Failed phase")
}

// TestPatchModel tests the patchModel handler
func TestPatchModel(t *testing.T) {
	model := genMockRemoteAPIK8sModel("model-to-patch")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	newModelName := "updated-model-name"
	newDisplayName := "Updated Display Name"
	patchReq := PatchModelRequest{
		ModelName:   &newModelName,
		DisplayName: &newDisplayName,
	}
	body, _ := json.Marshal(patchReq)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-to-patch"}}
	c.Request, _ = http.NewRequest("PATCH", "/models/model-to-patch", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")

	c.Set(common.UserId, adminModelUserID)
	result, err := h.patchModel(c)
	assert.NilError(t, err)
	assert.Assert(t, result != nil)

	modelInfo := result.(ModelInfo)
	assert.Equal(t, modelInfo.ModelName, "updated-model-name")
	assert.Equal(t, modelInfo.DisplayName, "Updated Display Name")
}

// TestPatchModel_NoFields tests patch with no fields provided
func TestPatchModel_NoFields(t *testing.T) {
	model := genMockRemoteAPIK8sModel("model-to-patch")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	patchReq := PatchModelRequest{} // No fields
	body, _ := json.Marshal(patchReq)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-to-patch"}}
	c.Request, _ = http.NewRequest("PATCH", "/models/model-to-patch", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")

	_, err := h.patchModel(c)
	assert.ErrorContains(t, err, "at least one field must be provided")
}

// TestGetModelWorkloads tests the getModelWorkloads handler
func TestGetModelWorkloads(t *testing.T) {
	model := genMockLocalK8sModel("model-1", "ws1")
	workload1 := genMockWorkloadForModel("workload-1", "model-1", "ws1", v1.WorkloadRunning)
	workload2 := genMockWorkloadForModel("workload-2", "model-1", "ws1", v1.WorkloadPending)
	workload3 := genMockWorkloadForModel("workload-3", "other-model", "ws1", v1.WorkloadRunning)

	k8sClient := fake.NewClientBuilder().
		WithObjects(model, workload1, workload2, workload3).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-1"}}
	c.Request, _ = http.NewRequest("GET", "/models/model-1/workloads", nil)
	c.Set(common.UserId, adminModelUserID)

	result, err := h.getModelWorkloads(c)
	assert.NilError(t, err)

	resp := result.(*ModelWorkloadsResponse)
	assert.Equal(t, resp.Total, 2) // Only workloads associated with model-1
}

// TestGetWorkloadConfig tests the getWorkloadConfig handler
func TestGetWorkloadConfig(t *testing.T) {
	model := genMockLocalK8sModel("model-1", "")
	model.Status.Phase = v1.ModelPhaseReady
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/apps/models/test-model",
			Status:    v1.LocalPathStatusReady,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-1"}}
	c.Request, _ = http.NewRequest("GET", "/models/model-1/workload-config?workspace=ws1", nil)

	result, err := h.getWorkloadConfig(c)
	assert.NilError(t, err)

	config := result.(WorkloadConfigResponse)
	assert.Assert(t, config.DisplayName != "")
	assert.Equal(t, config.ModelID, "model-1")
	assert.Equal(t, config.Workspace, "ws1")
	assert.Assert(t, config.Env["PRIMUS_SOURCE_MODEL"] != "")
}

// TestGetWorkloadConfig_RemoteAPIModel tests workload config for remote API model
func TestGetWorkloadConfig_RemoteAPIModel(t *testing.T) {
	model := genMockRemoteAPIK8sModel("remote-model")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "remote-model"}}
	c.Request, _ = http.NewRequest("GET", "/models/remote-model/workload-config?workspace=ws1", nil)

	_, err := h.getWorkloadConfig(c)
	assert.ErrorContains(t, err, "only local models can be deployed")
}

// TestGetWorkloadConfig_ModelNotReady tests workload config for non-ready model
func TestGetWorkloadConfig_ModelNotReady(t *testing.T) {
	model := genMockLocalK8sModel("model-1", "")
	model.Status.Phase = v1.ModelPhaseDownloading

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-1"}}
	c.Request, _ = http.NewRequest("GET", "/models/model-1/workload-config?workspace=ws1", nil)

	_, err := h.getWorkloadConfig(c)
	assert.ErrorContains(t, err, "not ready")
}

func TestGetSftConfig(t *testing.T) {
	model := genMockLocalK8sModel("model-qwen", "ws1")
	model.Spec.DisplayName = "Qwen/Qwen3-8B"
	model.Spec.Source.URL = "https://huggingface.co/Qwen/Qwen3-8B"
	model.Spec.Source.ModelName = "Qwen/Qwen3-8B"

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-qwen"}}
	c.Request, _ = http.NewRequest("GET", "/models/model-qwen/sft-config?workspace=ws1", nil)

	c.Set(common.UserId, adminModelUserID)
	result, err := h.getSftConfig(c)
	assert.NilError(t, err)

	resp := result.(*SftConfigResponse)
	assert.Equal(t, resp.Supported, true)
	assert.Equal(t, resp.Model.ID, "model-qwen")
	assert.Equal(t, resp.Model.ModelName, "Qwen/Qwen3-8B")
	assert.Equal(t, resp.DatasetFilter.DatasetType, "sft")
	assert.Equal(t, resp.DatasetFilter.Workspace, "ws1")
	assert.Assert(t, resp.Defaults != nil)
	assert.Equal(t, resp.Defaults.ExportModel, true)
	assert.Equal(t, resp.Defaults.Priority, 1)
	assert.Equal(t, resp.Defaults.Image, GetDefaultSftImage())
	assert.Equal(t, resp.Defaults.TrainConfig.Peft, "none")
	assert.Equal(t, resp.Defaults.TrainConfig.TrainIters, 100)
	assert.Equal(t, resp.Defaults.TrainConfig.GlobalBatchSize, 8)
	assert.Equal(t, resp.Defaults.TrainConfig.MicroBatchSize, 1)
	assert.Equal(t, resp.Defaults.TrainConfig.SeqLength, 2048)
	assert.Equal(t, resp.Defaults.TrainConfig.FinetuneLr, 5e-6)
	assert.Equal(t, resp.Defaults.TrainConfig.LrWarmupIters, 5)
	assert.Equal(t, resp.Defaults.TrainConfig.SaveInterval, 50)
	assert.Equal(t, resp.Defaults.TrainConfig.TensorModelParallelSize, 1)
	assert.DeepEqual(t, resp.Options.DatasetFormatOptions, []string{"alpaca"})
	assert.DeepEqual(t, resp.Options.PeftOptions, []string{"none", "lora"})
}

func TestGetSftConfig_32BDefaults(t *testing.T) {
	model := genMockLocalK8sModel("model-qwen-32b", "ws1")
	model.Spec.DisplayName = "Qwen/Qwen3-32B"
	model.Spec.Source.URL = "https://huggingface.co/Qwen/Qwen3-32B"
	model.Spec.Source.ModelName = "Qwen/Qwen3-32B"

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-qwen-32b"}}
	c.Request, _ = http.NewRequest("GET", "/models/model-qwen-32b/sft-config?workspace=ws1", nil)

	c.Set(common.UserId, adminModelUserID)
	result, err := h.getSftConfig(c)
	assert.NilError(t, err)

	resp := result.(*SftConfigResponse)
	assert.Equal(t, resp.Supported, true)
	assert.Assert(t, resp.Defaults != nil)
	assert.Equal(t, resp.Defaults.TrainConfig.Peft, "none")
	assert.Equal(t, resp.Defaults.TrainConfig.TrainIters, 1000)
	assert.Equal(t, resp.Defaults.TrainConfig.GlobalBatchSize, 8)
	assert.Equal(t, resp.Defaults.TrainConfig.MicroBatchSize, 1)
	assert.Equal(t, resp.Defaults.TrainConfig.SeqLength, 2048)
	assert.Equal(t, resp.Defaults.TrainConfig.FinetuneLr, 5e-6)
	assert.Equal(t, resp.Defaults.TrainConfig.LrWarmupIters, 10)
	assert.Equal(t, resp.Defaults.TrainConfig.SaveInterval, 500)
	assert.Equal(t, resp.Defaults.TrainConfig.TensorModelParallelSize, 8)
}

func TestGetSftConfig_InferRecipeForGenericQwenModel(t *testing.T) {
	model := genMockLocalK8sModel("model-qwen-generic", "ws1")
	model.Spec.DisplayName = "Qwen/Qwen2.5-7B-Instruct"
	model.Spec.Source.URL = "https://huggingface.co/Qwen/Qwen2.5-7B-Instruct"
	model.Spec.Source.ModelName = "Qwen/Qwen2.5-7B-Instruct"

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-qwen-generic"}}
	c.Request, _ = http.NewRequest("GET", "/models/model-qwen-generic/sft-config?workspace=ws1", nil)

	c.Set(common.UserId, adminModelUserID)
	result, err := h.getSftConfig(c)
	assert.NilError(t, err)

	resp := result.(*SftConfigResponse)
	assert.Equal(t, resp.Supported, true)
	assert.Assert(t, resp.Defaults != nil)
	assert.Equal(t, resp.Defaults.Recipe, "qwen.qwen3")
	assert.Equal(t, resp.Defaults.Flavor, "qwen3_8b_finetune_config")
	assert.Equal(t, resp.Defaults.ModelSize, "8b")
	assert.DeepEqual(t, resp.Options.PeftOptions, []string{"none", "lora"})
}

func TestGetSftConfig_OverrideAllowsUnknownModel(t *testing.T) {
	model := genMockLocalK8sModel("model-custom", "ws1")
	model.Spec.DisplayName = "Acme/Custom-9B"
	model.Spec.Source.URL = "https://huggingface.co/Acme/Custom-9B"
	model.Spec.Source.ModelName = "Acme/Custom-9B"

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-custom"}}
	c.Request, _ = http.NewRequest(
		"GET",
		"/models/model-custom/sft-config?workspace=ws1&recipe=qwen.qwen3&flavor=qwen3_8b_finetune_config&modelSize=8b",
		nil,
	)

	c.Set(common.UserId, adminModelUserID)
	result, err := h.getSftConfig(c)
	assert.NilError(t, err)

	resp := result.(*SftConfigResponse)
	assert.Equal(t, resp.Supported, true)
	assert.Assert(t, resp.Defaults != nil)
	assert.Equal(t, resp.Defaults.Recipe, "qwen.qwen3")
	assert.Equal(t, resp.Defaults.Flavor, "qwen3_8b_finetune_config")
	assert.Equal(t, resp.Defaults.ModelSize, "8b")
}

func TestGetSftConfig_UnknownModelFallsBackToDefaultRecipe(t *testing.T) {
	model := genMockLocalK8sModel("model-generic-fallback", "ws1")
	model.Spec.DisplayName = "Tongyi-MAI/Z-Image-Turbo"
	model.Spec.Source.URL = "https://huggingface.co/Tongyi-MAI/Z-Image-Turbo"
	model.Spec.Source.ModelName = "Tongyi-MAI/Z-Image-Turbo"

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-generic-fallback"}}
	c.Request, _ = http.NewRequest("GET", "/models/model-generic-fallback/sft-config?workspace=ws1", nil)

	c.Set(common.UserId, adminModelUserID)
	result, err := h.getSftConfig(c)
	assert.NilError(t, err)

	resp := result.(*SftConfigResponse)
	assert.Equal(t, resp.Supported, true)
	assert.Assert(t, resp.Defaults != nil)
	assert.Equal(t, resp.Defaults.Recipe, "qwen.qwen3")
	assert.Equal(t, resp.Defaults.Flavor, "qwen3_8b_finetune_config")
	assert.Equal(t, resp.Defaults.ModelSize, "8b")
	assert.Equal(t, resp.Reason, "")
}

func TestGetSftConfig_SharedLocalPathAccessible(t *testing.T) {
	model := genMockLocalK8sModel("model-qwen-shared-local", "ws2")
	model.Spec.DisplayName = "Qwen/Qwen3-8B"
	model.Spec.Source.URL = "https://huggingface.co/Qwen/Qwen3-8B"
	model.Spec.Source.ModelName = "Qwen/Qwen3-8B"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws2",
			Path:      "/shared/models/qwen3-8b",
			Status:    v1.LocalPathStatusReady,
		},
	}
	ws1 := genMockWorkspace("ws1", "/shared")
	ws2 := genMockWorkspace("ws2", "/shared")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model, ws1, ws2).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-qwen-shared-local"}}
	c.Request, _ = http.NewRequest("GET", "/models/model-qwen-shared-local/sft-config?workspace=ws1", nil)

	c.Set(common.UserId, adminModelUserID)
	result, err := h.getSftConfig(c)
	assert.NilError(t, err)

	resp := result.(*SftConfigResponse)
	assert.Equal(t, resp.Supported, true)
	assert.Assert(t, resp.Defaults != nil)
}

func TestGetSftConfig_MissingLocalPathInWorkspace(t *testing.T) {
	model := genMockLocalK8sModel("model-qwen-missing-local", "ws2")
	model.Spec.DisplayName = "Qwen/Qwen3-8B"
	model.Spec.Source.URL = "https://huggingface.co/Qwen/Qwen3-8B"
	model.Spec.Source.ModelName = "Qwen/Qwen3-8B"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws2",
			Path:      "/apps/models/qwen3-8b",
			Status:    v1.LocalPathStatusReady,
		},
	}
	ws1 := genMockWorkspace("ws1", "/workspace-a")
	ws2 := genMockWorkspace("ws2", "/workspace-b")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model, ws1, ws2).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-qwen-missing-local"}}
	c.Request, _ = http.NewRequest("GET", "/models/model-qwen-missing-local/sft-config?workspace=ws1", nil)

	c.Set(common.UserId, adminModelUserID)
	result, err := h.getSftConfig(c)
	assert.NilError(t, err)

	resp := result.(*SftConfigResponse)
	assert.Equal(t, resp.Supported, false)
	assert.Assert(t, strings.Contains(resp.Reason, "not available locally"))
	assert.Assert(t, resp.Defaults == nil)
}

func TestResolveDatasetPath_SharedLocalPathAccessible(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	localPaths, err := json.Marshal([]dbclient.DatasetLocalPathDB{
		{
			Workspace: "ws2",
			Path:      "/shared/datasets/alpaca.jsonl",
			Status:    dbclient.DatasetStatusReady,
		},
	})
	assert.NilError(t, err)

	mockDB.EXPECT().
		GetDataset(gomock.Any(), "dataset-shared-local").
		Return(&dbclient.Dataset{
			DatasetId:  "dataset-shared-local",
			LocalPaths: string(localPaths),
		}, nil)

	ws1 := genMockWorkspace("ws1", "/shared")
	ws2 := genMockWorkspace("ws2", "/shared")
	k8sClient := fake.NewClientBuilder().
		WithObjects(ws1, ws2).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandlerWithDB(k8sClient, mockDB)

	path, err := h.resolveDatasetPath(context.Background(), "dataset-shared-local", "ws1")
	assert.NilError(t, err)
	assert.Equal(t, path, "/shared/datasets/alpaca.jsonl")
}

func TestCreateSftJob_InjectsAinicEnvForSharedNfsMultinode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	localPaths, err := json.Marshal([]dbclient.DatasetLocalPathDB{
		{
			Workspace: "ws1",
			Path:      "/shared_nfs/datasets/alpaca",
			Status:    dbclient.DatasetStatusReady,
		},
	})
	assert.NilError(t, err)

	mockDB.EXPECT().
		GetDataset(gomock.Any(), "dataset-sft-ainic").
		Return(&dbclient.Dataset{
			DatasetId:  "dataset-sft-ainic",
			LocalPaths: string(localPaths),
		}, nil).
		AnyTimes()

	model := genMockLocalK8sModel("model-qwen-ainic", "ws1")
	model.Spec.DisplayName = "Qwen/Qwen3-8B"
	model.Spec.Source.URL = "https://huggingface.co/Qwen/Qwen3-8B"
	model.Spec.Source.ModelName = "Qwen/Qwen3-8B"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/shared_nfs/models/Qwen/Qwen3-8B",
			Status:    v1.LocalPathStatusReady,
		},
	}
	ws1 := genMockWorkspace("ws1", "/shared_nfs")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model, ws1).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandlerWithDB(k8sClient, mockDB)

	exportModel := false
	reqBody, err := json.Marshal(CreateSftJobRequest{
		DisplayName:      "ainic-multinode-sft",
		Workspace:        "ws1",
		ModelId:          "model-qwen-ainic",
		DatasetId:        "dataset-sft-ainic",
		ExportModel:      &exportModel,
		Image:            "test-image",
		NodeCount:        2,
		GpuCount:         8,
		Cpu:              "80",
		Memory:           "1000Gi",
		SharedMemory:     "500Gi",
		EphemeralStorage: "1000Gi",
		TrainConfig: SftTrainConfig{
			Peft: "none",
		},
	})
	assert.NilError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/sft/jobs", bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, "user-1")
	c.Set(common.UserName, "Test User")

	result, err := h.createSftJob(c)
	assert.NilError(t, err)

	resp := result.(*CreateSftJobResponse)
	workload := &v1.Workload{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: resp.WorkloadId}, workload)
	assert.NilError(t, err)

	assert.Equal(t, workload.Spec.Env["USING_AINIC"], "1")
	assert.Equal(t, workload.Spec.Env["NCCL_IB_GID_INDEX"], "1")
	assert.Equal(t, workload.Spec.Env["DATA_PATH"], "/shared_nfs/sft-shared-data/"+resp.WorkloadId)
}

func TestCreateSftJob_OverrideAllowsUnknownModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	localPaths, err := json.Marshal([]dbclient.DatasetLocalPathDB{
		{
			Workspace: "ws1",
			Path:      "/shared_nfs/datasets/custom-sft",
			Status:    dbclient.DatasetStatusReady,
		},
	})
	assert.NilError(t, err)

	mockDB.EXPECT().
		GetDataset(gomock.Any(), "dataset-custom-sft").
		Return(&dbclient.Dataset{
			DatasetId:  "dataset-custom-sft",
			LocalPaths: string(localPaths),
		}, nil).
		AnyTimes()

	model := genMockLocalK8sModel("model-custom-override", "ws1")
	model.Spec.DisplayName = "Acme/Custom-9B"
	model.Spec.Source.URL = "https://huggingface.co/Acme/Custom-9B"
	model.Spec.Source.ModelName = "Acme/Custom-9B"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/shared_nfs/models/custom-9b",
			Status:    v1.LocalPathStatusReady,
		},
	}
	ws1 := genMockWorkspace("ws1", "/shared_nfs")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model, ws1).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandlerWithDB(k8sClient, mockDB)

	exportModel := false
	reqBody, err := json.Marshal(CreateSftJobRequest{
		DisplayName:      "custom-override-sft",
		Workspace:        "ws1",
		ModelId:          "model-custom-override",
		DatasetId:        "dataset-custom-sft",
		Recipe:           "qwen.qwen3",
		Flavor:           "qwen3_8b_finetune_config",
		ModelSize:        "8b",
		ExportModel:      &exportModel,
		Image:            "test-image",
		NodeCount:        1,
		GpuCount:         8,
		Cpu:              "80",
		Memory:           "1000Gi",
		EphemeralStorage: "1000Gi",
		TrainConfig: SftTrainConfig{
			Peft: "lora",
		},
	})
	assert.NilError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/sft/jobs", bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, "user-1")
	c.Set(common.UserName, "Test User")

	result, err := h.createSftJob(c)
	assert.NilError(t, err)

	resp := result.(*CreateSftJobResponse)
	workload := &v1.Workload{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: resp.WorkloadId}, workload)
	assert.NilError(t, err)
	assert.Equal(t, workload.Spec.Images[0], "test-image")

	entrypoint, err := base64.StdEncoding.DecodeString(workload.Spec.EntryPoints[0])
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(entrypoint), "recipe: qwen.qwen3"))
	assert.Assert(t, strings.Contains(string(entrypoint), "flavor: qwen3_8b_finetune_config"))
	assert.Assert(t, strings.Contains(string(entrypoint), `peft: "lora"`))
}

func TestCreateSftJob_UnknownModelFallsBackToDefaultRecipe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	localPaths, err := json.Marshal([]dbclient.DatasetLocalPathDB{
		{
			Workspace: "ws1",
			Path:      "/shared_nfs/datasets/fallback-sft",
			Status:    dbclient.DatasetStatusReady,
		},
	})
	assert.NilError(t, err)

	mockDB.EXPECT().
		GetDataset(gomock.Any(), "dataset-fallback-sft").
		Return(&dbclient.Dataset{
			DatasetId:  "dataset-fallback-sft",
			LocalPaths: string(localPaths),
		}, nil).
		AnyTimes()

	model := genMockLocalK8sModel("model-fallback", "ws1")
	model.Spec.DisplayName = "Tongyi-MAI/Z-Image-Turbo"
	model.Spec.Source.URL = "https://huggingface.co/Tongyi-MAI/Z-Image-Turbo"
	model.Spec.Source.ModelName = "Tongyi-MAI/Z-Image-Turbo"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/shared_nfs/models/z-image-turbo",
			Status:    v1.LocalPathStatusReady,
		},
	}
	ws1 := genMockWorkspace("ws1", "/shared_nfs")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model, ws1).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandlerWithDB(k8sClient, mockDB)

	exportModel := false
	reqBody, err := json.Marshal(CreateSftJobRequest{
		DisplayName:      "fallback-sft",
		Workspace:        "ws1",
		ModelId:          "model-fallback",
		DatasetId:        "dataset-fallback-sft",
		ExportModel:      &exportModel,
		Image:            "test-image",
		NodeCount:        1,
		GpuCount:         8,
		Cpu:              "80",
		Memory:           "1000Gi",
		EphemeralStorage: "1000Gi",
		TrainConfig: SftTrainConfig{
			Peft: "lora",
		},
	})
	assert.NilError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/sft/jobs", bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, "user-1")
	c.Set(common.UserName, "Test User")

	result, err := h.createSftJob(c)
	assert.NilError(t, err)

	resp := result.(*CreateSftJobResponse)
	workload := &v1.Workload{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: resp.WorkloadId}, workload)
	assert.NilError(t, err)

	entrypoint, err := base64.StdEncoding.DecodeString(workload.Spec.EntryPoints[0])
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(entrypoint), "recipe: qwen.qwen3"))
	assert.Assert(t, strings.Contains(string(entrypoint), "flavor: qwen3_8b_finetune_config"))
}

func TestGetSftConfig_UnsupportedModel(t *testing.T) {
	model := genMockRemoteAPIK8sModel("remote-model")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "remote-model"}}
	c.Request, _ = http.NewRequest("GET", "/models/remote-model/sft-config?workspace=ws1", nil)

	c.Set(common.UserId, adminModelUserID)
	result, err := h.getSftConfig(c)
	assert.NilError(t, err)

	resp := result.(*SftConfigResponse)
	assert.Equal(t, resp.Supported, false)
	assert.Equal(t, resp.Reason, "only local or local_path models can be fine-tuned")
	assert.Assert(t, resp.Defaults == nil)
}

// TestConvertK8sModelToInfo tests the convertK8sModelToInfo function
func TestConvertK8sModelToInfo(t *testing.T) {
	model := genMockLocalK8sModel("test-model", "ws1")
	model.Spec.Tags = []string{"llm", "text-generation", "english"}
	model.Spec.Origin = "fine_tuned"
	model.Spec.SftJobId = "sft-job-1"
	model.Spec.BaseModel = "Qwen/Qwen3-8B"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/apps/models/test",
			Status:    v1.LocalPathStatusReady,
			Message:   "Download completed",
		},
	}
	v1.SetLabel(model, v1.UserIdLabel, "user-1")
	v1.SetAnnotation(model, v1.UserNameAnnotation, "Test User")

	h := newMockModelHandler(nil)
	info := h.convertK8sModelToInfo(model)

	assert.Equal(t, info.ID, "test-model")
	assert.Equal(t, info.DisplayName, model.Spec.DisplayName)
	assert.Equal(t, info.Description, model.Spec.Description)
	assert.Equal(t, info.AccessMode, string(v1.AccessModeLocal))
	assert.Equal(t, info.Workspace, "ws1")
	assert.Equal(t, len(info.LocalPaths), 1)
	assert.Equal(t, info.LocalPaths[0].Workspace, "ws1")
	assert.Equal(t, info.Origin, "fine_tuned")
	assert.Equal(t, info.SftJobId, "sft-job-1")
	assert.Equal(t, info.BaseModel, "Qwen/Qwen3-8B")
	assert.Equal(t, info.UserId, "user-1")
	assert.Equal(t, info.UserName, "Test User")
}

// TestParseListModelQuery tests the parseListModelQuery function
func TestParseListModelQuery(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedLimit  int
		expectedOffset int
	}{
		{
			name:           "Default values",
			query:          "",
			expectedLimit:  0,
			expectedOffset: 0,
		},
		{
			name:           "Custom values",
			query:          "limit=20&offset=5",
			expectedLimit:  20,
			expectedOffset: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("GET", "/models?"+tt.query, nil)

			query, err := parseListModelQuery(c)
			assert.NilError(t, err)
			assert.Equal(t, query.Limit, tt.expectedLimit)
			assert.Equal(t, query.Offset, tt.expectedOffset)
		})
	}
}

// TestFindModelBySourceURL tests the findModelBySourceURL function
func TestFindModelBySourceURL(t *testing.T) {
	model1 := genMockLocalK8sModel("model-1", "")
	model1.Spec.Source.URL = "https://huggingface.co/test/model-a"

	model2 := genMockLocalK8sModel("model-2", "ws1")
	model2.Spec.Source.URL = "https://huggingface.co/test/model-b"

	k8sClient := fake.NewClientBuilder().
		WithObjects(model1, model2).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	t.Run("Find existing model", func(t *testing.T) {
		found, err := h.findModelBySourceURL(context.Background(), "https://huggingface.co/test/model-a", "")
		assert.NilError(t, err)
		assert.Assert(t, found != nil)
		assert.Equal(t, found.ID, "model-1")
	})

	t.Run("Model not found", func(t *testing.T) {
		found, err := h.findModelBySourceURL(context.Background(), "https://huggingface.co/non/existent", "")
		assert.NilError(t, err)
		assert.Assert(t, found == nil)
	})
}

// TestDeleteModelWithSecrets tests deletion with token and apiKey secrets
func TestDeleteModelWithSecrets(t *testing.T) {
	model := genMockRemoteAPIK8sModel("model-with-secrets")
	model.Spec.Source.Token = &corev1.LocalObjectReference{Name: "model-with-secrets-token"}
	model.Spec.Source.ApiKey = &corev1.LocalObjectReference{Name: "model-with-secrets-apikey"}

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-with-secrets-token",
			Namespace: common.PrimusSafeNamespace,
		},
	}

	apiKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-with-secrets-apikey",
			Namespace: common.PrimusSafeNamespace,
		},
	}

	// Add corev1 scheme for Secret support
	testScheme := scheme.Scheme
	_ = corev1.AddToScheme(testScheme)

	k8sClient := fake.NewClientBuilder().
		WithObjects(model, tokenSecret, apiKeySecret).
		WithScheme(testScheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "model-with-secrets"}}
	c.Request, _ = http.NewRequest("DELETE", "/models/model-with-secrets", nil)
	c.Set(common.UserId, adminModelUserID)

	result, err := h.deleteModel(c)
	assert.NilError(t, err)
	assert.Assert(t, result != nil)

	// Verify secrets are deleted
	tokenSecretCheck := &corev1.Secret{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{
		Name:      "model-with-secrets-token",
		Namespace: common.PrimusSafeNamespace,
	}, tokenSecretCheck)
	assert.ErrorContains(t, err, "not found")

	apiKeySecretCheck := &corev1.Secret{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{
		Name:      "model-with-secrets-apikey",
		Namespace: common.PrimusSafeNamespace,
	}, apiKeySecretCheck)
	assert.ErrorContains(t, err, "not found")
}

// TestModelInfoTags tests tag handling in ModelInfo
func TestModelInfoTags(t *testing.T) {
	model := genMockLocalK8sModel("test-model", "")
	model.Spec.Tags = []string{"llm", "text-generation", "pytorch", "transformers"}

	h := newMockModelHandler(nil)
	info := h.convertK8sModelToInfo(model)

	assert.Equal(t, info.Tags, "llm,text-generation,pytorch,transformers")
	assert.Assert(t, len(info.CategorizedTags) > 0)
}

// TestModelPhaseMessages tests model phase and message handling
func TestModelPhaseMessages(t *testing.T) {
	tests := []struct {
		phase   v1.ModelPhase
		message string
	}{
		{v1.ModelPhasePending, "Waiting for processing"},
		{v1.ModelPhaseUploading, "Uploading to S3"},
		{v1.ModelPhaseDownloading, "Downloading to local storage"},
		{v1.ModelPhaseReady, "Model is ready"},
		{v1.ModelPhaseFailed, "Download failed"},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			model := genMockLocalK8sModel("test-model", "")
			model.Status.Phase = tt.phase
			model.Status.Message = tt.message

			h := newMockModelHandler(nil)
			info := h.convertK8sModelToInfo(model)

			assert.Equal(t, info.Phase, string(tt.phase))
			assert.Equal(t, info.Message, tt.message)
		})
	}
}

// --- merged from models_db_read_rbac_test.go ---

// dbReadModels returns a fixed set of DB models with mixed visibility used to
// exercise read-path RBAC on the database code path.
func dbReadModels() []*dbclient.Model {
	return []*dbclient.Model{
		{ID: "d-pub", AccessMode: "local", DisplayName: "pub", UserId: "owner-1", Workspace: ""},
		{ID: "d-own", AccessMode: "local", DisplayName: "own", UserId: "owner-1", Workspace: "ws-1"},
		{ID: "d-wsonly", AccessMode: "local", DisplayName: "wsonly", UserId: "nobody", Workspace: "ws-1"},
		{ID: "d-other", AccessMode: "local", DisplayName: "other", UserId: "stranger-1", Workspace: "ws-2"},
	}
}

func newDBReadRBACHandler(t *testing.T, m dbclient.Interface) *Handler {
	t.Helper()
	return &Handler{
		dbClient:         m,
		k8sClient:        ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build(),
		accessController: newReadRBACAC(t),
	}
}

// TestListModelsDBReadRBAC verifies #9: read visibility filtering also applies
// on the database code path of listModels (not only the K8s fallback).
func TestListModelsDBReadRBAC(t *testing.T) {
	cases := []struct {
		name string
		user string
		want []string
	}{
		{"member sees public + ws-1", "member-1", []string{"d-pub", "d-own", "d-wsonly"}},
		{"owner sees public + owned", "owner-1", []string{"d-pub", "d-own"}},
		{"stranger sees public + owned", "stranger-1", []string{"d-pub", "d-other"}},
		{"admin sees all", "admin-1", []string{"d-pub", "d-own", "d-wsonly", "d-other"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			m := mock_client.NewMockInterface(ctrl)
			m.EXPECT().ListModels(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(dbReadModels(), nil)
			h := newDBReadRBACHandler(t, m)

			res, err := h.listModels(readRBACCtx(tc.user, "limit=100&offset=0", nil))
			testifyassert.NoError(t, err)
			resp, ok := res.(*ListModelResponse)
			testifyassert.True(t, ok)
			got := make(map[string]bool, len(resp.Items))
			for _, it := range resp.Items {
				got[it.ID] = true
			}
			assert.Equal(t, len(tc.want), len(got), "unexpected visible set: %v", got)
			for _, id := range tc.want {
				testifyassert.True(t, got[id], "expected %s visible to %s", id, tc.user)
			}
		})
	}
}

// TestGetModelDBReadRBAC verifies #9: getModel enforces read authorization on
// the database code path and returns 403 for models the caller may not access.
func TestGetModelDBReadRBAC(t *testing.T) {
	cases := []struct {
		name   string
		user   string
		denied bool
	}{
		{"member denied other workspace", "member-1", true},
		{"owner allowed", "stranger-1", false},
		{"admin allowed", "admin-1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			m := mock_client.NewMockInterface(ctrl)
			m.EXPECT().GetModelByID(gomock.Any(), "d-other").
				Return(&dbclient.Model{ID: "d-other", AccessMode: "local", UserId: "stranger-1", Workspace: "ws-2"}, nil)
			h := newDBReadRBACHandler(t, m)

			_, err := h.getModel(readRBACCtx(tc.user, "", gin.Params{{Key: "id", Value: "d-other"}}))
			if tc.denied {
				testifyassert.Error(t, err)
				testifyassert.Contains(t, err.Error(), "not allowed")
			} else {
				testifyassert.NoError(t, err)
			}
		})
	}
}

// --- merged from models_pure_test.go ---

// TestTagsToDB verifies tag slice serialization to a comma-joined string.
func TestTagsToDB(t *testing.T) {
	if tagsToDB(nil) != "" {
		t.Error("expected empty string for nil tags")
	}
	if got := tagsToDB([]string{"a", "b", "c"}); got != "a,b,c" {
		t.Errorf("unexpected joined tags: %s", got)
	}
}

// TestExtractTarget verifies target normalization and subpath validation.
func TestExtractTarget(t *testing.T) {
	vol, sub, err := extractTarget(nil)
	if err != nil || vol != "" || sub != "" {
		t.Errorf("nil target should return empty values, got vol=%q sub=%q err=%v", vol, sub, err)
	}

	vol, sub, err = extractTarget(&ModelTargetReq{Volume: " data ", Subpath: "/models/foo/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vol != "data" || sub != "models/foo" {
		t.Errorf("unexpected normalization: vol=%q sub=%q", vol, sub)
	}

	if _, _, err := extractTarget(&ModelTargetReq{Subpath: "../etc"}); err == nil {
		t.Error("expected error for path-traversal subpath")
	}
}

// TestIsSafeSubpath verifies allowed and rejected subpaths.
func TestIsSafeSubpath(t *testing.T) {
	if !isSafeSubpath("") {
		t.Error("empty subpath should be safe")
	}
	if !isSafeSubpath("models/foo-bar_1.2/baz") {
		t.Error("valid subpath should be safe")
	}
	if isSafeSubpath("foo/../bar") {
		t.Error("path traversal should be rejected")
	}
	if isSafeSubpath("foo bar") {
		t.Error("space should be rejected")
	}
}

// TestIsSafeS3URI verifies allowed and rejected S3 URIs.
func TestIsSafeS3URI(t *testing.T) {
	if isSafeS3URI("") {
		t.Error("empty S3 URI should be unsafe")
	}
	if !isSafeS3URI("s3://bucket/key-1_2.bin") {
		t.Error("valid S3 URI should be safe")
	}
	if isSafeS3URI("s3://bucket/$(rm -rf)") {
		t.Error("shell metacharacters should be rejected")
	}
}

// TestIsSafeURL verifies allowed and rejected endpoint URLs.
func TestIsSafeURL(t *testing.T) {
	if !isSafeURL("") {
		t.Error("empty URL should be considered safe (optional)")
	}
	if !isSafeURL("https://s3.example.com:9000") {
		t.Error("valid URL should be safe")
	}
	if isSafeURL("https://x?a=b") {
		t.Error("query characters should be rejected")
	}
}

// TestModelNameSortKey verifies the sort key strips prefix and lowercases.
func TestModelNameSortKey(t *testing.T) {
	if got := modelNameSortKey("Qwen/Qwen3-8B"); got != "qwen3-8b" {
		t.Errorf("unexpected sort key: %s", got)
	}
	if got := modelNameSortKey("  Plain  "); got != "plain" {
		t.Errorf("unexpected sort key: %s", got)
	}
}

// TestMatchModelOrigin verifies origin matching semantics.
func TestMatchModelOrigin(t *testing.T) {
	if !matchModelOrigin("custom", "custom") {
		t.Error("custom origin should match custom query")
	}
	if matchModelOrigin("external", "custom") {
		t.Error("external origin should not match custom query")
	}
	if !matchModelOrigin("external", "external") {
		t.Error("exact origin should match")
	}
}

// TestEnrichInferenceXInfo verifies InferenceX availability is marked by display name.
func TestEnrichInferenceXInfo(t *testing.T) {
	items := []ModelInfo{
		{DisplayName: "deepseek-ai/DeepSeek-R1-0528"},
		{DisplayName: "unknown-org/Unknown-Model"},
	}
	enrichInferenceXInfo(items)

	if !items[0].HasInferenceX || items[0].InferenceXModel != "DeepSeek-R1-0528" {
		t.Errorf("expected first model to be marked InferenceX, got %+v", items[0])
	}
	if items[1].HasInferenceX {
		t.Error("unknown model should not be marked InferenceX")
	}
}

// TestSanitizeLabelValue verifies invalid chars are replaced and length is bounded.
func TestSanitizeLabelValue(t *testing.T) {
	if sanitizeLabelValue("") != "" {
		t.Error("empty stays empty")
	}
	if got := sanitizeLabelValue("Qwen/Qwen3-8B"); got != "Qwen_Qwen3-8B" {
		t.Errorf("unexpected sanitized label: %s", got)
	}
	long := strings.Repeat("a", 100)
	if len(sanitizeLabelValue(long)) > 63 {
		t.Error("label value must be bounded to 63 chars")
	}
}

// --- merged from models_rbac_test.go ---

// adminModelUserID is the user id used by existing write-op tests; it is seeded
// as a system administrator so those tests keep passing under fail-closed RBAC.
const adminModelUserID = "u1"

var (
	adminModelACOnce sync.Once
	adminModelACInst *authority.AccessController
)

// adminModelAC returns a shared AccessController whose backing store contains a
// wildcard system-admin role bound to adminModelUserID. Existing model write
// tests use this via newMockModelHandler so their happy paths keep working.
// It is built with a direct struct (not authority.NewAccessController) to avoid
// the process-wide singleton created elsewhere in the test binary.
func adminModelAC() *authority.AccessController {
	adminModelACOnce.Do(func() {
		scheme := runtime.NewScheme()
		_ = v1.AddToScheme(scheme)
		role := &v1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: string(v1.SystemAdminRole)},
			Rules: []v1.PolicyRule{{
				Resources:    []string{authority.AllResource},
				GrantedUsers: []string{authority.GrantedAllUser},
				Verbs:        []v1.RoleVerb{v1.AllVerb},
			}},
		}
		user := &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: adminModelUserID},
			Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
		}
		adminModelACInst = &authority.AccessController{
			Client: ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(role, user).Build(),
		}
	})
	return adminModelACInst
}

// newModelOwnerAC builds an AccessController that grants model owners
// get/list/update/delete and workspace members create, mirroring the default
// role rules. Users owner-1 and other-1 both carry this role but neither is an
// administrator, so ownership is what decides access.
func newModelOwnerAC(t *testing.T) *authority.AccessController {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "model-role"},
		Rules: []v1.PolicyRule{
			{
				Resources:    []string{"model"},
				GrantedUsers: []string{authority.GrantedOwner},
				Verbs:        []v1.RoleVerb{v1.GetVerb, v1.ListVerb, v1.UpdateVerb, v1.DeleteVerb},
			},
			{
				Resources:    []string{"model"},
				GrantedUsers: []string{authority.GrantedWorkspaceUser},
				Verbs:        []v1.RoleVerb{v1.CreateVerb},
			},
		},
	}
	owner := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "owner-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{"model-role"}},
	}
	other := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "other-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{"model-role"}},
	}
	// wsmember-1 is a member of workspace "ws-1" (workspace-user), so it may
	// create models in ws-1 but is not an owner of existing models.
	wsMember := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "wsmember-1"},
		Spec: v1.UserSpec{
			Type:      v1.DefaultUserType,
			Roles:     []v1.UserRole{"model-role"},
			Resources: map[string][]string{common.UserWorkspaces: {"ws-1"}},
		},
	}
	return &authority.AccessController{
		Client: ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(role, owner, other, wsMember).Build(),
	}
}

func ownedModel(name, owner string) *v1.Model {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: name}}
	m.Labels = map[string]string{v1.UserIdLabel: owner}
	m.Spec.Source.AccessMode = v1.AccessModeRemoteAPI
	m.Status.Phase = v1.ModelPhaseReady
	return m
}

func deleteCtx(userID, modelID string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: modelID}}
	if userID != "" {
		c.Set(common.UserId, userID)
	}
	return c, w
}

// TestDeleteModelDeniedForNonOwner verifies S3: a user who is neither the owner
// nor an admin cannot delete another user's model.
func TestDeleteModelDeniedForNonOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	model := ownedModel("m-owned", "owner-1")
	k8s := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}

	c, _ := deleteCtx("other-1", "m-owned")
	_, err := h.deleteModel(c)
	if err == nil {
		t.Fatal("expected forbidden error for non-owner delete, got nil")
	}
	if code := getHTTPStatusCode(err); code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%v)", code, err)
	}
}

// TestDeleteModelAllowedForOwner verifies the owner can delete their own model.
func TestDeleteModelAllowedForOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	model := ownedModel("m-owned", "owner-1")
	k8s := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}

	c, _ := deleteCtx("owner-1", "m-owned")
	if _, err := h.deleteModel(c); err != nil {
		t.Fatalf("expected owner delete to succeed, got %v", err)
	}
}

// TestDeleteModelDeniedWhenNoAccessController verifies fail-closed behavior.
func TestDeleteModelDeniedWhenNoAccessController(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	model := ownedModel("m-owned", "owner-1")
	k8s := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s} // no access controller

	c, _ := deleteCtx("owner-1", "m-owned")
	if _, err := h.deleteModel(c); err == nil {
		t.Fatal("expected fail-closed error when access controller is nil, got nil")
	}
}

// TestRetryModelDeniedForNonOwner verifies that retry (a re-download, i.e. a
// state-changing write) is not allowed for a non-owner, matching patch/delete.
func TestRetryModelDeniedForNonOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	model := ownedModel("m-owned", "owner-1")
	model.Status.Phase = v1.ModelPhaseFailed
	k8s := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}

	c, _ := deleteCtx("other-1", "m-owned")
	_, err := h.retryModel(c)
	if err == nil {
		t.Fatal("expected forbidden error for non-owner retry, got nil")
	}
	if code := getHTTPStatusCode(err); code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%v)", code, err)
	}
}

func modelScheme2(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return s
}

// TestCreateModelDeniedForNonAdminPublicModel: creating a public model (empty
// workspace) is admin-only; a normal user must be denied.
func TestCreateModelDeniedForNonAdminPublicModel(t *testing.T) {
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme2(t)).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}
	c := sessCtx(t, http.MethodPost, `{"displayName":"P","source":{"accessMode":"local","url":"https://huggingface.co/x/y"}}`, "other-1", nil)
	_, err := h.createModel(c)
	if err == nil || getHTTPStatusCode(err) != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin public model create, got %v", err)
	}
}

// TestCreateModelDeniedForNonWorkspaceMember: creating in a workspace the user
// does not belong to must be denied.
func TestCreateModelDeniedForNonWorkspaceMember(t *testing.T) {
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme2(t)).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}
	c := sessCtx(t, http.MethodPost, `{"displayName":"P","workspace":"ws-1","source":{"accessMode":"local","url":"https://huggingface.co/x/y"}}`, "other-1", nil)
	_, err := h.createModel(c)
	if err == nil || getHTTPStatusCode(err) != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member workspace create, got %v", err)
	}
}

// TestCreateModelAllowedForWorkspaceMember: a workspace member may create a
// model in that workspace (authorization must not block it).
func TestCreateModelAllowedForWorkspaceMember(t *testing.T) {
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme2(t)).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}
	c := sessCtx(t, http.MethodPost, `{"displayName":"P","workspace":"ws-1","source":{"accessMode":"local","url":"https://huggingface.co/x/y"}}`, "wsmember-1", nil)
	_, err := h.createModel(c)
	if err != nil && getHTTPStatusCode(err) == http.StatusForbidden {
		t.Fatalf("workspace member create must not be forbidden, got %v", err)
	}
}

// TestPatchModelDeniedForNonOwner: patching another user's model must be denied.
func TestPatchModelDeniedForNonOwner(t *testing.T) {
	model := ownedModel("m-owned", "owner-1")
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme2(t)).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}
	c := sessCtx(t, http.MethodPatch, `{"displayName":"new"}`, "other-1", gin.Params{{Key: "id", Value: "m-owned"}})
	_, err := h.patchModel(c)
	if err == nil || getHTTPStatusCode(err) != http.StatusForbidden {
		t.Fatalf("expected 403 for non-owner patch, got %v", err)
	}
}

// TestPatchModelAllowedForOwner: the model owner may patch their own model.
func TestPatchModelAllowedForOwner(t *testing.T) {
	model := ownedModel("m-owned", "owner-1")
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme2(t)).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}
	c := sessCtx(t, http.MethodPatch, `{"displayName":"new"}`, "owner-1", gin.Params{{Key: "id", Value: "m-owned"}})
	if _, err := h.patchModel(c); err != nil {
		t.Fatalf("owner patch must succeed, got %v", err)
	}
}

// --- merged from models_read_rbac_endpoints_test.go ---

// TestGetModelWorkloadsReadRBAC verifies that getModelWorkloads enforces the
// same read visibility as getModel: users who cannot see a private model must
// not be able to enumerate its associated workloads.
func TestGetModelWorkloadsReadRBAC(t *testing.T) {
	h := newReadRBACHandler(t) // m-other lives in ws-2, owned by stranger-1
	cases := []struct {
		name    string
		user    string
		modelID string
		denied  bool
	}{
		{"member denied other workspace", "member-1", "m-other", true},
		{"owner allowed", "stranger-1", "m-other", false},
		{"admin allowed", "admin-1", "m-other", false},
		{"public visible to member", "member-1", "m-pub", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.getModelWorkloads(readRBACCtx(tc.user, "", gin.Params{{Key: "id", Value: tc.modelID}}))
			if tc.denied {
				testifyassert.Error(t, err)
				testifyassert.Contains(t, err.Error(), "not allowed")
			} else {
				testifyassert.NoError(t, err)
			}
		})
	}
}

// newLocalReadModel builds a Ready local model (deployable) owned by owner in
// the given workspace, with a ready local path so getWorkloadConfig can resolve
// a path once the visibility gate passes.
func newLocalReadModel(name, owner, workspace string) *v1.Model {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: name}}
	m.Labels = map[string]string{v1.UserIdLabel: owner}
	m.Spec.Workspace = workspace
	m.Spec.DisplayName = name
	m.Spec.Source.AccessMode = v1.AccessModeLocal
	m.Status.Phase = v1.ModelPhaseReady
	m.Status.LocalPaths = []v1.ModelLocalPath{{
		Workspace: workspace,
		Path:      "/apps/models/" + name,
		Status:    v1.LocalPathStatusReady,
	}}
	return m
}

// TestGetWorkloadConfigReadRBAC verifies getWorkloadConfig gates on read
// visibility before returning the on-disk model path / launch command.
func TestGetWorkloadConfigReadRBAC(t *testing.T) {
	// A private local model in ws-2 owned by stranger-1.
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).
		WithObjects(newLocalReadModel("lm-other", "stranger-1", "ws-2")).Build()
	h := &Handler{k8sClient: k8s, accessController: newReadRBACAC(t)}

	// member-1 (member of ws-1, not ws-2, not owner) must be denied before any
	// path is exposed.
	_, err := h.getWorkloadConfig(readRBACCtx("member-1", "workspace=ws-2", gin.Params{{Key: "id", Value: "lm-other"}}))
	testifyassert.Error(t, err)
	testifyassert.Contains(t, err.Error(), "not allowed")

	// admin passes the visibility gate (it must not fail with a 403).
	_, err = h.getWorkloadConfig(readRBACCtx("admin-1", "workspace=ws-2", gin.Params{{Key: "id", Value: "lm-other"}}))
	if err != nil {
		testifyassert.NotContains(t, err.Error(), "not allowed")
	}
}

// --- merged from models_read_rbac_test.go ---

// newReadRBACAC seeds users used to exercise read-path visibility:
//   - admin-1     : system administrator (sees everything)
//   - readonly-1  : system-admin-readonly (sees everything)
//   - owner-1     : plain user, owns some models, member of no workspace
//   - member-1    : plain user, member of workspace "ws-1"
//   - stranger-1  : plain user, owns some models, member of no workspace
//
// canViewModel does not consult role rules, so no Role objects are required.
func newReadRBACAC(t *testing.T) *authority.AccessController {
	t.Helper()
	admin := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
	}
	readonly := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "readonly-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminReadonlyRole}},
	}
	owner := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "owner-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType},
	}
	member := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "member-1"},
		Spec: v1.UserSpec{
			Type:      v1.DefaultUserType,
			Resources: map[string][]string{common.UserWorkspaces: {"ws-1"}},
		},
	}
	stranger := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "stranger-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType},
	}
	return &authority.AccessController{
		Client: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).
			WithObjects(admin, readonly, owner, member, stranger).Build(),
	}
}

// newReadModel builds a Ready model owned by owner in the given workspace. An
// empty workspace marks the model public (visible to everyone).
func newReadModel(name, owner, workspace string) *v1.Model {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: name}}
	m.Labels = map[string]string{v1.UserIdLabel: owner}
	m.Spec.Workspace = workspace
	m.Spec.Source.AccessMode = v1.AccessModeRemoteAPI
	m.Status.Phase = v1.ModelPhaseReady
	return m
}

func readRBACCtx(userID, rawQuery string, params gin.Params) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
	c.Params = params
	if userID != "" {
		c.Set(common.UserId, userID)
	}
	return c
}

func newReadRBACHandler(t *testing.T) *Handler {
	t.Helper()
	// dbClient is intentionally nil so listModels/getModel take the K8s path,
	// which lets these tests control owner/workspace precisely.
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithObjects(
		newReadModel("m-pub", "owner-1", ""),          // public
		newReadModel("m-own", "owner-1", "ws-1"),      // owned by owner-1, in ws-1
		newReadModel("m-wsonly", "nobody", "ws-1"),    // ws-1, owned by a non-participant
		newReadModel("m-other", "stranger-1", "ws-2"), // ws-2, owned by stranger-1
	).Build()
	return &Handler{k8sClient: k8s, accessController: newReadRBACAC(t)}
}

// TestGetModelReadRBAC verifies #9: getModel enforces resource-level read
// visibility and returns 403 for models the caller may not see.
func TestGetModelReadRBAC(t *testing.T) {
	h := newReadRBACHandler(t)
	cases := []struct {
		name    string
		user    string
		modelID string
		denied  bool
	}{
		{"public visible to stranger", "stranger-1", "m-pub", false},
		{"owner sees own private", "owner-1", "m-own", false},
		{"workspace member sees ws model", "member-1", "m-wsonly", false},
		{"owner sees own regardless of ws", "stranger-1", "m-other", false},
		{"stranger denied others private", "stranger-1", "m-own", true},
		{"member denied other workspace", "member-1", "m-other", true},
		{"admin sees any private", "admin-1", "m-other", false},
		{"readonly admin sees any private", "readonly-1", "m-own", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.getModel(readRBACCtx(tc.user, "", gin.Params{{Key: "id", Value: tc.modelID}}))
			if tc.denied {
				testifyassert.Error(t, err)
				testifyassert.Contains(t, err.Error(), "not allowed")
			} else {
				testifyassert.NoError(t, err)
			}
		})
	}
}

// TestListModelsReadRBAC verifies #9: listModels only returns models visible to
// the caller (public + owned + member workspaces; admins see all).
func TestListModelsReadRBAC(t *testing.T) {
	h := newReadRBACHandler(t)
	listIDs := func(user string) map[string]bool {
		res, err := h.listModels(readRBACCtx(user, "limit=100&offset=0", nil))
		testifyassert.NoError(t, err)
		resp, ok := res.(*ListModelResponse)
		testifyassert.True(t, ok)
		ids := make(map[string]bool, len(resp.Items))
		for _, it := range resp.Items {
			ids[it.ID] = true
		}
		return ids
	}

	cases := []struct {
		name string
		user string
		want []string
	}{
		{"stranger sees public + owned", "stranger-1", []string{"m-pub", "m-other"}},
		{"owner sees public + owned", "owner-1", []string{"m-pub", "m-own"}},
		{"member sees public + ws-1 models", "member-1", []string{"m-pub", "m-own", "m-wsonly"}},
		{"admin sees all", "admin-1", []string{"m-pub", "m-own", "m-wsonly", "m-other"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := listIDs(tc.user)
			assert.Equal(t, len(tc.want), len(got), "unexpected visible set: %v", got)
			for _, id := range tc.want {
				testifyassert.True(t, got[id], "expected %s visible to %s", id, tc.user)
			}
		})
	}
}

// TestListModelsReadRBACUnresolvedUserPublicOnly verifies fail-closed behavior:
// when the requesting user cannot be resolved (empty user id), only public
// models are returned.
func TestListModelsReadRBACUnresolvedUserPublicOnly(t *testing.T) {
	h := newReadRBACHandler(t)
	res, err := h.listModels(readRBACCtx("", "limit=100&offset=0", nil))
	testifyassert.NoError(t, err)
	resp := res.(*ListModelResponse)
	assert.Equal(t, int64(1), resp.Total)
	assert.Equal(t, "m-pub", resp.Items[0].ID)
}

// --- merged from model_create_dispatch_test.go ---

func dispatchCapableHandler(t *testing.T) *Handler {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	return newMockModelHandler(ctrlfake.NewClientBuilder().WithScheme(s).Build())
}

// TestCreateModelDispatchLocalPath verifies dispatch to the local_path flow succeeds.
func TestCreateModelDispatchLocalPath(t *testing.T) {
	h := dispatchCapableHandler(t)
	res, err := h.createModel(sessCtx(t, http.MethodPost,
		`{"displayName":"LP","source":{"accessMode":"local_path","localPath":"/wekafs/m"}}`, "u1", nil))
	require.NoError(t, err)
	testifyassert.NotNil(t, res)
}

// TestCreateModelDispatchS3Sync verifies dispatch to the s3_sync flow succeeds.
func TestCreateModelDispatchS3Sync(t *testing.T) {
	h := dispatchCapableHandler(t)
	res, err := h.createModel(sessCtx(t, http.MethodPost,
		`{"displayName":"S3","source":{"accessMode":"s3_sync"},"s3Source":{"uri":"s3://b/p"}}`, "u1", nil))
	require.NoError(t, err)
	testifyassert.NotNil(t, res)
}

// TestCreateModelRemoteAPISuccess verifies the remote_api flow creates a ready model and api key secret.
func TestCreateModelRemoteAPISuccess(t *testing.T) {
	h := dispatchCapableHandler(t)
	res, err := h.createModel(sessCtx(t, http.MethodPost,
		`{"displayName":"Remote","source":{"accessMode":"remote_api","url":"https://api.example.com","modelName":"gpt","apiKey":"k"}}`, "u1", nil))
	require.NoError(t, err)
	testifyassert.NotNil(t, res)
}

// --- merged from model_crud_test.go ---

// modelScheme returns a scheme with the project API types registered.
func modelScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// modelGinCtx builds a gin context with optional params and query.
func modelGinCtx(t *testing.T, params gin.Params, rawQuery string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	r := httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
	c.Request = r
	c.Params = params
	return c
}

func TestGetModelFromDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetModelByID(gomock.Any(), "m1").
		Return(&dbclient.Model{ID: "m1", AccessMode: "local", DisplayName: "My Model"}, nil)

	h := &Handler{dbClient: m, k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	res, err := h.getModel(modelGinCtx(t, gin.Params{{Key: "id", Value: "m1"}}, ""))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestGetModelEmptyID(t *testing.T) {
	h := &Handler{k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	_, err := h.getModel(modelGinCtx(t, nil, ""))
	testifyassert.Error(t, err)
}

func TestGetModelNotFound(t *testing.T) {
	// No db client, empty k8s -> not found.
	h := &Handler{k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	_, err := h.getModel(modelGinCtx(t, gin.Params{{Key: "id", Value: "missing"}}, ""))
	testifyassert.Error(t, err)
}

func TestListModelsFromDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListModels(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.Model{
			{ID: "m1", AccessMode: "local", DisplayName: "A"},
			{ID: "m2", AccessMode: "local", DisplayName: "B"},
		}, nil)

	h := &Handler{dbClient: m, k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	res, err := h.listModels(modelGinCtx(t, nil, ""))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestListModelsEmptyDBFallsBackToK8s(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListModels(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.Model{}, nil)

	h := &Handler{dbClient: m, k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	res, err := h.listModels(modelGinCtx(t, nil, ""))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

// --- merged from model_dup_test.go ---

// TestFindModelBySourceURLListError verifies S14: when the K8s List call fails,
// the duplicate check must surface the error instead of silently reporting
// "no duplicate", which could otherwise allow creating duplicate models.
func TestFindModelBySourceURLListError(t *testing.T) {
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	failing := ctrlfake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
		List: func(_ context.Context, _ ctrlclient.WithWatch, _ ctrlclient.ObjectList, _ ...ctrlclient.ListOption) error {
			return errors.New("api server down")
		},
	}).Build()

	h := newMockModelHandler(failing)
	if _, err := h.findModelBySourceURL(context.Background(), "https://huggingface.co/x/y", "ws1"); err == nil {
		t.Fatal("expected error when List fails, got nil")
	}
}

// TestCreateModelFromS3SyncListError verifies S14 also covers the s3_sync path:
// when the duplicate-check List fails, s3_sync creation must surface the error
// instead of silently skipping dedup (which could create duplicate models).
func TestCreateModelFromS3SyncListError(t *testing.T) {
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	failing := ctrlfake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
		List: func(_ context.Context, _ ctrlclient.WithWatch, _ ctrlclient.ObjectList, _ ...ctrlclient.ListOption) error {
			return errors.New("api server down")
		},
	}).Build()

	h := &Handler{k8sClient: failing, accessController: adminModelAC()}
	c := sessCtx(t, http.MethodPost, `{"displayName":"S3","source":{"accessMode":"s3_sync"},"s3Source":{"uri":"s3://b/p"}}`, adminModelUserID, nil)
	if _, err := h.createModel(c); err == nil {
		t.Fatal("expected error when List fails during s3_sync duplicate check, got nil")
	}
}

// --- merged from model_localpath_test.go ---

// TestCreateModelFromLocalPathSuccess verifies a Ready model CR is created from a local path.
func TestCreateModelFromLocalPathSuccess(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()
	h := newMockModelHandler(cl)

	req := &CreateModelRequest{
		DisplayName: "My Model",
		Source: ModelSourceReq{
			AccessMode: "local_path",
			LocalPath:  "/wekafs/models/my-model",
		},
	}
	res, err := h.createModelFromLocalPath(context.Background(), req, "uid-1", "user-1")
	require.NoError(t, err)
	resp, ok := res.(*CreateResponse)
	require.True(t, ok)
	testifyassert.NotEmpty(t, resp.ID)

	created := &v1.Model{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: resp.ID}, created))
	assert.Equal(t, v1.AccessModeLocalPath, created.Spec.Source.AccessMode)
	assert.Equal(t, v1.ModelPhaseReady, created.Status.Phase)
	assert.Equal(t, "external", created.Spec.Origin)
}

// TestCreateModelFromLocalPathFineTuned verifies origin defaults to fine_tuned when sftJobId is set.
func TestCreateModelFromLocalPathFineTuned(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()
	h := newMockModelHandler(cl)

	req := &CreateModelRequest{
		DisplayName: "Tuned Model",
		SftJobId:    "sft-123",
		Source: ModelSourceReq{
			AccessMode: "local_path",
			LocalPath:  "/wekafs/models/tuned",
		},
	}
	res, err := h.createModelFromLocalPath(context.Background(), req, "uid-1", "user-1")
	require.NoError(t, err)
	resp := res.(*CreateResponse)

	created := &v1.Model{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: resp.ID}, created))
	assert.Equal(t, "fine_tuned", created.Spec.Origin)
	assert.Equal(t, "sft-123", created.Spec.SftJobId)
}

// TestCreateModelFromLocalPathMissingLocalPath verifies validation of the local path.
func TestCreateModelFromLocalPathMissingLocalPath(t *testing.T) {
	h := newMockModelHandler(ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build())
	req := &CreateModelRequest{DisplayName: "M"}
	_, err := h.createModelFromLocalPath(context.Background(), req, "", "")
	testifyassert.Error(t, err)
}

// TestCreateModelFromLocalPathMissingDisplayName verifies validation of the display name.
func TestCreateModelFromLocalPathMissingDisplayName(t *testing.T) {
	h := newMockModelHandler(ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build())
	req := &CreateModelRequest{Source: ModelSourceReq{LocalPath: "/x"}}
	_, err := h.createModelFromLocalPath(context.Background(), req, "", "")
	testifyassert.Error(t, err)
}

// --- merged from model_more_test.go ---

func failedModelClient(t *testing.T, phase v1.ModelPhase) *Handler {
	t.Helper()
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	model.Status.Phase = phase
	cl := ctrlfake.NewClientBuilder().
		WithScheme(modelScheme(t)).
		WithObjects(model).
		WithStatusSubresource(&v1.Model{}).
		Build()
	return &Handler{k8sClient: cl, accessController: adminModelAC()}
}

func TestRetryModelHandler(t *testing.T) {
	h := failedModelClient(t, v1.ModelPhaseFailed)
	res, err := h.retryModel(sessCtx(t, http.MethodPost, "", adminModelUserID, gin.Params{{Key: "id", Value: "m1"}}))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestRetryModelHandlerNotFailed(t *testing.T) {
	h := failedModelClient(t, v1.ModelPhaseReady)
	_, err := h.retryModel(sessCtx(t, http.MethodPost, "", adminModelUserID, gin.Params{{Key: "id", Value: "m1"}}))
	testifyassert.Error(t, err)
}

func TestRetryModelHandlerNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithStatusSubresource(&v1.Model{}).Build()
	h := &Handler{k8sClient: cl}
	_, err := h.retryModel(sessCtx(t, http.MethodPost, "", "", gin.Params{{Key: "id", Value: "missing"}}))
	testifyassert.Error(t, err)
}

func TestPatchModelHandler(t *testing.T) {
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithObjects(model).Build()
	h := &Handler{k8sClient: cl, accessController: adminModelAC()}
	res, err := h.patchModel(sessCtx(t, http.MethodPatch, `{"displayName":"new"}`, adminModelUserID, gin.Params{{Key: "id", Value: "m1"}}))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestPatchModelHandlerNoFields(t *testing.T) {
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithObjects(model).Build()
	h := &Handler{k8sClient: cl}
	_, err := h.patchModel(sessCtx(t, http.MethodPatch, `{}`, "", gin.Params{{Key: "id", Value: "m1"}}))
	testifyassert.Error(t, err)
}

func TestPatchModelHandlerBadID(t *testing.T) {
	h := &Handler{}
	_, err := h.patchModel(sessCtx(t, http.MethodPatch, `{"displayName":"x"}`, "", nil))
	testifyassert.Error(t, err)
}

func TestGetModelWorkloadsHandler(t *testing.T) {
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithObjects(model).Build()
	h := &Handler{k8sClient: cl}
	res, err := h.getModelWorkloads(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "m1"}}))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestGetModelWorkloadsHandlerNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()
	h := &Handler{k8sClient: cl}
	_, err := h.getModelWorkloads(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "missing"}}))
	testifyassert.Error(t, err)
}

// --- merged from model_s3sync_test.go ---

func s3SyncHandler(t *testing.T) (*Handler, ctrlclient.Client) {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	cl := ctrlfake.NewClientBuilder().WithScheme(s).Build()
	return newMockModelHandler(cl), cl
}

// TestCreateModelFromS3SyncValidation covers the request validation branches.
func TestCreateModelFromS3SyncValidation(t *testing.T) {
	h, _ := s3SyncHandler(t)
	ctx := context.Background()

	cases := []struct {
		name string
		req  *CreateModelRequest
	}{
		{"missing s3 source", &CreateModelRequest{DisplayName: "M"}},
		{"missing display name", &CreateModelRequest{S3Source: &S3SourceReq{URI: "s3://b/p"}}},
		{"not s3 scheme", &CreateModelRequest{DisplayName: "M", S3Source: &S3SourceReq{URI: "http://b/p"}}},
		{"empty bucket", &CreateModelRequest{DisplayName: "M", S3Source: &S3SourceReq{URI: "s3:///p"}}},
		{"unsafe uri", &CreateModelRequest{DisplayName: "M", S3Source: &S3SourceReq{URI: "s3://b/$(x)"}}},
		{"ak without sk", &CreateModelRequest{DisplayName: "M", S3Source: &S3SourceReq{URI: "s3://b/p", AccessKeyID: "ak"}}},
		{"creds without endpoint", &CreateModelRequest{DisplayName: "M", S3Source: &S3SourceReq{URI: "s3://b/p", AccessKeyID: "ak", SecretAccessKey: "sk"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.createModelFromS3Sync(ctx, tc.req, "", "")
			testifyassert.Error(t, err)
		})
	}
}

// TestCreateModelFromS3SyncSuccessNoCreds verifies a pending model is created without credentials.
func TestCreateModelFromS3SyncSuccessNoCreds(t *testing.T) {
	h, cl := s3SyncHandler(t)
	req := &CreateModelRequest{
		DisplayName: "S3 Model",
		S3Source:    &S3SourceReq{URI: "s3://my-bucket/prefix"},
	}
	res, err := h.createModelFromS3Sync(context.Background(), req, "uid", "uname")
	require.NoError(t, err)
	resp := res.(*CreateResponse)

	created := &v1.Model{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: resp.ID}, created))
	assert.Equal(t, v1.ModelPhasePending, created.Status.Phase)
	assert.Equal(t, v1.TrueStr, created.Labels[v1.ModelS3ImportLabel])
}

// TestCreateModelFromS3SyncSuccessWithCreds verifies the source secret is created with credentials.
func TestCreateModelFromS3SyncSuccessWithCreds(t *testing.T) {
	h, cl := s3SyncHandler(t)
	req := &CreateModelRequest{
		DisplayName: "S3 Model Creds",
		S3Source: &S3SourceReq{
			URI:             "s3://my-bucket/prefix",
			AccessKeyID:     "ak",
			SecretAccessKey: "sk",
			Endpoint:        "https://s3.us-west-2.amazonaws.com",
			Region:          "us-west-2",
		},
	}
	res, err := h.createModelFromS3Sync(context.Background(), req, "uid", "uname")
	require.NoError(t, err)
	resp := res.(*CreateResponse)

	created := &v1.Model{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: resp.ID}, created))
	secretName := created.Annotations[v1.ModelS3SourceSecretAnn]
	testifyassert.NotEmpty(t, secretName)
}

// TestCreateModelFromS3SyncDuplicate verifies an existing model with the same source is rejected.
func TestCreateModelFromS3SyncDuplicate(t *testing.T) {
	h, cl := s3SyncHandler(t)
	existing := &v1.Model{}
	existing.Name = "existing"
	existing.Spec.Source.URL = "s3://my-bucket/prefix"
	existing.Spec.Source.AccessMode = v1.AccessModeLocal
	require.NoError(t, cl.Create(context.Background(), existing))

	req := &CreateModelRequest{
		DisplayName: "Dup",
		S3Source:    &S3SourceReq{URI: "s3://my-bucket/prefix"},
	}
	_, err := h.createModelFromS3Sync(context.Background(), req, "", "")
	testifyassert.Error(t, err)
}
