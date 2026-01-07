/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// TestConvertToApiKeyResponseItem tests the conversion from database record to response item
func TestConvertToApiKeyResponseItem(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name     string
		record   *dbclient.ApiKey
		validate func(*testing.T, view.ApiKeyResponseItem)
	}{
		{
			name: "complete record",
			record: &dbclient.ApiKey{
				Id:             1,
				Name:           "test-key",
				UserId:         "user-123",
				ApiKey:         "ak-secret-key-value", // Should NOT appear in response
				ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
				CreationTime:   pq.NullTime{Time: now, Valid: true},
				Whitelist:      `["192.168.1.1", "10.0.0.0/8"]`,
				Deleted:        false,
				DeletionTime:   pq.NullTime{Valid: false},
			},
			validate: func(t *testing.T, result view.ApiKeyResponseItem) {
				assert.Equal(t, int64(1), result.Id)
				assert.Equal(t, "test-key", result.Name)
				assert.Equal(t, "user-123", result.UserId)
				assert.NotEmpty(t, result.ExpirationTime)
				assert.NotEmpty(t, result.CreationTime)
				assert.Equal(t, []string{"192.168.1.1", "10.0.0.0/8"}, result.Whitelist)
				assert.False(t, result.Deleted)
				assert.Nil(t, result.DeletionTime)
			},
		},
		{
			name: "deleted record",
			record: &dbclient.ApiKey{
				Id:             2,
				Name:           "deleted-key",
				UserId:         "user-456",
				ApiKey:         "ak-deleted-secret",
				ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
				CreationTime:   pq.NullTime{Time: now.Add(-48 * time.Hour), Valid: true},
				Whitelist:      "[]",
				Deleted:        true,
				DeletionTime:   pq.NullTime{Time: now, Valid: true},
			},
			validate: func(t *testing.T, result view.ApiKeyResponseItem) {
				assert.Equal(t, int64(2), result.Id)
				assert.Equal(t, "deleted-key", result.Name)
				assert.True(t, result.Deleted)
				assert.NotNil(t, result.DeletionTime)
			},
		},
		{
			name: "empty whitelist",
			record: &dbclient.ApiKey{
				Id:             3,
				Name:           "no-whitelist",
				UserId:         "user-789",
				ApiKey:         "ak-secret",
				ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
				CreationTime:   pq.NullTime{Time: now, Valid: true},
				Whitelist:      "",
				Deleted:        false,
			},
			validate: func(t *testing.T, result view.ApiKeyResponseItem) {
				assert.Equal(t, int64(3), result.Id)
				assert.Equal(t, []string{}, result.Whitelist)
			},
		},
		{
			name: "null whitelist JSON",
			record: &dbclient.ApiKey{
				Id:             4,
				Name:           "null-whitelist",
				UserId:         "user-000",
				ApiKey:         "ak-secret",
				ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
				CreationTime:   pq.NullTime{Time: now, Valid: true},
				Whitelist:      "null",
				Deleted:        false,
			},
			validate: func(t *testing.T, result view.ApiKeyResponseItem) {
				assert.Equal(t, []string{}, result.Whitelist)
			},
		},
		{
			name: "empty array whitelist JSON",
			record: &dbclient.ApiKey{
				Id:             5,
				Name:           "empty-array-whitelist",
				UserId:         "user-111",
				ApiKey:         "ak-secret",
				ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
				CreationTime:   pq.NullTime{Time: now, Valid: true},
				Whitelist:      "[]",
				Deleted:        false,
			},
			validate: func(t *testing.T, result view.ApiKeyResponseItem) {
				assert.Equal(t, []string{}, result.Whitelist)
			},
		},
		{
			name: "invalid times",
			record: &dbclient.ApiKey{
				Id:             6,
				Name:           "invalid-times",
				UserId:         "user-222",
				ApiKey:         "ak-secret",
				ExpirationTime: pq.NullTime{Valid: false},
				CreationTime:   pq.NullTime{Valid: false},
				Whitelist:      "[]",
				Deleted:        false,
			},
			validate: func(t *testing.T, result view.ApiKeyResponseItem) {
				assert.Empty(t, result.ExpirationTime)
				assert.Empty(t, result.CreationTime)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToApiKeyResponseItem(tt.record)
			tt.validate(t, result)
		})
	}
}

