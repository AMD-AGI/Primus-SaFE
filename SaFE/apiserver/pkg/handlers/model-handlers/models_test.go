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
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
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
		Items: []dbclient.Model{
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
	testModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
			// Note: Model is cluster-scoped, no namespace needed
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
	testModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-api-model",
			// Note: Model is cluster-scoped, no namespace needed
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
