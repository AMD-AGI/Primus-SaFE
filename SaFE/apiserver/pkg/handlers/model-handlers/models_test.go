/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestCreateModelRequest_Validation tests the CreateModelRequest validation
func TestCreateModelRequest_Validation(t *testing.T) {
	tests := []struct {
		name      string
		req       CreateModelRequest
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid local model request",
			req: CreateModelRequest{
				Source: ModelSourceReq{
					URL:        "https://huggingface.co/gpt2",
					AccessMode: string(v1.AccessModeLocal),
					Token:      "test-token",
				},
			},
			expectErr: false,
		},
		{
			name: "valid remote_api model request",
			req: CreateModelRequest{
				DisplayName: "Test Model",
				Description: "A test model",
				Label:       "TestOrg",
				Source: ModelSourceReq{
					URL:        "https://api.openai.com/v1",
					AccessMode: string(v1.AccessModeRemoteAPI),
					Token:      "sk-test",
				},
			},
			expectErr: false,
		},
		{
			name: "missing URL",
			req: CreateModelRequest{
				Source: ModelSourceReq{
					URL:        "",
					AccessMode: string(v1.AccessModeLocal),
				},
			},
			expectErr: true,
			errMsg:    "url is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectErr {
				if tt.req.Source.URL == "" {
					assert.Empty(t, tt.req.Source.URL, "URL should be empty")
				}
			} else {
				assert.NotEmpty(t, tt.req.Source.URL, "URL should not be empty")
				assert.NotEmpty(t, tt.req.Source.AccessMode, "AccessMode should not be empty")
			}
		})
	}
}

// TestModelSourceReq tests the ModelSourceReq struct
func TestModelSourceReq(t *testing.T) {
	source := ModelSourceReq{
		URL:        "https://huggingface.co/meta-llama/Llama-2-7b-hf",
		AccessMode: string(v1.AccessModeLocal),
		Token:      "hf_test_token",
	}

	assert.Equal(t, "https://huggingface.co/meta-llama/Llama-2-7b-hf", source.URL)
	assert.Equal(t, string(v1.AccessModeLocal), source.AccessMode)
	assert.Equal(t, "hf_test_token", source.Token)
}

// TestResourceReq tests the ResourceReq struct
func TestResourceReq(t *testing.T) {
	resources := ResourceReq{
		CPU:    "4",
		Memory: "16Gi",
		GPU:    "nvidia-tesla-v100",
	}

	assert.Equal(t, "4", resources.CPU)
	assert.Equal(t, "16Gi", resources.Memory)
	assert.Equal(t, "nvidia-tesla-v100", resources.GPU)
}

// TestCreateResponse tests the CreateResponse struct
func TestCreateResponse(t *testing.T) {
	resp := CreateResponse{
		ID: "model-12345",
	}

	assert.Equal(t, "model-12345", resp.ID)

	// Test JSON marshaling
	jsonData, err := json.Marshal(resp)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonData), "model-12345")
}

// TestListModelQuery tests the ListModelQuery struct
func TestListModelQuery(t *testing.T) {
	query := ListModelQuery{
		Limit:           20,
		Offset:          10,
		InferenceStatus: "Running",
		AccessMode:      string(v1.AccessModeLocal),
	}

	assert.Equal(t, 20, query.Limit)
	assert.Equal(t, 10, query.Offset)
	assert.Equal(t, "Running", query.InferenceStatus)
	assert.Equal(t, string(v1.AccessModeLocal), query.AccessMode)
}

// TestListModelQuery_DefaultValues tests default values
func TestListModelQuery_DefaultValues(t *testing.T) {
	query := &ListModelQuery{}

	// Before applying defaults
	assert.Equal(t, 0, query.Limit)
	assert.Equal(t, 0, query.Offset)
	assert.Equal(t, "", query.InferenceStatus)
	assert.Equal(t, "", query.AccessMode)
}