// TestCreateApiKeyHandler tests the CreateApiKey HTTP handler
func TestCreateApiKeyHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("missing user id returns unauthorized", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		_, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		reqBody := view.CreateApiKeyRequest{
			Name:    "test-key",
			TTLDays: 30,
		}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		// Intentionally not setting common.UserId

		h.CreateApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("empty name returns bad request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		reqBody := view.CreateApiKeyRequest{
			Name:    "",
			TTLDays: 30,
		}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, mockUser.Name)

		h.CreateApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("invalid ttlDays returns bad request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		// Test TTLDays = 0
		reqBody := view.CreateApiKeyRequest{
			Name:    "test-key",
			TTLDays: 0,
		}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, mockUser.Name)

		h.CreateApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("ttlDays exceeds max returns bad request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		reqBody := view.CreateApiKeyRequest{
			Name:    "test-key",
			TTLDays: 400, // Exceeds MaxTTLDays (366)
		}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, mockUser.Name)

		h.CreateApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("invalid whitelist returns bad request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		reqBody := view.CreateApiKeyRequest{
			Name:      "test-key",
			TTLDays:   30,
			Whitelist: []string{"invalid-ip-format"},
		}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, mockUser.Name)

		h.CreateApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful creation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx interface{}, apiKey *dbclient.ApiKey) error {
				apiKey.Id = 123 // Simulate database assigned ID
				return nil
			},
		)

		reqBody := view.CreateApiKeyRequest{
			Name:      "my-api-key",
			TTLDays:   30,
			Whitelist: []string{"192.168.1.0/24"},
		}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, mockUser.Name)

		h.CreateApiKey(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		var response view.CreateApiKeyResponse
		err := json.Unmarshal(rsp.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.NotEmpty(t, response.ApiKey) // API key should be returned on creation
		assert.Equal(t, "my-api-key", response.Name)
		assert.NotEmpty(t, response.ExpirationTime)
		assert.NotEmpty(t, response.CreationTime)
	})

	t.Run("nil db client returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         nil, // No DB client
		}

		reqBody := view.CreateApiKeyRequest{
			Name:    "test-key1",
			TTLDays: 30,
		}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, mockUser.Name)

		h.CreateApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("database insert error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).Return(assert.AnError)

		reqBody := view.CreateApiKeyRequest{
			Name:    "my-api-key",
			TTLDays: 30,
		}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, mockUser.Name)

		h.CreateApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful creation with empty whitelist", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx interface{}, apiKey *dbclient.ApiKey) error {
				apiKey.Id = 124
				return nil
			},
		)

		reqBody := view.CreateApiKeyRequest{
			Name:      "my-api-key",
			TTLDays:   30,
			Whitelist: []string{}, // Empty whitelist
		}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, mockUser.Name)

		h.CreateApiKey(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful creation with CIDR whitelist", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx interface{}, apiKey *dbclient.ApiKey) error {
				apiKey.Id = 125
				return nil
			},
		)

		reqBody := view.CreateApiKeyRequest{
			Name:      "my-api-key",
			TTLDays:   30,
			Whitelist: []string{"10.0.0.0/8", "172.16.0.0/12"},
		}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, mockUser.Name)

		h.CreateApiKey(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})

	t.Run("invalid CIDR in whitelist returns bad request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		reqBody := view.CreateApiKeyRequest{
			Name:      "test-key",
			TTLDays:   30,
			Whitelist: []string{"10.0.0.0/99"}, // Invalid CIDR
		}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, mockUser.Name)

		h.CreateApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}

