/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ── CreateUser tests ──────────────────────────────────────────────────────

func TestCreateUser_Success(t *testing.T) {
	var received CreateUserRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/user/new", r.URL.Path)
		assert.Equal(t, "Bearer sk-master", r.Header.Get("Authorization"))
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"user_id":"test@amd.com"}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	err := client.CreateUser(context.Background(), "test@amd.com")

	assert.NoError(t, err)
	assert.Equal(t, "test@amd.com", received.UserID)
	assert.Equal(t, "test@amd.com", received.UserEmail)
	assert.Equal(t, []string{"team-123"}, received.Teams)
	assert.False(t, received.AutoCreateKey)
}

func TestCreateUser_Conflict409(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"User already exists"}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	err := client.CreateUser(context.Background(), "test@amd.com")
	assert.NoError(t, err)
}

func TestCreateUser_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	err := client.CreateUser(context.Background(), "test@amd.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestCreateUser_NoAdminKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "", "team-123")
	err := client.CreateUser(context.Background(), "test@amd.com")
	assert.NoError(t, err)
}

// ── CreateKey tests ───────────────────────────────────────────────────────

func TestCreateKey_Success(t *testing.T) {
	var received CreateKeyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/key/generate", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(CreateKeyResponse{
			Key:     "sk-generated-key",
			KeyName: "sk-...key",
			TokenID: "hash-abc123",
			Expires: "2027-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	resp, err := client.CreateKey(context.Background(), "test@amd.com", "apim-key-value")

	assert.NoError(t, err)
	assert.Equal(t, "sk-generated-key", resp.Key)
	assert.Equal(t, "hash-abc123", resp.TokenID)
	assert.Equal(t, "test@amd.com", received.UserID)
	assert.Equal(t, "team-123", received.TeamID)
	assert.Equal(t, "test@amd.com", received.KeyAlias)
	assert.Equal(t, "apim-key-value", received.Metadata["apim_key"])
	assert.Equal(t, "test@amd.com", received.Metadata["safe_user_id"])
}

func TestCreateKey_LiteLLMError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"Key alias already exists"}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	resp, err := client.CreateKey(context.Background(), "test@amd.com", "apim-key")

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "400")
}

// ── UpdateKeyMetadata tests ───────────────────────────────────────────────

func TestUpdateKeyMetadata_Success(t *testing.T) {
	var received UpdateKeyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/key/update", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	err := client.UpdateKeyMetadata(context.Background(), "hash1234567890123456", "new-apim-key", "test@amd.com")

	assert.NoError(t, err)
	assert.Equal(t, "hash1234567890123456", received.Key)
	assert.Equal(t, "new-apim-key", received.Metadata["apim_key"])
	assert.Equal(t, "test@amd.com", received.Metadata["safe_user_id"])
}

func TestUpdateKeyMetadata_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"key not found"}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	err := client.UpdateKeyMetadata(context.Background(), "hash1234567890123456", "apim-key", "test@amd.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

// ── DeleteKey tests ───────────────────────────────────────────────────────

func TestDeleteKey_ByHash_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body DeleteKeyRequest
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, []string{"hash1234567890123456"}, body.Keys)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"deleted_keys":["hash123"]}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	err := client.DeleteKey(context.Background(), "hash1234567890123456", "test@amd.com")
	assert.NoError(t, err)
}