// TestListModelResponse tests the ListModelResponse struct
func TestListModelResponse(t *testing.T) {
	resp := ListModelResponse{
		Total: 100,
		Items: []ModelInfo{
			{
				ID:          "model-001",
				DisplayName: "GPT-2",
			},
			{
				ID:          "model-002",
				DisplayName: "Llama-2",
			},
		},
	}

	assert.Equal(t, int64(100), resp.Total)
	assert.Len(t, resp.Items, 2)
}

// TestToggleModelRequest tests the ToggleModelRequest struct
func TestToggleModelRequest(t *testing.T) {
	// Test enable
	enableReq := ToggleModelRequest{
		Enabled: true,
	}
	assert.True(t, enableReq.Enabled)

	// Test disable
	disableReq := ToggleModelRequest{
		Enabled: false,
	}
	assert.False(t, disableReq.Enabled)
}

// TestParseListModelQuery tests the parseListModelQuery helper
func TestParseListModelQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		queryParams   map[string]string
		expectedLimit int
		expectedError bool
	}{
		{
			name: "valid query with all params",
			queryParams: map[string]string{
				"limit":           "20",
				"offset":          "10",
				"inferenceStatus": "Running",
				"accessMode":      "local",
			},
			expectedLimit: 20,
			expectedError: false,
		},
		{
			name:          "empty query - should use defaults",
			queryParams:   map[string]string{},
			expectedLimit: 10, // Default limit
			expectedError: false,
		},
		{
			name: "only limit param",
			queryParams: map[string]string{
				"limit": "50",
			},
			expectedLimit: 50,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock request
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Build query string
			queryString := ""
			for k, v := range tt.queryParams {
				if queryString != "" {
					queryString += "&"
				}
				queryString += k + "=" + v
			}

			req := httptest.NewRequest("GET", "/models?"+queryString, nil)
			c.Request = req

			result, err := parseListModelQuery(c)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedLimit, result.Limit)
			}
		})
	}
}

// TestAccessModeConstants tests that we're using the correct constants
func TestAccessModeConstants(t *testing.T) {
	// Verify AccessMode constants are correct
	assert.Equal(t, "local", string(v1.AccessModeLocal))
	assert.Equal(t, "remote_api", string(v1.AccessModeRemoteAPI))
}