// TestListApiKeyHandler tests the ListApiKey HTTP handler
func TestListApiKeyHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Now().UTC()

	t.Run("missing user id returns unauthorized", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		_, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys", nil)
		// Intentionally not setting common.UserId

		h.ListApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful list with records", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountApiKeys(gomock.Any(), gomock.Any()).Return(2, nil)
		mockDB.EXPECT().SelectApiKeys(gomock.Any(), gomock.Any(), gomock.Any(), view.DefaultQueryLimit, 0).Return([]*dbclient.ApiKey{
			{
				Id:             1,
				Name:           "key-1",
				UserId:         mockUser.Name,
				ApiKey:         "ak-secret-1",
				ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
				CreationTime:   pq.NullTime{Time: now, Valid: true},
				Whitelist:      "[]",
				Deleted:        false,
			},
			{
				Id:             2,
				Name:           "key-2",
				UserId:         mockUser.Name,
				ApiKey:         "ak-secret-2",
				ExpirationTime: pq.NullTime{Time: now.Add(48 * time.Hour), Valid: true},
				CreationTime:   pq.NullTime{Time: now.Add(-24 * time.Hour), Valid: true},
				Whitelist:      `["192.168.1.1"]`,
				Deleted:        false,
			},
		}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListApiKey(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		var response view.ListApiKeyResponse
		err := json.Unmarshal(rsp.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, 2, response.TotalCount)
		assert.Equal(t, 2, len(response.Items))

		// Verify response items have expected values
		for _, item := range response.Items {
			assert.NotEmpty(t, item.Name)
			assert.NotEmpty(t, item.UserId)
			// ApiKey field is not included in ApiKeyResponseItem struct
		}
	})

	t.Run("successful list with empty results", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountApiKeys(gomock.Any(), gomock.Any()).Return(0, nil)
		mockDB.EXPECT().SelectApiKeys(gomock.Any(), gomock.Any(), gomock.Any(), view.DefaultQueryLimit, 0).Return([]*dbclient.ApiKey{}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListApiKey(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		var response view.ListApiKeyResponse
		err := json.Unmarshal(rsp.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, 0, response.TotalCount)
		assert.Equal(t, 0, len(response.Items))
	})

	t.Run("list with pagination parameters", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountApiKeys(gomock.Any(), gomock.Any()).Return(5, nil)
		mockDB.EXPECT().SelectApiKeys(gomock.Any(), gomock.Any(), gomock.Any(), 2, 2).Return([]*dbclient.ApiKey{
			{
				Id:             3,
				Name:           "key-3",
				UserId:         mockUser.Name,
				ApiKey:         "ak-secret-3",
				ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
				CreationTime:   pq.NullTime{Time: now, Valid: true},
				Whitelist:      "[]",
				Deleted:        false,
			},
		}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys?limit=2&offset=2", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListApiKey(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		var response view.ListApiKeyResponse
		err := json.Unmarshal(rsp.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, 5, response.TotalCount) // Total count from CountApiKeys
		assert.Equal(t, 1, len(response.Items)) // Only 1 item returned in this page
	})

	t.Run("nil db client returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         nil,
		}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("database count error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountApiKeys(gomock.Any(), gomock.Any()).Return(0, assert.AnError)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("database select error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountApiKeys(gomock.Any(), gomock.Any()).Return(5, nil)
		mockDB.EXPECT().SelectApiKeys(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, assert.AnError)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("list with sortBy parameter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountApiKeys(gomock.Any(), gomock.Any()).Return(0, nil)
		mockDB.EXPECT().SelectApiKeys(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return([]*dbclient.ApiKey{}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys?sortBy=expirationTime&order=asc", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListApiKey(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})

	t.Run("list with invalid order returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys?order=invalid", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}

// TestDeleteApiKeyHandler tests the DeleteApiKey HTTP handler
func TestDeleteApiKeyHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Now().UTC()

	t.Run("missing user id returns unauthorized", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		_, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/apikeys/1", nil)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		// Intentionally not setting common.UserId

		h.DeleteApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("empty id returns bad request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/apikeys/", nil)
		c.Params = gin.Params{{Key: "id", Value: ""}}
		c.Set(common.UserId, mockUser.Name)

		h.DeleteApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("invalid id format returns bad request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/apikeys/invalid", nil)
		c.Params = gin.Params{{Key: "id", Value: "invalid"}}
		c.Set(common.UserId, mockUser.Name)

		h.DeleteApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("api key not found returns not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().GetApiKeyById(gomock.Any(), int64(999)).Return(nil, assert.AnError)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/apikeys/999", nil)
		c.Params = gin.Params{{Key: "id", Value: "999"}}
		c.Set(common.UserId, mockUser.Name)

		h.DeleteApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("api key belongs to different user returns forbidden", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().GetApiKeyById(gomock.Any(), int64(1)).Return(&dbclient.ApiKey{
			Id:             1,
			Name:           "other-user-key",
			UserId:         "other-user-id", // Different user
			ApiKey:         "ak-secret",
			ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
			Deleted:        false,
		}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/apikeys/1", nil)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Set(common.UserId, mockUser.Name)

		h.DeleteApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("already deleted api key returns bad request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().GetApiKeyById(gomock.Any(), int64(1)).Return(&dbclient.ApiKey{
			Id:             1,
			Name:           "deleted-key",
			UserId:         mockUser.Name,
			ApiKey:         "ak-secret",
			ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
			Deleted:        true, // Already deleted
		}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/apikeys/1", nil)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Set(common.UserId, mockUser.Name)

		h.DeleteApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful deletion", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().GetApiKeyById(gomock.Any(), int64(1)).Return(&dbclient.ApiKey{
			Id:             1,
			Name:           "my-key",
			UserId:         mockUser.Name,
			ApiKey:         "ak-secret",
			ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
			Deleted:        false,
		}, nil)
		mockDB.EXPECT().SetApiKeyDeleted(gomock.Any(), mockUser.Name, int64(1)).Return(nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/apikeys/1", nil)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Set(common.UserId, mockUser.Name)

		h.DeleteApiKey(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})

	t.Run("nil db client returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         nil,
		}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/apikeys/1", nil)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Set(common.UserId, mockUser.Name)

		h.DeleteApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("database delete error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().GetApiKeyById(gomock.Any(), int64(1)).Return(&dbclient.ApiKey{
			Id:             1,
			Name:           "my-key",
			UserId:         mockUser.Name,
			ApiKey:         "ak-secret",
			ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
			Deleted:        false,
		}, nil)
		mockDB.EXPECT().SetApiKeyDeleted(gomock.Any(), mockUser.Name, int64(1)).Return(assert.AnError)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/apikeys/1", nil)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Set(common.UserId, mockUser.Name)

		h.DeleteApiKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}

// TestParseListApiKeyQuery tests the parseListApiKeyQuery function
func TestParseListApiKeyQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("default values when no params", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys", nil)

		query, err := parseListApiKeyQuery(c)
		assert.NoError(t, err)
		assert.Equal(t, view.DefaultQueryLimit, query.Limit)
		assert.Equal(t, dbclient.DESC, query.Order)
		assert.Equal(t, dbclient.CreateTime, query.SortBy)
	})

	t.Run("custom pagination params", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys?limit=50&offset=10", nil)

		query, err := parseListApiKeyQuery(c)
		assert.NoError(t, err)
		assert.Equal(t, 50, query.Limit)
		assert.Equal(t, 10, query.Offset)
	})

	t.Run("custom sort params", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys?sortBy=ExpirationTime&order=asc", nil)

		query, err := parseListApiKeyQuery(c)
		assert.NoError(t, err)
		assert.Equal(t, "expirationtime", query.SortBy) // Converted to lowercase
		assert.Equal(t, "asc", query.Order)
	})

	t.Run("zero limit uses default", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/apikeys?limit=0", nil)

		query, err := parseListApiKeyQuery(c)
		assert.NoError(t, err)
		assert.Equal(t, view.DefaultQueryLimit, query.Limit)
	})
}

