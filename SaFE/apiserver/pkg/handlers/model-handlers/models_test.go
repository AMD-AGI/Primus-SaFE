/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newMockModelHandler creates a mock Handler for testing
func newMockModelHandler(k8sClient client.Client) *Handler {
	return &Handler{
		k8sClient: k8sClient,
		dbClient:  nil, // No database client for unit tests
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

		result, err := h.listModels(c)
		assert.NilError(t, err)

		resp := result.(*ListModelResponse)
		assert.Equal(t, resp.Total, int64(3))
	})

	t.Run("Filter by accessMode", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/models?accessMode=remote_api", nil)

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

// TestConvertK8sModelToInfo tests the convertK8sModelToInfo function
func TestConvertK8sModelToInfo(t *testing.T) {
	model := genMockLocalK8sModel("test-model", "ws1")
	model.Spec.Tags = []string{"llm", "text-generation", "english"}
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{
			Workspace: "ws1",
			Path:      "/apps/models/test",
			Status:    v1.LocalPathStatusReady,
			Message:   "Download completed",
		},
	}

	h := newMockModelHandler(nil)
	info := h.convertK8sModelToInfo(model)

	assert.Equal(t, info.ID, "test-model")
	assert.Equal(t, info.DisplayName, model.Spec.DisplayName)
	assert.Equal(t, info.Description, model.Spec.Description)
	assert.Equal(t, info.AccessMode, string(v1.AccessModeLocal))
	assert.Equal(t, info.Workspace, "ws1")
	assert.Equal(t, len(info.LocalPaths), 1)
	assert.Equal(t, info.LocalPaths[0].Workspace, "ws1")
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
			expectedLimit:  10,
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

