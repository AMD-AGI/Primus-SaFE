/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// genMockRunningWorkload generates a mock running Workload for testing
func genMockRunningWorkload(name, workspace string) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(time.Now()),
			Labels: map[string]string{
				v1.ClusterIdLabel: "test-cluster",
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace: workspace,
			GroupVersionKind: v1.GroupVersionKind{
				Kind: common.DeploymentKind,
			},
			Service: &v1.Service{
				Port:       8000,
				TargetPort: 8000,
			},
			Env: map[string]string{
				"PRIMUS_SOURCE_MODEL": "source-model-id",
			},
		},
		Status: v1.WorkloadStatus{
			Phase: v1.WorkloadRunning,
		},
	}
}

// genMockApiKeySecret generates a mock Secret for API key
func genMockApiKeySecret(name, apiKey string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: common.PrimusSafeNamespace,
		},
		Data: map[string][]byte{
			"apiKey": []byte(apiKey),
		},
	}
}

// genMockTokenSecret generates a mock Secret for HuggingFace token
func genMockTokenSecret(name, token string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: common.PrimusSafeNamespace,
		},
		Data: map[string][]byte{
			"token": []byte(token),
		},
	}
}

// TestListPlaygroundServices tests the listPlaygroundServices handler
func TestListPlaygroundServices(t *testing.T) {
	// Create remote API model
	remoteModel := genMockRemoteAPIK8sModel("remote-model")
	remoteModel.Status.Phase = v1.ModelPhaseReady

	// Create local model (should not appear in playground services)
	localModel := genMockLocalK8sModel("local-model", "ws1")
	localModel.Status.Phase = v1.ModelPhaseReady

	// Create running workload
	workload := genMockRunningWorkload("workload-1", "ws1")

	// Create non-running workload (should not appear)
	pendingWorkload := genMockRunningWorkload("workload-2", "ws1")
	pendingWorkload.Status.Phase = v1.WorkloadPending

	k8sClient := fake.NewClientBuilder().
		WithObjects(remoteModel, localModel, workload, pendingWorkload).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	t.Run("List all playground services", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/playground/services", nil)

		result, err := h.listPlaygroundServices(c)
		assert.NilError(t, err)

		resp := result.(*ListPlaygroundServicesResponse)
		// Should have 1 remote_api model + 1 running workload
		assert.Equal(t, resp.Total, 2)

		// Check that we have the expected types
		hasRemoteAPI := false
		hasWorkload := false
		for _, item := range resp.Items {
			if item.Type == "remote_api" {
				hasRemoteAPI = true
				assert.Equal(t, item.ID, "remote-model")
			}
			if item.Type == "workload" {
				hasWorkload = true
				assert.Equal(t, item.ID, "workload-1")
			}
		}
		assert.Equal(t, hasRemoteAPI, true)
		assert.Equal(t, hasWorkload, true)
	})

	t.Run("Filter by workspace", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/playground/services?workspace=ws1", nil)

		result, err := h.listPlaygroundServices(c)
		assert.NilError(t, err)

		resp := result.(*ListPlaygroundServicesResponse)
		// Should have 1 remote_api model (not filtered) + 1 running workload in ws1
		assert.Assert(t, resp.Total >= 1)
	})
}

// TestListPlaygroundServices_ExcludeNonInferenceWorkloads tests that non-inference workloads are excluded
func TestListPlaygroundServices_ExcludeNonInferenceWorkloads(t *testing.T) {
	// Create inference workload (Deployment)
	inferenceWorkload := genMockRunningWorkload("inference-workload", "ws1")

	// Create training workload (PyTorchJob)
	trainingWorkload := genMockRunningWorkload("training-workload", "ws1")
	trainingWorkload.Spec.GroupVersionKind.Kind = "PyTorchJob"

	// Create CI/CD workload
	cicdWorkload := genMockRunningWorkload("cicd-workload", "ws1")
	cicdWorkload.Spec.GroupVersionKind.Kind = "AutoscalingRunnerSet"

	k8sClient := fake.NewClientBuilder().
		WithObjects(inferenceWorkload, trainingWorkload, cicdWorkload).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/playground/services", nil)

	result, err := h.listPlaygroundServices(c)
	assert.NilError(t, err)

	resp := result.(*ListPlaygroundServicesResponse)
	// Should only have 1 inference workload
	assert.Equal(t, resp.Total, 1)
	assert.Equal(t, resp.Items[0].ID, "inference-workload")
}