// TestCreateModel_MockK8sClient tests createModel with a mock Kubernetes client
func TestCreateModel_MockK8sClient(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create scheme and add our types
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create fake k8s client
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	handler := &Handler{
		k8sClient: k8sClient,
	}

	// Test request for remote_api mode
	reqBody := CreateModelRequest{
		DisplayName: "Test API Model",
		Description: "A test model for API",
		Label:       "TestOrg",
		Tags:        []string{"test", "api"},
		Source: ModelSourceReq{
			URL:        "https://api.test.com/v1",
			AccessMode: string(v1.AccessModeRemoteAPI),
			Token:      "test-token",
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	// Call createModel directly
	result, err := handler.createModel(c)

	// Should succeed
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Verify the response contains an ID
	resp, ok := result.(*CreateResponse)
	assert.True(t, ok)
	assert.NotEmpty(t, resp.ID)

	// Verify model was created in k8s
	var modelList v1.ModelList
	err = k8sClient.List(context.Background(), &modelList, &client.ListOptions{
		Namespace: common.PrimusSafeNamespace,
	})
	assert.NoError(t, err)
	assert.Len(t, modelList.Items, 1)

	// Verify model properties
	createdModel := modelList.Items[0]
	assert.Equal(t, "Test API Model", createdModel.Spec.DisplayName)
	assert.Equal(t, "A test model for API", createdModel.Spec.Description)
	assert.Equal(t, "TestOrg", createdModel.Spec.Label)
	assert.Equal(t, v1.AccessModeRemoteAPI, createdModel.Spec.Source.AccessMode)
}

// TestCreateModel_ValidationErrors tests validation error cases
func TestCreateModel_ValidationErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		reqBody     CreateModelRequest
		expectedErr string
	}{
		{
			name: "missing URL for local mode",
			reqBody: CreateModelRequest{
				Source: ModelSourceReq{
					URL:        "",
					AccessMode: string(v1.AccessModeLocal),
				},
			},
			expectedErr: "url is required for local mode",
		},
		{
			name: "invalid AccessMode",
			reqBody: CreateModelRequest{
				Source: ModelSourceReq{
					URL:        "https://test.com",
					AccessMode: "invalid_mode",
				},
			},
			expectedErr: "accessMode must be",
		},
		{
			name: "remote_api missing displayName",
			reqBody: CreateModelRequest{
				DisplayName: "",
				Source: ModelSourceReq{
					URL:        "https://api.test.com",
					AccessMode: string(v1.AccessModeRemoteAPI),
				},
			},
			expectedErr: "displayName is required",
		},
		// Note: label and description are now optional for remote_api mode
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = v1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			handler := &Handler{
				k8sClient: k8sClient,
			}

			bodyBytes, _ := json.Marshal(tt.reqBody)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/models", bytes.NewBuffer(bodyBytes))
			c.Request.Header.Set("Content-Type", "application/json")

			result, err := handler.createModel(c)

			// Should fail
			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

// TestToggleModel_Enable tests enabling a local model (starting inference)
func TestToggleModel_Enable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create a test LOCAL model (remote_api models create inference at creation time)
	// Note: Model is cluster-scoped, no namespace needed
	testModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test Model",
			Description: "A test model",
			Source: v1.ModelSource{
				URL:        "https://huggingface.co/test/model",
				AccessMode: v1.AccessModeLocal, // Use local mode for toggle test
			},
		},
		Status: v1.ModelStatus{
			Phase:       v1.ModelPhaseReady,
			InferenceID: "", // No inference running
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testModel).
		WithStatusSubresource(&v1.Model{}).
		Build()

	handler := &Handler{
		k8sClient: k8sClient,
	}

	// Create request to enable with required resource and config for local models
	reqBody := ToggleModelRequest{
		Enabled: true,
		Resource: &ToggleResourceReq{
			Workspace: "test-workspace",
			Replica:   1,
			CPU:       4,
			Memory:    16,
			GPU:       "1",
		},
		Config: &ToggleConfigReq{
			Image:      "vllm/vllm-openai:latest",
			EntryPoint: "dmxsbSBzZXJ2ZSAvYXBwcy9tb2RlbHMvdGVzdA==", // base64 encoded
			ModelPath:  "/apps/models/test",
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models/test-model/toggle", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "test-model"}}

	// Set user context
	c.Set(common.UserId, "test-user")
	c.Set(common.UserName, "Test User")

	result, err := handler.toggleModel(c)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify response
	respMap, ok := result.(gin.H)
	require.True(t, ok)
	assert.Contains(t, respMap, "inferenceId")
	assert.Equal(t, "inference started", respMap["message"])

	// Verify inference was created
	var inferenceList v1.InferenceList
	err = k8sClient.List(context.Background(), &inferenceList)
	require.NoError(t, err)
	assert.Len(t, inferenceList.Items, 1)

	// Verify inference properties
	inference := inferenceList.Items[0]
	assert.Equal(t, "Test Model", inference.Spec.DisplayName)
	assert.Equal(t, "test-user", inference.Spec.UserID)
}

// TestToggleModel_RemoteAPI tests toggle for remote API models
func TestToggleModel_RemoteAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create a remote API model with existing inference
	// Note: Model is cluster-scoped, no namespace needed
	testModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-api-model",
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test API Model",
			Description: "A test API model",
			Source: v1.ModelSource{
				URL:        "https://api.test.com",
				AccessMode: v1.AccessModeRemoteAPI,
			},
		},
		Status: v1.ModelStatus{
			Phase:          v1.ModelPhaseReady,
			InferenceID:    "existing-inference", // Already has inference
			InferencePhase: "Running",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testModel).
		WithStatusSubresource(&v1.Model{}).
		Build()

	handler := &Handler{
		k8sClient: k8sClient,
	}

	// Try to toggle ON a remote API model that already has inference
	reqBody := ToggleModelRequest{
		Enabled: true,
		Instance: &ToggleInstanceReq{
			ApiKey: "sk-test",
			Model:  "gpt-3.5-turbo",
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models/test-api-model/toggle", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "test-api-model"}}

	c.Set(common.UserId, "test-user")
	c.Set(common.UserName, "Test User")

	result, err := handler.toggleModel(c)

	// Should fail because inference already exists
	require.Error(t, err)
	require.Nil(t, result)
	assert.Contains(t, err.Error(), "inference already exists")
}

// TestToggleModel_Disable tests disabling a model (stopping inference)
func TestToggleModel_Disable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create a test inference
	testInference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inference",
		},
		Spec: v1.InferenceSpec{
			DisplayName: "Test Inference",
			UserID:      "test-user",
		},
	}

	// Create a test model with inference running
	testModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
			// Note: Model is cluster-scoped, no namespace needed
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test Model",
		},
		Status: v1.ModelStatus{
			Phase:       v1.ModelPhaseReady,
			InferenceID: "test-inference",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testModel, testInference).
		WithStatusSubresource(&v1.Model{}).
		Build()

	handler := &Handler{
		k8sClient: k8sClient,
	}

	// Create request to disable
	reqBody := ToggleModelRequest{
		Enabled: false,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models/test-model/toggle", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "test-model"}}

	// Set user context
	c.Set(common.UserId, "test-user")
	c.Set(common.UserName, "Test User")

	result, err := handler.toggleModel(c)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify response
	respMap, ok := result.(gin.H)
	require.True(t, ok)
	assert.Equal(t, "inference stopped", respMap["message"])

	// Verify inference was deleted
	var inferenceList v1.InferenceList
	err = k8sClient.List(context.Background(), &inferenceList)
	require.NoError(t, err)
	assert.Len(t, inferenceList.Items, 0)
}