func TestDeleteKey_HashNotFound_FallbackAlias(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body DeleteKeyRequest
		json.NewDecoder(r.Body).Decode(&body)

		if calls == 1 {
			assert.NotEmpty(t, body.Keys)
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"No keys found"}`))
			return
		}
		assert.Equal(t, []string{"test@amd.com"}, body.KeyAliases)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"deleted_keys":["test@amd.com"]}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	err := client.DeleteKey(context.Background(), "wrong-hash-12345678", "test@amd.com")
	assert.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestDeleteKey_HashNotFound_NoAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"No keys found"}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	err := client.DeleteKey(context.Background(), "wrong-hash-12345678", "")
	assert.NoError(t, err)
}

func TestDeleteKey_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	err := client.DeleteKey(context.Background(), "hash1234567890123456", "test@amd.com")
	assert.Error(t, err)
}

// ── GetUserDailyActivity tests ────────────────────────────────────────────

func TestGetUserDailyActivity_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/user/daily/activity", r.URL.Path)
		assert.Equal(t, "test@amd.com", r.URL.Query().Get("user_id"))
		assert.Equal(t, "2026-03-10", r.URL.Query().Get("start_date"))
		assert.Equal(t, "2026-03-17", r.URL.Query().Get("end_date"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(DailyActivityResponse{
			Results: []DailyResult{
				{
					Date: "2026-03-17",
					Metrics: DailyMetrics{
						Spend:              0.005,
						PromptTokens:       100,
						CompletionTokens:   50,
						TotalTokens:        150,
						APIRequests:        10,
						SuccessfulRequests: 8,
						FailedRequests:     2,
					},
					Breakdown: &DailyBreakdown{
						Models: map[string]MetricWithMetadata{
							"gpt-4o": {
								Metrics: DailyMetrics{
									Spend:              0.005,
									APIRequests:        10,
									SuccessfulRequests: 8,
									FailedRequests:     2,
								},
							},
						},
					},
				},
			},
			Metadata: ActivityTotals{
				TotalSpend:              0.005,
				TotalPromptTokens:       100,
				TotalCompletionTokens:   50,
				TotalAPIRequests:        10,
				TotalSuccessfulRequests: 8,
				TotalFailedRequests:     2,
			},
		})
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	resp, err := client.GetUserDailyActivity(context.Background(), "test@amd.com", "2026-03-10", "2026-03-17")

	assert.NoError(t, err)
	assert.Len(t, resp.Results, 1)
	assert.Equal(t, "2026-03-17", resp.Results[0].Date)
	assert.Equal(t, int64(10), resp.Results[0].Metrics.APIRequests)
	assert.Equal(t, int64(8), resp.Results[0].Metrics.SuccessfulRequests)
	assert.Equal(t, float64(0.005), resp.Results[0].Breakdown.Models["gpt-4o"].Metrics.Spend)
	assert.Equal(t, float64(0.005), resp.Metadata.TotalSpend)
}

func TestGetUserDailyActivity_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"db error"}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	resp, err := client.GetUserDailyActivity(context.Background(), "test@amd.com", "2026-03-10", "2026-03-17")
	assert.Error(t, err)
	assert.Nil(t, resp)
}

// ── GetUserInfo tests ─────────────────────────────────────────────────────

func TestGetUserInfo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/user/info", r.URL.Path)
		assert.Equal(t, "test@amd.com", r.URL.Query().Get("user_id"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(UserInfoResponse{
			UserID: "test@amd.com",
			UserInfo: UserInfoData{
				Spend:      99.99,
				ModelSpend: map[string]float64{"gpt-4o": 80.0, "gpt-4": 19.99},
			},
			Keys: []UserInfoKeyData{
				{Token: "hash123", KeyAlias: "test@amd.com", Spend: 99.99},
			},
		})
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	resp, err := client.GetUserInfo(context.Background(), "test@amd.com")

	assert.NoError(t, err)
	assert.Equal(t, "test@amd.com", resp.UserID)
	assert.Equal(t, 99.99, resp.UserInfo.Spend)
	assert.Equal(t, 80.0, resp.UserInfo.ModelSpend["gpt-4o"])
	assert.Len(t, resp.Keys, 1)
}

func TestGetUserInfo_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"User not found"}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	resp, err := client.GetUserInfo(context.Background(), "nonexistent@amd.com")
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "404")
}

// ── litellmError tests ────────────────────────────────────────────────────

func TestLitellmError_Error(t *testing.T) {
	e := &litellmError{StatusCode: 500, Body: "internal error"}
	assert.Equal(t, "LiteLLM returned HTTP 500: internal error", e.Error())
}

func TestIsNotFoundErr_True(t *testing.T) {
	assert.True(t, isNotFoundErr(&litellmError{StatusCode: 404, Body: "not found"}))
}

func TestIsNotFoundErr_False(t *testing.T) {
	assert.False(t, isNotFoundErr(&litellmError{StatusCode: 500}))
	assert.False(t, isNotFoundErr(nil))
	assert.False(t, isNotFoundErr(assert.AnError))
}

// ── NewLiteLLMClient tests ────────────────────────────────────────────────

func TestNewLiteLLMClient(t *testing.T) {
	client := NewLiteLLMClient("http://localhost:4000", "sk-key", "team-1")
	assert.Equal(t, "http://localhost:4000", client.endpoint)
	assert.Equal(t, "sk-key", client.adminKey)
	assert.Equal(t, "team-1", client.teamID)
	assert.NotNil(t, client.httpClient)
}