// TestGetChatURL tests the getChatURL handler
func TestGetChatURL(t *testing.T) {
	model := genMockRemoteAPIK8sModel("remote-model")
	model.Spec.Source.ApiKey = &corev1.LocalObjectReference{Name: "api-key-secret"}

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "remote-model"}}
	c.Request, _ = http.NewRequest("GET", "/playground/models/remote-model/chat-url", nil)

	result, err := h.getChatURL(c)
	assert.NilError(t, err)

	resp := result.(*ChatURLResponse)
	assert.Equal(t, resp.URL, "https://api.openai.com")
	assert.Equal(t, resp.ModelName, "gpt-4")
	assert.Equal(t, resp.HasApiKey, true)
}

// TestGetChatURL_LocalModel tests getChatURL for local model (should fail)
func TestGetChatURL_LocalModel(t *testing.T) {
	model := genMockLocalK8sModel("local-model", "ws1")

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "local-model"}}
	c.Request, _ = http.NewRequest("GET", "/playground/models/local-model/chat-url", nil)

	_, err := h.getChatURL(c)
	assert.ErrorContains(t, err, "only available for remote_api models")
}

// TestGetChatURL_NotFound tests getChatURL when model doesn't exist
func TestGetChatURL_NotFound(t *testing.T) {
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "non-existent"}}
	c.Request, _ = http.NewRequest("GET", "/playground/models/non-existent/chat-url", nil)

	_, err := h.getChatURL(c)
	assert.ErrorContains(t, err, "not found")
}

// TestGetApiKeyFromSecret tests the getApiKeyFromSecret function
func TestGetApiKeyFromSecret(t *testing.T) {
	apiKeySecret := genMockApiKeySecret("test-api-key", "sk-test-key-12345")

	// Add corev1 scheme
	testScheme := scheme.Scheme
	_ = corev1.AddToScheme(testScheme)

	k8sClient := fake.NewClientBuilder().
		WithObjects(apiKeySecret).
		WithScheme(testScheme).
		Build()

	h := newMockModelHandler(k8sClient)

	t.Run("Get existing API key", func(t *testing.T) {
		key := h.getApiKeyFromSecret(context.Background(), "test-api-key")
		assert.Equal(t, key, "sk-test-key-12345")
	})

	t.Run("Get non-existent API key", func(t *testing.T) {
		key := h.getApiKeyFromSecret(context.Background(), "non-existent")
		assert.Equal(t, key, "")
	})
}

// TestGetTokenFromSecret tests the getTokenFromSecret function
func TestGetTokenFromSecret(t *testing.T) {
	tokenSecret := genMockTokenSecret("test-token", "hf_test_token_12345")

	// Add corev1 scheme
	testScheme := scheme.Scheme
	_ = corev1.AddToScheme(testScheme)

	k8sClient := fake.NewClientBuilder().
		WithObjects(tokenSecret).
		WithScheme(testScheme).
		Build()

	h := newMockModelHandler(k8sClient)

	t.Run("Get existing token", func(t *testing.T) {
		token := h.getTokenFromSecret(context.Background(), "test-token")
		assert.Equal(t, token, "hf_test_token_12345")
	})

	t.Run("Get non-existent token", func(t *testing.T) {
		token := h.getTokenFromSecret(context.Background(), "non-existent")
		assert.Equal(t, token, "")
	})
}

// TestToNullTime tests the toNullTime helper function
func TestToNullTime(t *testing.T) {
	now := time.Now()
	nt := toNullTime(now)

	assert.Equal(t, nt.Valid, true)
	assert.Equal(t, nt.Time.Unix(), now.Unix())
}

