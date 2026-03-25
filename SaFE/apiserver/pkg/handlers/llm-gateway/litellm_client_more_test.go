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

func TestGetKeyInfo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/key/info", r.URL.Path)
		assert.Equal(t, "hash123", r.URL.Query().Get("key"))
		assert.Equal(t, "Bearer sk-master", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"info":{"spend":12.5,"max_budget":30}}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	info, err := client.GetKeyInfo(context.Background(), "hash123")

	assert.NoError(t, err)
	if assert.NotNil(t, info) {
		assert.Equal(t, 12.5, info.Spend)
		if assert.NotNil(t, info.MaxBudget) {
			assert.Equal(t, 30.0, *info.MaxBudget)
		}
	}
}

func TestGetKeyInfo_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"info":`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	info, err := client.GetKeyInfo(context.Background(), "hash123")

	assert.Error(t, err)
	assert.Nil(t, info)
	assert.Contains(t, err.Error(), "failed to decode response")
}

func TestUpdateKeyBudget_SendsSetAndRemovePayloads(t *testing.T) {
	type requestSnapshot struct {
		Key       string   `json:"key"`
		MaxBudget *float64 `json:"max_budget"`
	}

	var requests []requestSnapshot
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/key/update", r.URL.Path)
		assert.Equal(t, "Bearer sk-master", r.Header.Get("Authorization"))

		var body requestSnapshot
		err := json.NewDecoder(r.Body).Decode(&body)
		assert.NoError(t, err)
		requests = append(requests, body)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	maxBudget := 55.5

	err := client.UpdateKeyBudget(context.Background(), "hash123", &maxBudget)
	assert.NoError(t, err)

	err = client.UpdateKeyBudget(context.Background(), "hash123", nil)
	assert.NoError(t, err)

	if assert.Len(t, requests, 2) {
		assert.Equal(t, "hash123", requests[0].Key)
		if assert.NotNil(t, requests[0].MaxBudget) {
			assert.Equal(t, 55.5, *requests[0].MaxBudget)
		}
		assert.Nil(t, requests[1].MaxBudget)
	}
}

func TestGetSpendLogs_DefaultsInvalidPageValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/spend/logs/v2", r.URL.Path)
		assert.Equal(t, "1", r.URL.Query().Get("page"))
		assert.Equal(t, "100", r.URL.Query().Get("page_size"))
		assert.Equal(t, "test@amd.com", r.URL.Query().Get("user_id"))
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SpendLogsResponse{
			Data: []SpendLogEntry{
				{RequestID: "req-1", Spend: 1.2},
			},
			Page:       1,
			PageSize:   100,
			TotalPages: 1,
		})
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	resp, err := client.GetSpendLogs(context.Background(), "test@amd.com", "2026-03-01", "2026-03-02", 0, 0)

	assert.NoError(t, err)
	if assert.NotNil(t, resp) && assert.Len(t, resp.Data, 1) {
		assert.Equal(t, "req-1", resp.Data[0].RequestID)
	}
}

func TestGetAllSpendLogs_CollectsMultiplePages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "1":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(SpendLogsResponse{
				Data:       []SpendLogEntry{{RequestID: "req-1", Spend: 1.0}},
				Page:       1,
				PageSize:   100,
				TotalPages: 2,
			})
		case "2":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(SpendLogsResponse{
				Data:       []SpendLogEntry{{RequestID: "req-2", Spend: 2.0}},
				Page:       2,
				PageSize:   100,
				TotalPages: 2,
			})
		default:
			t.Fatalf("unexpected page: %s", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	logs, err := client.GetAllSpendLogs(context.Background(), "test@amd.com", "2026-03-01", "2026-03-02", 0)

	assert.NoError(t, err)
	if assert.Len(t, logs, 2) {
		assert.Equal(t, "req-1", logs[0].RequestID)
		assert.Equal(t, "req-2", logs[1].RequestID)
	}
}

func TestGetAllSpendLogs_StopsAtMaxPages(t *testing.T) {
	requestedPages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPages++
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SpendLogsResponse{
			Data:       []SpendLogEntry{{RequestID: "req-1", Spend: 1.0}},
			Page:       1,
			PageSize:   100,
			TotalPages: 3,
		})
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "sk-master", "team-123")
	logs, err := client.GetAllSpendLogs(context.Background(), "test@amd.com", "2026-03-01", "2026-03-02", 1)

	assert.NoError(t, err)
	assert.Len(t, logs, 1)
	assert.Equal(t, 1, requestedPages)
}
