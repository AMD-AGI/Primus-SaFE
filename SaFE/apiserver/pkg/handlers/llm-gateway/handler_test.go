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
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commoncrypto "github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func init() {
	gin.SetMode(gin.TestMode)
	// Disable crypto in test env so Encrypt/Decrypt are passthrough (no AES key needed)
	viper.Set("crypto.enable", false)
}

// newTestHandler creates a Handler with a mock DB, mock LiteLLM server, and crypto disabled (passthrough).
func newTestHandler(t *testing.T, mockDB *mock_client.MockInterface, litellmServer *httptest.Server) *Handler {
	// crypto is disabled in test env (no key configured) → Encrypt/Decrypt are passthrough
	// NewCrypto is safe to call — returns instance with empty key, which means no-op encrypt/decrypt
	crypto := &commoncrypto.Crypto{}

	proxy, err := newLLMProxy(litellmServer.URL)
	assert.NoError(t, err)

	return &Handler{
		dbClient:      mockDB,
		litellmClient: NewLiteLLMClient(litellmServer.URL, "sk-test-master-key", "test-team-id"),
		crypto:        crypto,
		proxy:         proxy,
		// accessController is nil — getUserEmail will fallback to userName
	}
}

// setUserContext simulates auth middleware by setting userId in gin context.
func setUserContext(c *gin.Context, userId, userName string) {
	c.Set(common.UserId, userId)
	c.Set(common.UserName, userName)
}

// ── maskKey tests ─────────────────────────────────────────────────────────

func TestMaskKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"short key", "abc", "ab****"},
		{"8 char key", "abcdefgh", "ab****"},
		{"normal key", "abcdefghijklmnop", "abcd********mnop"},
		{"long key", "sk-1234567890abcdefghijklmnop", "sk-1********mnop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, maskKey(tt.input))
		})
	}
}

// ── GetBinding tests ──────────────────────────────────────────────────────