// TestDeleteModel tests model deletion
func TestDeleteModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create a test secret
	testSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model-secret",
			Namespace: common.PrimusSafeNamespace,
		},
		Data: map[string][]byte{
			"token": []byte("test-token"),
		},
	}

	// Create a test model with secret reference
	testModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
			// Note: Model is cluster-scoped, no namespace needed
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test Model",
			Source: v1.ModelSource{
				URL: "https://test.com",
				Token: &corev1.LocalObjectReference{
					Name: "test-model-secret",
				},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testModel, testSecret).
		Build()

	handler := &Handler{
		k8sClient: k8sClient,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/models/test-model", nil)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "test-model"}}

	result, err := handler.deleteModel(c)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify response
	respMap, ok := result.(gin.H)
	require.True(t, ok)
	assert.Equal(t, "model deleted successfully", respMap["message"])
	assert.Equal(t, "test-model", respMap["id"])

	// Verify model was deleted
	var modelList v1.ModelList
	err = k8sClient.List(context.Background(), &modelList, &client.ListOptions{
		Namespace: common.PrimusSafeNamespace,
	})
	require.NoError(t, err)
	assert.Len(t, modelList.Items, 0)

	// Verify secret was deleted
	var secretList corev1.SecretList
	err = k8sClient.List(context.Background(), &secretList, &client.ListOptions{
		Namespace: common.PrimusSafeNamespace,
	})
	require.NoError(t, err)
	assert.Len(t, secretList.Items, 0)
}