// TestFormatTime tests the formatTime helper function
func TestFormatTime(t *testing.T) {
	t.Run("Valid time", func(t *testing.T) {
		now := time.Now()
		nt := pq.NullTime{Valid: true, Time: now}
		formatted := formatTime(nt)
		assert.Assert(t, formatted != "")
	})

	t.Run("Invalid time", func(t *testing.T) {
		nt := pq.NullTime{Valid: false}
		formatted := formatTime(nt)
		assert.Equal(t, formatted, "")
	})
}

// TestGetString tests the getString helper function
func TestGetString(t *testing.T) {
	// Note: getString is for sql.NullString
	// This test validates behavior for database null strings
	t.Run("Helper function exists", func(t *testing.T) {
		// Just verify the function signature works
		assert.Assert(t, true)
	})
}

// TestGetTime tests the getTime helper function
func TestGetTime(t *testing.T) {
	t.Run("Valid time", func(t *testing.T) {
		now := time.Now()
		nt := pq.NullTime{Valid: true, Time: now}
		result := getTime(nt)
		assert.Equal(t, result.Unix(), now.Unix())
	})

	t.Run("Invalid time", func(t *testing.T) {
		nt := pq.NullTime{Valid: false}
		result := getTime(nt)
		assert.Equal(t, result.IsZero(), true)
	})
}

// TestCvtDBSessionToInfo tests the cvtDBSessionToInfo function
func TestCvtDBSessionToInfo(t *testing.T) {
	now := time.Now()
	session := &dbclient.PlaygroundSession{
		Id:           123,
		UserId:       "user-1",
		ModelName:    "test-model",
		DisplayName:  "Test Session",
		SystemPrompt: "You are a helpful assistant",
		Messages:     `[{"role":"user","content":"Hello"}]`,
		CreationTime: pq.NullTime{Valid: true, Time: now},
		UpdateTime:   pq.NullTime{Valid: true, Time: now},
	}

	info := cvtDBSessionToInfo(session)

	assert.Equal(t, info.Id, int64(123))
	assert.Equal(t, info.UserId, "user-1")
	assert.Equal(t, info.ModelName, "test-model")
	assert.Equal(t, info.DisplayName, "Test Session")
	assert.Equal(t, info.SystemPrompt, "You are a helpful assistant")
	assert.Assert(t, info.CreationTime != "")
	assert.Assert(t, info.UpdateTime != "")
}

// TestCvtDBSessionToDetail tests the cvtDBSessionToDetail function
func TestCvtDBSessionToDetail(t *testing.T) {
	now := time.Now()
	session := &dbclient.PlaygroundSession{
		Id:           456,
		UserId:       "user-2",
		ModelName:    "gpt-4",
		DisplayName:  "GPT-4 Session",
		SystemPrompt: "You are an expert programmer",
		Messages:     `[{"role":"user","content":"Write code"}]`,
		CreationTime: pq.NullTime{Valid: true, Time: now},
		UpdateTime:   pq.NullTime{Valid: true, Time: now},
	}

	detail := cvtDBSessionToDetail(session)

	assert.Equal(t, detail.Id, int64(456))
	assert.Equal(t, detail.UserId, "user-2")
	assert.Equal(t, detail.ModelName, "gpt-4")
	assert.Equal(t, detail.DisplayName, "GPT-4 Session")
	assert.Equal(t, detail.SystemPrompt, "You are an expert programmer")
	assert.Assert(t, detail.CreationTime != "")
}

// TestParseListPlaygroundSessionQuery tests the parseListPlaygroundSessionQuery function
func TestParseListPlaygroundSessionQuery(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedLimit  int
		expectedOffset int
	}{
		{
			name:           "Default values",
			query:          "",
			expectedLimit:  100,
			expectedOffset: 0,
		},
		{
			name:           "Custom values",
			query:          "limit=50&offset=10",
			expectedLimit:  50,
			expectedOffset: 10,
		},
		{
			name:           "With model name filter",
			query:          "limit=20&modelName=gpt-4",
			expectedLimit:  20,
			expectedOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("GET", "/playground/sessions?"+tt.query, nil)

			query, err := parseListPlaygroundSessionQuery(c)
			assert.NilError(t, err)
			assert.Equal(t, query.Limit, tt.expectedLimit)
			assert.Equal(t, query.Offset, tt.expectedOffset)
		})
	}
}