// TestBuildListApiKeyOrderBy tests the buildListApiKeyOrderBy function
func TestBuildListApiKeyOrderBy(t *testing.T) {
	dbTags := dbclient.GetApiKeyFieldTags()

	t.Run("default sort by creation time", func(t *testing.T) {
		req := &view.ListApiKeyRequest{
			Order: dbclient.DESC,
		}
		orderBy := buildListApiKeyOrderBy(req, dbTags)
		assert.Len(t, orderBy, 1)
		assert.Contains(t, orderBy[0], "creation_time")
	})

	t.Run("sort by expiration time with secondary sort", func(t *testing.T) {
		req := &view.ListApiKeyRequest{
			SortBy: "expirationtime",
			Order:  dbclient.ASC,
		}
		orderBy := buildListApiKeyOrderBy(req, dbTags)
		assert.Len(t, orderBy, 2)
		assert.Contains(t, orderBy[0], "expiration_time")
		assert.Contains(t, orderBy[1], "creation_time")
	})

	t.Run("sort by creation time without duplicate", func(t *testing.T) {
		req := &view.ListApiKeyRequest{
			SortBy: "creationtime",
			Order:  dbclient.ASC,
		}
		orderBy := buildListApiKeyOrderBy(req, dbTags)
		assert.Len(t, orderBy, 1)
		assert.Contains(t, orderBy[0], "creation_time")
	})

	t.Run("invalid sort field", func(t *testing.T) {
		req := &view.ListApiKeyRequest{
			SortBy: "invalidfield",
			Order:  dbclient.DESC,
		}
		orderBy := buildListApiKeyOrderBy(req, dbTags)
		// Should have at least creation_time as fallback
		assert.Len(t, orderBy, 1)
		assert.Contains(t, orderBy[0], "creation_time")
	})
}