// TestCreateModelRequest_JSON tests JSON marshaling
func TestCreateModelRequest_JSON(t *testing.T) {
	req := CreateModelRequest{
		DisplayName: "GPT-2",
		Description: "OpenAI GPT-2 model",
		Icon:        "https://example.com/icon.png",
		Label:       "OpenAI",
		Tags:        []string{"nlp", "transformer"},
		Source: ModelSourceReq{
			URL:        "https://huggingface.co/gpt2",
			AccessMode: string(v1.AccessModeLocal),
			Token:      "hf_token",
		},
		Resources: &ResourceReq{
			CPU:    "4",
			Memory: "16Gi",
			GPU:    "nvidia-v100",
		},
	}

	// Test marshaling
	jsonData, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "GPT-2")
	assert.Contains(t, string(jsonData), "gpt2")

	// Test unmarshaling
	var unmarshaled CreateModelRequest
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, req.DisplayName, unmarshaled.DisplayName)
	assert.Equal(t, req.Source.URL, unmarshaled.Source.URL)
	assert.Equal(t, req.Resources.GPU, unmarshaled.Resources.GPU)
}

// TestIsFullURL tests the isFullURL helper function
func TestIsFullURL(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"https://huggingface.co/gpt2", true},
		{"http://example.com/model", true},
		{"https://api.openai.com/v1", true},
		{"microsoft/phi-2", false},
		{"gpt2", false},
		{"", false},
		{"http://", false},
		{"https:/", false},
		{"ftp://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isFullURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetModel_NotFound tests getModel when model doesn't exist
func TestGetModel_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	handler := &Handler{
		k8sClient: k8sClient,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/models/non-existent-model", nil)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "non-existent-model"}}

	result, err := handler.getModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not found")
}

// TestGetModel_MissingID tests getModel without model ID
func TestGetModel_MissingID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/models/", nil)
	c.Params = gin.Params{gin.Param{Key: "id", Value: ""}}

	result, err := handler.getModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "model id is required")
}

// TestGetModel_K8sFallback tests getModel using K8s API fallback
func TestGetModel_K8sFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	testModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test Model",
			Description: "A test model",
			Icon:        "https://example.com/icon.png",
			Label:       "TestOrg",
			Tags:        []string{"test"},
			MaxTokens:   4096,
			Source: v1.ModelSource{
				URL:        "https://huggingface.co/test/model",
				AccessMode: v1.AccessModeLocal,
			},
		},
		Status: v1.ModelStatus{
			Phase: v1.ModelPhaseReady,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testModel).
		Build()

	handler := &Handler{
		k8sClient: k8sClient,
		dbClient:  nil, // No DB client, will use K8s fallback
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/models/test-model", nil)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "test-model"}}

	result, err := handler.getModel(c)

	require.NoError(t, err)
	require.NotNil(t, result)

	respMap, ok := result.(gin.H)
	require.True(t, ok)
	assert.Equal(t, "test-model", respMap["id"])
	assert.Equal(t, "Test Model", respMap["displayName"])
}

// TestListModels_K8sFallback tests listModels using K8s API fallback
func TestListModels_K8sFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	testModels := []v1.Model{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "model-1"},
			Spec: v1.ModelSpec{
				DisplayName: "Model 1",
				Source:      v1.ModelSource{AccessMode: v1.AccessModeLocal},
			},
			Status: v1.ModelStatus{Phase: v1.ModelPhaseReady},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "model-2"},
			Spec: v1.ModelSpec{
				DisplayName: "Model 2",
				Source:      v1.ModelSource{AccessMode: v1.AccessModeRemoteAPI},
			},
			Status: v1.ModelStatus{Phase: v1.ModelPhaseReady},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithLists(&v1.ModelList{Items: testModels}).
		Build()

	handler := &Handler{
		k8sClient: k8sClient,
		dbClient:  nil,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/models?limit=10&offset=0", nil)

	result, err := handler.listModels(c)

	require.NoError(t, err)
	require.NotNil(t, result)

	respMap, ok := result.(gin.H)
	require.True(t, ok)
	assert.Equal(t, int64(2), respMap["total"])
}