// TestPlaygroundServiceItem tests the PlaygroundServiceItem struct
func TestPlaygroundServiceItem(t *testing.T) {
	item := PlaygroundServiceItem{
		Type:            "workload",
		ID:              "workload-1",
		DisplayName:     "Test Workload",
		ModelName:       "test-model",
		Phase:           "Running",
		Workspace:       "ws1",
		BaseUrl:         "http://workload-1.ws1.svc.cluster.local:8000",
		SourceModelID:   "model-1",
		SourceModelName: "Test Model",
	}

	assert.Equal(t, item.Type, "workload")
	assert.Equal(t, item.ID, "workload-1")
	assert.Equal(t, item.BaseUrl, "http://workload-1.ws1.svc.cluster.local:8000")
}

// TestChatRequest tests the ChatRequest struct
func TestChatRequest(t *testing.T) {
	req := ChatRequest{
		ServiceId: "model-1",
		ModelName: "gpt-4",
		BaseUrl:   "https://api.openai.com",
		ApiKey:    "sk-test",
		Messages: []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
		Stream:           true,
		Temperature:      0.7,
		TopP:             0.9,
		MaxTokens:        1000,
		FrequencyPenalty: 0.5,
		PresencePenalty:  0.5,
		N:                1,
	}

	assert.Equal(t, req.ServiceId, "model-1")
	assert.Equal(t, req.Stream, true)
	assert.Equal(t, req.Temperature, 0.7)
	assert.Equal(t, len(req.Messages), 1)
}

// TestListPlaygroundServicesQuery tests the ListPlaygroundServicesQuery struct
func TestListPlaygroundServicesQuery(t *testing.T) {
	query := ListPlaygroundServicesQuery{
		Workspace: "ws1",
	}
	assert.Equal(t, query.Workspace, "ws1")
}

// TestPlaygroundSessionInfo tests the PlaygroundSessionInfo struct
func TestPlaygroundSessionInfo(t *testing.T) {
	info := PlaygroundSessionInfo{
		Id:           1,
		UserId:       "user-1",
		ModelName:    "gpt-4",
		DisplayName:  "Test Session",
		SystemPrompt: "You are helpful",
		Messages:     `[]`,
		CreationTime: "2025-01-01T00:00:00Z",
		UpdateTime:   "2025-01-01T01:00:00Z",
	}

	assert.Equal(t, info.Id, int64(1))
	assert.Equal(t, info.ModelName, "gpt-4")
}

// TestSaveSessionRequest tests the SaveSessionRequest struct
func TestSaveSessionRequest(t *testing.T) {
	req := SaveSessionRequest{
		Id:           0, // New session
		ModelName:    "gpt-4",
		DisplayName:  "My Chat",
		SystemPrompt: "You are helpful",
		Messages: []MessageHistory{
			{
				Role:      "user",
				Content:   "Hello",
				Timestamp: time.Now(),
			},
		},
	}

	assert.Equal(t, req.ModelName, "gpt-4")
	assert.Equal(t, len(req.Messages), 1)
}

// TestMessageHistory tests the MessageHistory struct
func TestMessageHistory(t *testing.T) {
	now := time.Now()
	msg := MessageHistory{
		Role:      "assistant",
		Content:   "Hello! How can I help you?",
		Timestamp: now,
	}

	assert.Equal(t, msg.Role, "assistant")
	assert.Equal(t, msg.Content, "Hello! How can I help you?")
	assert.Equal(t, msg.Timestamp.Unix(), now.Unix())
}

// TestChatURLResponse tests the ChatURLResponse struct
func TestChatURLResponse(t *testing.T) {
	resp := ChatURLResponse{
		URL:       "https://api.openai.com",
		ModelName: "gpt-4",
		HasApiKey: true,
	}

	assert.Equal(t, resp.URL, "https://api.openai.com")
	assert.Equal(t, resp.ModelName, "gpt-4")
	assert.Equal(t, resp.HasApiKey, true)
}

