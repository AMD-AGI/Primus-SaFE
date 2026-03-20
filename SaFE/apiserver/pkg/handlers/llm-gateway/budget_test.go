/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestBuildBudgetResponse_WithLimit(t *testing.T) {
	maxBudget := 100.0
	resp := buildBudgetResponse("test@amd.com", &KeyInfoData{
		Spend:     40,
		MaxBudget: &maxBudget,
	}, "updated")

	if assert.NotNil(t, resp.MaxBudget) {
		assert.Equal(t, 100.0, *resp.MaxBudget)
	}
	if assert.NotNil(t, resp.Remaining) {
		assert.Equal(t, 60.0, *resp.Remaining)
	}
	if assert.NotNil(t, resp.UsagePercent) {
		assert.Equal(t, 40.0, *resp.UsagePercent)
	}
	assert.False(t, resp.BudgetExceeded)
	assert.Equal(t, "updated", resp.Message)
}

func TestBuildBudgetResponse_Exceeded(t *testing.T) {
	maxBudget := 25.0
	resp := buildBudgetResponse("test@amd.com", &KeyInfoData{
		Spend:     30,
		MaxBudget: &maxBudget,
	}, "")

	if assert.NotNil(t, resp.Remaining) {
		assert.Equal(t, -5.0, *resp.Remaining)
	}
	if assert.NotNil(t, resp.UsagePercent) {
		assert.Equal(t, 120.0, *resp.UsagePercent)
	}
	assert.True(t, resp.BudgetExceeded)
}

func TestGetBudget_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/key/info", r.URL.Path)
		assert.Equal(t, "hash123", r.URL.Query().Get("key"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"info":{"spend":12.5,"max_budget":20}}`))
	}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(&dbclient.LLMGatewayUserBinding{
		UserEmail:      "test@amd.com",
		LiteLLMKeyHash: "hash123",
	}, nil)

	router := ginBudgetRouter(handler, http.MethodGet, "/budget", handler.GetBudget)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/budget", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp BudgetResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "test@amd.com", resp.UserEmail)
	assert.Equal(t, 12.5, resp.Spend)
	if assert.NotNil(t, resp.MaxBudget) {
		assert.Equal(t, 20.0, *resp.MaxBudget)
	}
	if assert.NotNil(t, resp.Remaining) {
		assert.Equal(t, 7.5, *resp.Remaining)
	}
	assert.False(t, resp.BudgetExceeded)
}

func TestSetBudget_GetKeyInfoFailureFallsBackToUpdatedBudget(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	updateCalls := 0
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/key/update":
			updateCalls++
			var body UpdateKeyBudgetRequest
			err := json.NewDecoder(r.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, "hash123", body.Key)
			if assert.NotNil(t, body.MaxBudget) {
				assert.Equal(t, 50.0, *body.MaxBudget)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		case "/key/info":
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error":"upstream unavailable"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(&dbclient.LLMGatewayUserBinding{
		UserEmail:      "test@amd.com",
		LiteLLMKeyHash: "hash123",
	}, nil)

	router := ginBudgetRouter(handler, http.MethodPut, "/budget", handler.SetBudget)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/budget", strings.NewReader(`{"max_budget":50}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, updateCalls)

	var resp BudgetResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "Budget updated successfully", resp.Message)
	if assert.NotNil(t, resp.MaxBudget) {
		assert.Equal(t, 50.0, *resp.MaxBudget)
	}
	assert.Nil(t, resp.Remaining)
}

func TestRemoveBudget_LiteLLMFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/key/update", r.URL.Path)
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":"update failed"}`))
	}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(&dbclient.LLMGatewayUserBinding{
		UserEmail:      "test@amd.com",
		LiteLLMKeyHash: "hash123",
	}, nil)

	router := ginBudgetRouter(handler, http.MethodDelete, "/budget", handler.RemoveBudget)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/budget", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), "failed to remove budget limit")
}

func ginBudgetRouter(handler *Handler, method, path string, endpoint gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.Handle(method, path, func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		endpoint(c)
	})
	return router
}