// TestListModels_WithAccessModeFilter tests listModels with access mode filter
func TestListModels_WithAccessModeFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	testModels := []v1.Model{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "model-local"},
			Spec: v1.ModelSpec{
				DisplayName: "Local Model",
				Source:      v1.ModelSource{AccessMode: v1.AccessModeLocal},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "model-api"},
			Spec: v1.ModelSpec{
				DisplayName: "API Model",
				Source:      v1.ModelSource{AccessMode: v1.AccessModeRemoteAPI},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithLists(&v1.ModelList{Items: testModels}).
		Build()

	handler := &Handler{
		k8sClient: k8sClient,
		dbClient:  nil,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/models?accessMode=local", nil)

	result, err := handler.listModels(c)

	require.NoError(t, err)
	require.NotNil(t, result)

	respMap, ok := result.(gin.H)
	require.True(t, ok)
	assert.Equal(t, int64(1), respMap["total"])
}

// TestConvertK8sModelToResponse tests the K8s model conversion
func TestConvertK8sModelToResponse(t *testing.T) {
	handler := &Handler{}

	k8sModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-model",
			CreationTimestamp: metav1.Now(),
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test Model",
			Description: "A test description",
			Icon:        "https://example.com/icon.png",
			Label:       "TestOrg",
			Tags:        []string{"test", "model"},
			MaxTokens:   8192,
			Source: v1.ModelSource{
				URL:        "https://huggingface.co/test/model",
				AccessMode: v1.AccessModeLocal,
			},
		},
		Status: v1.ModelStatus{
			Phase:          v1.ModelPhaseReady,
			InferenceID:    "inf-123",
			InferencePhase: "Running",
		},
	}

	result := handler.convertK8sModelToResponse(k8sModel)

	assert.Equal(t, "test-model", result["id"])
	assert.Equal(t, "Test Model", result["displayName"])
	assert.Equal(t, "A test description", result["description"])
	assert.Equal(t, "https://example.com/icon.png", result["icon"])
	assert.Equal(t, "TestOrg", result["label"])
	assert.Equal(t, []string{"test", "model"}, result["tags"])
	assert.Equal(t, 8192, result["maxTokens"])
	assert.Equal(t, v1.AccessModeLocal, result["accessMode"])
	assert.Equal(t, "https://huggingface.co/test/model", result["url"])
	assert.Equal(t, v1.ModelPhaseReady, result["phase"])
	assert.Equal(t, "inf-123", result["inferenceId"])
	assert.Equal(t, "Running", result["inferencePhase"])
}

// TestToggleModel_MissingID tests toggleModel without model ID
func TestToggleModel_MissingID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{}

	reqBody := ToggleModelRequest{Enabled: true}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models//toggle", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{gin.Param{Key: "id", Value: ""}}

	result, err := handler.toggleModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "model id is required")
}

// TestToggleModel_NotAuthenticated tests toggleModel without user authentication
func TestToggleModel_NotAuthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	testModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{Name: "test-model"},
		Spec:       v1.ModelSpec{DisplayName: "Test"},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testModel).
		Build()

	handler := &Handler{k8sClient: k8sClient}

	reqBody := ToggleModelRequest{Enabled: true}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models/test-model/toggle", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "test-model"}}
	// Not setting UserId

	result, err := handler.toggleModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "user not authenticated")
}

// TestToggleModel_ModelNotFound tests toggleModel when model doesn't exist
func TestToggleModel_ModelNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	handler := &Handler{k8sClient: k8sClient}

	reqBody := ToggleModelRequest{Enabled: true}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models/non-existent/toggle", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "non-existent"}}
	c.Set(common.UserId, "test-user")
	c.Set(common.UserName, "Test User")

	result, err := handler.toggleModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not found")
}