func TestGetBinding_NotBound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(nil, nil)

	router := gin.New()
	router.GET("/binding", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.GetBinding(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/binding", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp BindingResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.False(t, resp.HasAPIMKey)
	assert.Equal(t, "test@amd.com", resp.UserEmail)
}

func TestGetBinding_Bound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	apimKey := "test-apim-key-12345"
	if handler.crypto != nil {
		encrypted, _ := handler.crypto.Encrypt([]byte(apimKey))
		apimKey = encrypted
	}
	binding := &dbclient.LLMGatewayUserBinding{
		UserEmail:         "test@amd.com",
		ApimKey:           apimKey,
		LiteLLMVirtualKey: "encrypted-vkey",
		LiteLLMKeyHash:    "hash123",
		KeyAlias:          "test@amd.com",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(binding, nil)

	router := gin.New()
	router.GET("/binding", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.GetBinding(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/binding", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp BindingResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.True(t, resp.HasAPIMKey)
	assert.Equal(t, "test@amd.com", resp.KeyAlias)
	assert.Contains(t, resp.ApimKeyHint, "****")
}

// ── CreateBinding tests ───────────────────────────────────────────────────

func TestCreateBinding_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)

	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/user/new":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"user_id":"test@amd.com"}`))
		case r.URL.Path == "/key/generate":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(CreateKeyResponse{
				Key:     "sk-test-virtual-key",
				KeyName: "sk-...key",
				TokenID: "abc123hash",
			})
		}
	}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(nil, nil)
	mockDB.EXPECT().CreateLLMBinding(gomock.Any(), gomock.Any()).Return(nil)

	router := gin.New()
	router.POST("/binding", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.CreateBinding(c)
	})

	body := `{"apim_key":"test-apim-key"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/binding", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp BindingResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.True(t, resp.HasAPIMKey)
	assert.Equal(t, "sk-test-virtual-key", resp.VirtualKey)
	assert.Equal(t, "test@amd.com", resp.KeyAlias)
}

func TestCreateBinding_AlreadyExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	existing := &dbclient.LLMGatewayUserBinding{UserEmail: "test@amd.com"}
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(existing, nil)

	router := gin.New()
	router.POST("/binding", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.CreateBinding(c)
	})

	body := `{"apim_key":"test-apim-key"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/binding", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestCreateBinding_MissingApimKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	router := gin.New()
	router.POST("/binding", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.CreateBinding(c)
	})

	body := `{}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/binding", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateBinding_LiteLLMUserAlreadyExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)

	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/user/new":
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error":"user already exists"}`))
		case r.URL.Path == "/key/generate":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(CreateKeyResponse{
				Key:     "sk-new-key",
				TokenID: "newhash123",
			})
		}
	}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(nil, nil)
	mockDB.EXPECT().CreateLLMBinding(gomock.Any(), gomock.Any()).Return(nil)

	router := gin.New()
	router.POST("/binding", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.CreateBinding(c)
	})

	body := `{"apim_key":"test-apim-key"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/binding", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

// ── DeleteBinding tests ───────────────────────────────────────────────────

func TestDeleteBinding_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)

	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"deleted_keys":["hash123"]}`))
	}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	binding := &dbclient.LLMGatewayUserBinding{
		UserEmail:      "test@amd.com",
		LiteLLMKeyHash: "hash123456789012345",
		KeyAlias:       "test@amd.com",
	}
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(binding, nil)
	mockDB.EXPECT().DeleteLLMBinding(gomock.Any(), "test@amd.com").Return(nil)

	router := gin.New()
	router.DELETE("/binding", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.DeleteBinding(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/binding", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteBinding_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(nil, nil)

	router := gin.New()
	router.DELETE("/binding", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.DeleteBinding(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/binding", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteBinding_FallbackToAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)

	callCount := 0
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var body DeleteKeyRequest
		json.NewDecoder(r.Body).Decode(&body)

		if len(body.Keys) > 0 {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"No keys found"}`))
			return
		}
		if len(body.KeyAliases) > 0 {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"deleted_keys":["test@amd.com"]}`))
			return
		}
	}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	binding := &dbclient.LLMGatewayUserBinding{
		UserEmail:      "test@amd.com",
		LiteLLMKeyHash: "wrong-hash-1234567890",
		KeyAlias:       "test@amd.com",
	}
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(binding, nil)
	mockDB.EXPECT().DeleteLLMBinding(gomock.Any(), "test@amd.com").Return(nil)

	router := gin.New()
	router.DELETE("/binding", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.DeleteBinding(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/binding", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, 2, callCount)
}

// ── ProxyLLMRequest tests ─────────────────────────────────────────────────

func TestProxyLLMRequest_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		assert.True(t, strings.HasPrefix(auth, "Bearer sk-"))
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}]}`))
	}))
	defer backend.Close()

	handler := newTestHandler(t, mockDB, backend)

	vkey := "sk-test-virtual-key"
	if handler.crypto != nil {
		encrypted, _ := handler.crypto.Encrypt([]byte(vkey))
		vkey = encrypted
	}

	binding := &dbclient.LLMGatewayUserBinding{
		UserEmail:         "test@amd.com",
		LiteLLMVirtualKey: vkey,
		LiteLLMKeyHash:    "hash123",
	}
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(binding, nil)

	router := gin.New()
	router.POST("/api/v1/llm-proxy/*proxyPath", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.ProxyLLMRequest(c)
	})

	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/v1/llm-proxy/v1/chat/completions",
		"application/json",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	assert.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestProxyLLMRequest_NoBinding(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(nil, nil)

	router := gin.New()
	router.POST("/proxy/*proxyPath", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.ProxyLLMRequest(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/proxy/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ── GetUsage tests ────────────────────────────────────────────────────────

func TestGetUsage_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)

	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/user/daily/activity")
		assert.Equal(t, "test@amd.com", r.URL.Query().Get("user_id"))
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(DailyActivityResponse{
			Results: []DailyResult{
				{
					Date: "2026-03-17",
					Metrics: DailyMetrics{
						Spend:              0.001,
						PromptTokens:       10,
						CompletionTokens:   5,
						TotalTokens:        15,
						APIRequests:        1,
						SuccessfulRequests: 1,
					},
				},
			},
			Metadata: ActivityTotals{
				TotalSpend:              0.001,
				TotalPromptTokens:       10,
				TotalCompletionTokens:   5,
				TotalAPIRequests:        1,
				TotalSuccessfulRequests: 1,
			},
		})
	}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	binding := &dbclient.LLMGatewayUserBinding{UserEmail: "test@amd.com"}
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(binding, nil)

	router := gin.New()
	router.GET("/usage", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.GetUsage(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/usage?start_date=2026-03-17&end_date=2026-03-17", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp UsageResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(0.001), resp.TotalSpend)
	assert.Equal(t, int64(1), resp.TotalSuccessfulRequests)
	assert.Len(t, resp.Daily, 1)
}

func TestGetUsage_MissingDates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	router := gin.New()
	router.GET("/usage", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.GetUsage(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/usage", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── GetSummary tests ──────────────────────────────────────────────────────

func TestGetSummary_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)

	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/user/info")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(UserInfoResponse{
			UserID: "test@amd.com",
			UserInfo: UserInfoData{
				Spend:      123.45,
				ModelSpend: map[string]float64{"gpt-4o": 100.0, "gpt-4": 23.45},
			},
		})
	}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	binding := &dbclient.LLMGatewayUserBinding{UserEmail: "test@amd.com"}
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(binding, nil)

	router := gin.New()
	router.GET("/summary", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.GetSummary(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/summary", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp SummaryResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 123.45, resp.TotalSpend)
	assert.Equal(t, 100.0, resp.ModelSpend["gpt-4o"])
}

func TestGetSummary_NoBinding(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(nil, nil)

	router := gin.New()
	router.GET("/summary", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.GetSummary(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/summary", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── newLLMProxy tests ─────────────────────────────────────────────────────

func TestNewLLMProxy_PathRewrite(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Received-Path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	tests := []struct {
		name         string
		endpoint     string
		requestPath  string
		expectedPath string
	}{
		{
			name:         "simple endpoint",
			endpoint:     backend.URL,
			requestPath:  "/api/v1/llm-proxy/v1/chat/completions",
			expectedPath: "/v1/chat/completions",
		},
		{
			name:         "endpoint with base path",
			endpoint:     backend.URL + "/llm-gateway",
			requestPath:  "/api/v1/llm-proxy/v1/models",
			expectedPath: "/llm-gateway/v1/models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy, err := newLLMProxy(tt.endpoint)
			assert.NoError(t, err)

			router := gin.New()
			router.Any("/api/v1/llm-proxy/*proxyPath", func(c *gin.Context) {
				proxy.ServeHTTP(c.Writer, c.Request)
			})

			server := httptest.NewServer(router)
			defer server.Close()

			resp, err := http.Get(server.URL + tt.requestPath)
			assert.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedPath, resp.Header.Get("X-Received-Path"))
		})
	}
}

func TestNewLLMProxy_InvalidEndpoint(t *testing.T) {
	_, err := newLLMProxy("://invalid")
	assert.Error(t, err)
}

// ── LiteLLMClient tests ──────────────────────────────────────────────────

func TestCreateUser_AutoCreateKeyFalse(t *testing.T) {
	var receivedBody CreateUserRequest

	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"user_id":"test@amd.com"}`))
	}))
	defer litellm.Close()

	client := NewLiteLLMClient(litellm.URL, "sk-master", "team-id")
	err := client.CreateUser(context.Background(), "test@amd.com")
	assert.NoError(t, err)
	assert.False(t, receivedBody.AutoCreateKey)
	assert.Equal(t, "test@amd.com", receivedBody.UserID)
}

func TestCreateUser_ConflictIsOK(t *testing.T) {
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"user already exists"}`))
	}))
	defer litellm.Close()

	client := NewLiteLLMClient(litellm.URL, "sk-master", "team-id")
	err := client.CreateUser(context.Background(), "test@amd.com")
	assert.NoError(t, err)
}

func TestDeleteKey_FallbackToAlias(t *testing.T) {
	callCount := 0
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var body DeleteKeyRequest
		json.NewDecoder(r.Body).Decode(&body)

		if len(body.Keys) > 0 {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"No keys found"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"deleted_keys":["alias"]}`))
	}))
	defer litellm.Close()

	client := NewLiteLLMClient(litellm.URL, "sk-master", "team-id")
	err := client.DeleteKey(context.Background(), "wrong-hash-123456789", "test@amd.com")
	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestIsNotFoundErr(t *testing.T) {
	assert.True(t, isNotFoundErr(&litellmError{StatusCode: 404}))
	assert.False(t, isNotFoundErr(&litellmError{StatusCode: 500}))
	assert.False(t, isNotFoundErr(nil))
}
