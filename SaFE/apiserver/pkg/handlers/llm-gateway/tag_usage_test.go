/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestParseIntParam(t *testing.T) {
	assert.Equal(t, 7, parseIntParam("7", 1))
	assert.Equal(t, 9, parseIntParam("", 9))
	assert.Equal(t, 3, parseIntParam("oops", 3))
}

func TestAggregateByTag(t *testing.T) {
	logs := []SpendLogEntry{
		{
			Spend:            1.5,
			PromptTokens:     10,
			CompletionTokens: 5,
			RequestTags:      json.RawMessage(`["team-a","User-Agent:SAFE"]`),
		},
		{
			Spend:            2.0,
			PromptTokens:     20,
			CompletionTokens: 8,
			RequestTags:      json.RawMessage(`["team-a","team-b"]`),
		},
		{
			Spend:            3.0,
			PromptTokens:     30,
			CompletionTokens: 12,
			RequestTags:      json.RawMessage(`["User-Agent:SAFE"]`),
		},
		{
			Spend:            4.0,
			PromptTokens:     40,
			CompletionTokens: 16,
			RequestTags:      json.RawMessage(`invalid-json`),
		},
	}

	result := aggregateByTag(logs)

	assert.Equal(t, 10.5, result.totalSpend)
	assert.Equal(t, int64(4), result.totalRequests)
	assert.Len(t, result.tags, 3)

	items := make(map[string]TagUsageItem, len(result.tags))
	for _, item := range result.tags {
		if item.TagName == nil {
			items[""] = item
			continue
		}
		items[*item.TagName] = item
	}

	assert.Equal(t, 3.5, items["team-a"].Spend)
	assert.Equal(t, int64(2), items["team-a"].APIRequests)
	assert.Equal(t, int64(30), items["team-a"].PromptTokens)
	assert.Equal(t, 2.0, items["team-b"].Spend)
	assert.Equal(t, int64(2), items[""].APIRequests)
	assert.Equal(t, 7.0, items[""].Spend)
}

func TestGetTagUsage_SuccessPaginationAndSorting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/spend/logs/v2", r.URL.Path)
		assert.Equal(t, "test@amd.com", r.URL.Query().Get("user_id"))
		assert.Equal(t, "2026-03-01", r.URL.Query().Get("start_date"))
		assert.Equal(t, "2026-03-10", r.URL.Query().Get("end_date"))
		assert.Equal(t, "1", r.URL.Query().Get("page"))
		assert.Equal(t, "100", r.URL.Query().Get("page_size"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SpendLogsResponse{
			Data: []SpendLogEntry{
				{Spend: 1.0, RequestTags: json.RawMessage(`["alpha"]`)},
				{Spend: 5.0, RequestTags: json.RawMessage(`["beta"]`)},
				{Spend: 3.0, RequestTags: json.RawMessage(`["alpha","User-Agent:SAFE"]`)},
				{Spend: 2.0, RequestTags: json.RawMessage(`[]`)},
			},
			Page:       1,
			PageSize:   100,
			TotalPages: 1,
		})
	}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(&dbclient.LLMGatewayUserBinding{
		UserEmail: "test@amd.com",
	}, nil)

	router := gin.New()
	router.GET("/tags/usage", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.GetTagUsage(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/tags/usage?start_date=2026-03-01&end_date=2026-03-10&page=1&page_size=2", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp TagUsageResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "test@amd.com", resp.UserEmail)
	assert.Equal(t, 11.0, resp.TotalSpend)
	assert.Equal(t, int64(4), resp.TotalRequests)
	assert.Equal(t, 2, resp.PageSize)
	assert.Equal(t, 3, resp.Total)
	assert.Equal(t, 2, resp.TotalPages)
	if assert.Len(t, resp.Tags, 2) {
		if assert.NotNil(t, resp.Tags[0].TagName) {
			assert.Equal(t, "beta", *resp.Tags[0].TagName)
		}
		assert.Equal(t, 5.0, resp.Tags[0].Spend)
		if assert.NotNil(t, resp.Tags[1].TagName) {
			assert.Equal(t, "alpha", *resp.Tags[1].TagName)
		}
		assert.Equal(t, 4.0, resp.Tags[1].Spend)
	}
}

func TestGetTagUsage_MissingDates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)

	router := gin.New()
	router.GET("/tags/usage", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.GetTagUsage(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/tags/usage", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "start_date and end_date are required")
}

func TestGetTagUsage_UsesDefaultAndMaxPaginationBounds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	litellm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SpendLogsResponse{
			Data: []SpendLogEntry{
				{Spend: 2.0, RequestTags: json.RawMessage(`["alpha"]`)},
			},
			Page:       1,
			PageSize:   100,
			TotalPages: 1,
		})
	}))
	defer litellm.Close()

	handler := newTestHandler(t, mockDB, litellm)
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "test@amd.com").Return(&dbclient.LLMGatewayUserBinding{
		UserEmail: "test@amd.com",
	}, nil)

	router := gin.New()
	router.GET("/tags/usage", func(c *gin.Context) {
		setUserContext(c, "user1", "test@amd.com")
		handler.GetTagUsage(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/tags/usage?start_date=2026-03-01&end_date=2026-03-10&page=0&page_size=999", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp TagUsageResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, maxTagPageSize, resp.PageSize)
}