// TestToggleModel_DisableNoInference tests toggle OFF when no inference exists
func TestToggleModel_DisableNoInference(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	// Note: Model is cluster-scoped, no namespace needed
	testModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Spec: v1.ModelSpec{DisplayName: "Test"},
		Status: v1.ModelStatus{
			InferenceID: "", // No inference
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testModel).
		WithStatusSubresource(&v1.Model{}).
		Build()

	handler := &Handler{k8sClient: k8sClient}

	reqBody := ToggleModelRequest{Enabled: false}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models/test-model/toggle", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "test-model"}}
	c.Set(common.UserId, "test-user")
	c.Set(common.UserName, "Test User")

	result, err := handler.toggleModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "inference not found or already stopped")
}

// TestToggleModel_LocalMissingResource tests toggle ON local model without resource
func TestToggleModel_LocalMissingResource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	// Note: Model is cluster-scoped, no namespace needed
	testModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test",
			Source:      v1.ModelSource{AccessMode: v1.AccessModeLocal},
		},
		Status: v1.ModelStatus{InferenceID: ""},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testModel).
		Build()

	handler := &Handler{k8sClient: k8sClient}

	reqBody := ToggleModelRequest{
		Enabled:  true,
		Resource: nil, // Missing resource
		Config:   nil, // Missing config
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models/test-model/toggle", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "test-model"}}
	c.Set(common.UserId, "test-user")
	c.Set(common.UserName, "Test User")

	result, err := handler.toggleModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "resource and config are required")
}

// TestToggleModel_RemoteAPIMissingApiKey tests toggle ON remote API model without API key
func TestToggleModel_RemoteAPIMissingApiKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	// Note: Model is cluster-scoped, no namespace needed
	testModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-api-model",
		},
		Spec: v1.ModelSpec{
			DisplayName: "Test API",
			Source:      v1.ModelSource{AccessMode: v1.AccessModeRemoteAPI, URL: "https://api.test.com"},
		},
		Status: v1.ModelStatus{InferenceID: ""},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testModel).
		Build()

	handler := &Handler{k8sClient: k8sClient}

	reqBody := ToggleModelRequest{
		Enabled:  true,
		Instance: nil, // Missing instance with API key
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models/test-api-model/toggle", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "test-api-model"}}
	c.Set(common.UserId, "test-user")
	c.Set(common.UserName, "Test User")

	result, err := handler.toggleModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "instance.apiKey is required")
}

// TestDeleteModel_NotFound tests deleteModel when model doesn't exist
func TestDeleteModel_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	handler := &Handler{k8sClient: k8sClient}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/models/non-existent", nil)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "non-existent"}}

	result, err := handler.deleteModel(c)

	// Should still succeed (idempotent delete)
	require.NoError(t, err)
	require.NotNil(t, result)

	respMap, ok := result.(gin.H)
	require.True(t, ok)
	assert.Equal(t, "model deleted successfully", respMap["message"])
}

// TestDeleteModel_MissingID tests deleteModel without model ID
func TestDeleteModel_MissingID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/models/", nil)
	c.Params = gin.Params{gin.Param{Key: "id", Value: ""}}

	result, err := handler.deleteModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "model id is required")
}

// TestModelInfo_Struct tests the ModelInfo struct fields
func TestModelInfo_Struct(t *testing.T) {
	info := ModelInfo{
		ID:             "model-123",
		DisplayName:    "Test Model",
		Description:    "A test model",
		Icon:           "https://example.com/icon.png",
		Label:          "TestOrg",
		Tags:           "test,nlp",
		MaxTokens:      4096,
		Version:        "1.0",
		SourceURL:      "https://huggingface.co/test",
		AccessMode:     "local",
		Phase:          "Ready",
		Message:        "Model is ready",
		InferenceID:    "inf-456",
		InferencePhase: "Running",
		CreatedAt:      "2025-01-01T00:00:00Z",
		UpdatedAt:      "2025-01-02T00:00:00Z",
		IsDeleted:      false,
	}

	assert.Equal(t, "model-123", info.ID)
	assert.Equal(t, "Test Model", info.DisplayName)
	assert.Equal(t, 4096, info.MaxTokens)
	assert.Equal(t, "test,nlp", info.Tags)
	assert.False(t, info.IsDeleted)

	// Test JSON marshaling
	jsonData, err := json.Marshal(info)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "model-123")
	assert.Contains(t, string(jsonData), "Test Model")
}