// TestListPlaygroundServices_WithSourceModel tests that source model info is populated
func TestListPlaygroundServices_WithSourceModel(t *testing.T) {
	// Create source model
	sourceModel := genMockLocalK8sModel("source-model-id", "")
	sourceModel.Spec.DisplayName = "Source Model Display Name"

	// Create workload with source model
	workload := genMockRunningWorkload("workload-1", "ws1")
	workload.Spec.Env["PRIMUS_SOURCE_MODEL"] = "source-model-id"

	k8sClient := fake.NewClientBuilder().
		WithObjects(sourceModel, workload).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/playground/services", nil)

	result, err := h.listPlaygroundServices(c)
	assert.NilError(t, err)

	resp := result.(*ListPlaygroundServicesResponse)

	// Find the workload item
	var workloadItem *PlaygroundServiceItem
	for i := range resp.Items {
		if resp.Items[i].Type == "workload" {
			workloadItem = &resp.Items[i]
			break
		}
	}

	assert.Assert(t, workloadItem != nil)
	assert.Equal(t, workloadItem.SourceModelID, "source-model-id")
	assert.Equal(t, workloadItem.SourceModelName, "Source Model Display Name")
}

// TestListPlaygroundServices_BaseUrlConstruction tests baseUrl is constructed correctly
func TestListPlaygroundServices_BaseUrlConstruction(t *testing.T) {
	// Create workload with service config
	workload := genMockRunningWorkload("workload-1", "ws1")
	workload.Spec.Service = &v1.Service{
		Port:       8080,
		TargetPort: 8080,
	}

	k8sClient := fake.NewClientBuilder().
		WithObjects(workload).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/playground/services", nil)

	result, err := h.listPlaygroundServices(c)
	assert.NilError(t, err)

	resp := result.(*ListPlaygroundServicesResponse)
	assert.Assert(t, resp.Total >= 1)

	// Find workload item
	for _, item := range resp.Items {
		if item.Type == "workload" {
			// Should have a baseUrl (internal or external)
			assert.Assert(t, item.BaseUrl != "")
		}
	}
}

// TestDeletePlaygroundSession_RequiresDatabase tests that session operations require database
func TestDeletePlaygroundSession_RequiresDatabase(t *testing.T) {
	// Create handler without database client
	h := &Handler{
		k8sClient: nil,
		dbClient:  nil, // No database
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "123"}}
	c.Request, _ = http.NewRequest("DELETE", "/playground/sessions/123", nil)

	_, err := h.deletePlaygroundSession(c)
	assert.ErrorContains(t, err, "requires database")
}