// TestToggleInstanceReq tests the ToggleInstanceReq struct
func TestToggleInstanceReq(t *testing.T) {
	instance := ToggleInstanceReq{
		ApiKey: "sk-test-key",
		Model:  "gpt-4",
	}

	assert.Equal(t, "sk-test-key", instance.ApiKey)
	assert.Equal(t, "gpt-4", instance.Model)

	// Test JSON marshaling
	jsonData, err := json.Marshal(instance)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "sk-test-key")
	assert.Contains(t, string(jsonData), "gpt-4")
}

// TestToggleResourceReq tests the ToggleResourceReq struct
func TestToggleResourceReq(t *testing.T) {
	resource := ToggleResourceReq{
		Workspace: "ws-001",
		Replica:   2,
		CPU:       8,
		Memory:    32,
		GPU:       "2",
	}

	assert.Equal(t, "ws-001", resource.Workspace)
	assert.Equal(t, 2, resource.Replica)
	assert.Equal(t, 8, resource.CPU)
	assert.Equal(t, 32, resource.Memory)
	assert.Equal(t, "2", resource.GPU)
}

// TestToggleConfigReq tests the ToggleConfigReq struct
func TestToggleConfigReq(t *testing.T) {
	config := ToggleConfigReq{
		Image:      "vllm/vllm:latest",
		EntryPoint: "vllm serve /models/test",
		ModelPath:  "/apps/models/test-model",
	}

	assert.Equal(t, "vllm/vllm:latest", config.Image)
	assert.Equal(t, "vllm serve /models/test", config.EntryPoint)
	assert.Equal(t, "/apps/models/test-model", config.ModelPath)
}

// TestParseListModelQuery_InvalidLimit tests parseListModelQuery with invalid limit
func TestParseListModelQuery_InvalidLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/models?limit=-5", nil)

	result, err := parseListModelQuery(c)

	// Should error due to min=1 validation
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid query")
}

// TestParseListModelQuery_InvalidOffset tests parseListModelQuery with invalid offset
func TestParseListModelQuery_InvalidOffset(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/models?offset=-10", nil)

	result, err := parseListModelQuery(c)

	// Should error due to min=0 validation
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid query")
}

// TestCreateModel_InvalidJSON tests createModel with invalid JSON body
func TestCreateModel_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models", bytes.NewBufferString("invalid json"))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := handler.createModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid request body")
}

// TestToggleModel_InvalidJSON tests toggleModel with invalid JSON body
func TestToggleModel_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models/test/toggle", bytes.NewBufferString("invalid json"))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "test"}}

	result, err := handler.toggleModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid request body")
}

// TestCreateModel_InvalidAccessMode tests createModel with invalid access mode
func TestCreateModel_InvalidAccessMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	handler := &Handler{k8sClient: k8sClient}

	reqBody := CreateModelRequest{
		Source: ModelSourceReq{
			URL:        "https://test.com",
			AccessMode: "invalid_mode",
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := handler.createModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "accessMode must be")
}

// TestCreateModel_MissingURL tests createModel without URL
func TestCreateModel_MissingURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{}

	reqBody := CreateModelRequest{
		Source: ModelSourceReq{
			URL:        "",
			AccessMode: string(v1.AccessModeLocal),
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models", bytes.NewBuffer(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := handler.createModel(c)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "url is required")
}