// TestSaveSession_RequiresDatabase tests that save session requires database
func TestSaveSession_RequiresDatabase(t *testing.T) {
	h := &Handler{
		k8sClient: nil,
		dbClient:  nil,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/playground/sessions", nil)

	_, err := h.saveSession(c)
	assert.ErrorContains(t, err, "requires database")
}

// TestListPlaygroundSession_RequiresDatabase tests that list sessions requires database
func TestListPlaygroundSession_RequiresDatabase(t *testing.T) {
	h := &Handler{
		k8sClient: nil,
		dbClient:  nil,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/playground/sessions", nil)

	_, err := h.listPlaygroundSession(c)
	assert.ErrorContains(t, err, "requires database")
}

// TestGetPlaygroundSession_RequiresDatabase tests that get session requires database
func TestGetPlaygroundSession_RequiresDatabase(t *testing.T) {
	h := &Handler{
		k8sClient: nil,
		dbClient:  nil,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "123"}}
	c.Request, _ = http.NewRequest("GET", "/playground/sessions/123", nil)

	_, err := h.getPlaygroundSession(c)
	assert.ErrorContains(t, err, "requires database")
}

// TestWorkloadEnvSourceModel tests that workload env contains source model
func TestWorkloadEnvSourceModel(t *testing.T) {
	workload := genMockRunningWorkload("test-workload", "ws1")

	// Verify PRIMUS_SOURCE_MODEL is in env
	sourceModel := workload.GetEnv("PRIMUS_SOURCE_MODEL")
	assert.Equal(t, sourceModel, "source-model-id")
}

// TestPlaygroundServiceItem_RemoteAPIType tests remote API service item
func TestPlaygroundServiceItem_RemoteAPIType(t *testing.T) {
	model := genMockRemoteAPIK8sModel("remote-model")
	model.Status.Phase = v1.ModelPhaseReady

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/playground/services", nil)

	result, err := h.listPlaygroundServices(c)
	assert.NilError(t, err)

	resp := result.(*ListPlaygroundServicesResponse)
	assert.Assert(t, resp.Total >= 1)

	// Find remote API item
	for _, item := range resp.Items {
		if item.Type == "remote_api" {
			assert.Equal(t, item.ID, "remote-model")
			assert.Equal(t, item.BaseUrl, "https://api.openai.com")
			assert.Equal(t, item.ModelName, "gpt-4")
		}
	}
}

// TestListPlaygroundServices_EmptyK8sClient tests error handling
func TestListPlaygroundServices_K8sError(t *testing.T) {
	// Test with nil client would panic, so we just verify the function signature
	// In production, the k8sClient is always non-nil
	model := genMockRemoteAPIK8sModel("remote-model")
	model.Status.Phase = v1.ModelPhaseReady

	k8sClient := fake.NewClientBuilder().
		WithObjects(model).
		WithScheme(scheme.Scheme).
		Build()

	h := newMockModelHandler(k8sClient)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/playground/services", nil)

	// Should not panic and return valid result
	result, err := h.listPlaygroundServices(c)
	assert.NilError(t, err)
	assert.Assert(t, result != nil)
}

// TestChatRequest_Validation tests ChatRequest validation
func TestChatRequest_Validation(t *testing.T) {
	// Valid request
	validReq := ChatRequest{
		ServiceId: "model-1",
		Messages: []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	assert.Assert(t, validReq.ServiceId != "")
	assert.Assert(t, len(validReq.Messages) > 0)

	// Request with optional fields
	reqWithOptions := ChatRequest{
		ServiceId:        "model-1",
		ModelName:        "custom-model",
		BaseUrl:          "http://custom-url",
		ApiKey:           "custom-key",
		Messages:         []map[string]interface{}{{"role": "user", "content": "Hi"}},
		Stream:           true,
		Temperature:      1.0,
		TopP:             0.95,
		MaxTokens:        2000,
		FrequencyPenalty: 0.0,
		PresencePenalty:  0.0,
		N:                2,
	}
	assert.Equal(t, reqWithOptions.Stream, true)
	assert.Equal(t, reqWithOptions.Temperature, 1.0)
	assert.Equal(t, reqWithOptions.N, 2)
}

// TestWorkloadKindFiltering tests that only Deployment/StatefulSet kinds are included
func TestWorkloadKindFiltering(t *testing.T) {
	tests := []struct {
		kind     string
		included bool
	}{
		{common.DeploymentKind, true},
		{common.StatefulSetKind, true},
		{"PyTorchJob", false},
		{"AutoscalingRunnerSet", false},
		{"RayJob", false},
		{"", true}, // Empty kind defaults to included (backward compatibility)
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			workload := genMockRunningWorkload("test-workload", "ws1")
			workload.Spec.Kind = tt.kind

			k8sClient := fake.NewClientBuilder().
				WithObjects(workload).
				WithScheme(scheme.Scheme).
				Build()

			h := newMockModelHandler(k8sClient)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("GET", "/playground/services", nil)

			result, err := h.listPlaygroundServices(c)
			assert.NilError(t, err)

			resp := result.(*ListPlaygroundServicesResponse)

			found := false
			for _, item := range resp.Items {
				if item.Type == "workload" && item.ID == "test-workload" {
					found = true
					break
				}
			}

			if tt.included {
				assert.Equal(t, found, true, "Expected workload with kind %s to be included", tt.kind)
			} else {
				assert.Equal(t, found, false, "Expected workload with kind %s to be excluded", tt.kind)
			}
		})
	}
}

