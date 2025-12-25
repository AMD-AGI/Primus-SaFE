/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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

	sqrl "github.com/Masterminds/squirrel"
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

// TestCvtToPublicKeyResponse tests the conversion from database PublicKey to response item
func TestCvtToPublicKeyResponse(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		pubKey   *dbclient.PublicKey
		validate func(*testing.T, view.ListPublicKeysResponseItem)
	}{
		{
			name: "complete public key",
			pubKey: &dbclient.PublicKey{
				Id:          101,
				UserId:      "user-123",
				Description: "My SSH key for production",
				PublicKey:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDexample user@host",
				Status:      true,
				CreateTime:  pq.NullTime{Time: now, Valid: true},
				UpdateTime:  pq.NullTime{Time: now.Add(1 * time.Hour), Valid: true},
			},
			validate: func(t *testing.T, result view.ListPublicKeysResponseItem) {
				assert.Equal(t, int64(101), result.Id)
				assert.Equal(t, "user-123", result.UserId)
				assert.Equal(t, "My SSH key for production", result.Description)
				assert.Contains(t, result.PublicKey, "ssh-rsa")
				assert.True(t, result.Status)
				assert.NotEmpty(t, result.CreateTime)
				assert.NotEmpty(t, result.UpdateTime)
			},
		},
		{
			name: "public key without times",
			pubKey: &dbclient.PublicKey{
				Id:          202,
				UserId:      "user-456",
				Description: "Development key",
				PublicKey:   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAexample dev@machine",
				Status:      true,
				CreateTime:  pq.NullTime{Valid: false},
				UpdateTime:  pq.NullTime{Valid: false},
			},
			validate: func(t *testing.T, result view.ListPublicKeysResponseItem) {
				assert.Equal(t, int64(202), result.Id)
				assert.Equal(t, "user-456", result.UserId)
				assert.Equal(t, "Development key", result.Description)
				assert.Contains(t, result.PublicKey, "ssh-ed25519")
				assert.True(t, result.Status)
				assert.Empty(t, result.CreateTime)
				assert.Empty(t, result.UpdateTime)
			},
		},
		{
			name: "disabled public key",
			pubKey: &dbclient.PublicKey{
				Id:          303,
				UserId:      "user-789",
				Description: "Old key - disabled",
				PublicKey:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABold user@oldhost",
				Status:      false, // Disabled
				CreateTime:  pq.NullTime{Time: now.Add(-30 * 24 * time.Hour), Valid: true},
				UpdateTime:  pq.NullTime{Time: now, Valid: true},
			},
			validate: func(t *testing.T, result view.ListPublicKeysResponseItem) {
				assert.Equal(t, int64(303), result.Id)
				assert.Equal(t, "user-789", result.UserId)
				assert.Equal(t, "Old key - disabled", result.Description)
				assert.False(t, result.Status) // Disabled
			},
		},
		{
			name: "public key with empty description",
			pubKey: &dbclient.PublicKey{
				Id:          404,
				UserId:      "user-000",
				Description: "",
				PublicKey:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAgen user@host",
				Status:      true,
				CreateTime:  pq.NullTime{Time: now, Valid: true},
				UpdateTime:  pq.NullTime{Time: now, Valid: true},
			},
			validate: func(t *testing.T, result view.ListPublicKeysResponseItem) {
				assert.Equal(t, int64(404), result.Id)
				assert.Empty(t, result.Description)
				assert.True(t, result.Status)
			},
		},
		{
			name: "ECDSA public key",
			pubKey: &dbclient.PublicKey{
				Id:          505,
				UserId:      "user-ecdsa",
				Description: "ECDSA key for automation",
				PublicKey:   "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYexample automation@server",
				Status:      true,
				CreateTime:  pq.NullTime{Time: now, Valid: true},
				UpdateTime:  pq.NullTime{Time: now, Valid: true},
			},
			validate: func(t *testing.T, result view.ListPublicKeysResponseItem) {
				assert.Equal(t, int64(505), result.Id)
				assert.Equal(t, "user-ecdsa", result.UserId)
				assert.Contains(t, result.PublicKey, "ecdsa-sha2-nistp256")
				assert.Equal(t, "ECDSA key for automation", result.Description)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToPublicKeyResponse(tt.pubKey)
			tt.validate(t, result)
		})
	}
}

// TestParseListPublicKeyQuery tests parsing of list public keys query parameters
func TestParseListPublicKeyQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		queryParams string
		validate    func(*testing.T, *view.ListPublicKeysRequest, error)
	}{
		{
			name:        "default values when no params",
			queryParams: "",
			validate: func(t *testing.T, query *view.ListPublicKeysRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, view.DefaultQueryLimit, query.Limit)
				assert.Equal(t, dbclient.DESC, query.Order)
				assert.Equal(t, dbclient.CreateTime, query.SortBy)
			},
		},
		{
			name:        "custom limit and offset",
			queryParams: "?limit=50&offset=10",
			validate: func(t *testing.T, query *view.ListPublicKeysRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 50, query.Limit)
				assert.Equal(t, 10, query.Offset)
			},
		},
		{
			name:        "ascending order",
			queryParams: "?order=asc",
			validate: func(t *testing.T, query *view.ListPublicKeysRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "asc", query.Order)
			},
		},
		{
			name:        "descending order",
			queryParams: "?order=desc",
			validate: func(t *testing.T, query *view.ListPublicKeysRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "desc", query.Order)
			},
		},
		{
			name:        "custom sortBy",
			queryParams: "?sortBy=UpdateTime",
			validate: func(t *testing.T, query *view.ListPublicKeysRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "updatetime", query.SortBy) // Should be lowercased
			},
		},
		{
			name:        "invalid order value",
			queryParams: "?order=invalid",
			validate: func(t *testing.T, query *view.ListPublicKeysRequest, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid query")
			},
		},
		{
			name:        "zero limit uses default",
			queryParams: "?limit=0",
			validate: func(t *testing.T, query *view.ListPublicKeysRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, view.DefaultQueryLimit, query.Limit)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsp := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rsp)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public-keys"+tt.queryParams, nil)

			query, err := parseListPublicKeyQuery(c)
			tt.validate(t, query, err)
		})
	}
}

// TestCvtToListPublicKeysSql tests conversion of query to SQL conditions
func TestCvtToListPublicKeysSql(t *testing.T) {
	tests := []struct {
		name     string
		query    *view.ListPublicKeysRequest
		validate func(*testing.T, sqrl.Sqlizer, []string, error)
	}{
		{
			name: "query with userId",
			query: &view.ListPublicKeysRequest{
				UserId: "test-user",
				Order:  dbclient.DESC,
				SortBy: dbclient.CreateTime,
			},
			validate: func(t *testing.T, sql sqrl.Sqlizer, orderBy []string, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, sql)
				assert.NotEmpty(t, orderBy)
			},
		},
		{
			name: "query with empty userId",
			query: &view.ListPublicKeysRequest{
				UserId: "",
				Order:  dbclient.DESC,
				SortBy: dbclient.CreateTime,
			},
			validate: func(t *testing.T, sql sqrl.Sqlizer, orderBy []string, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, sql)
			},
		},
		{
			name: "query with whitespace userId",
			query: &view.ListPublicKeysRequest{
				UserId: "   ",
				Order:  dbclient.DESC,
				SortBy: dbclient.CreateTime,
			},
			validate: func(t *testing.T, sql sqrl.Sqlizer, orderBy []string, err error) {
				assert.NoError(t, err)
				// Whitespace userId should be trimmed
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, orderBy, err := cvtToListPublicKeysSql(tt.query)
			tt.validate(t, sql, orderBy, err)
		})
	}
}

// TestBuildListPublicKeysOrderBy tests order by clause construction
func TestBuildListPublicKeysOrderBy(t *testing.T) {
	dbTags := dbclient.GetPublicKeyFieldTags()

	tests := []struct {
		name     string
		query    *view.ListPublicKeysRequest
		validate func(*testing.T, []string)
	}{
		{
			name: "custom sortBy field",
			query: &view.ListPublicKeysRequest{
				Order:  dbclient.DESC,
				SortBy: "updatetime",
			},
			validate: func(t *testing.T, orderBy []string) {
				assert.NotEmpty(t, orderBy)
				// Should have createtime as secondary sort
				assert.GreaterOrEqual(t, len(orderBy), 1)
			},
		},
		{
			name: "empty sortBy uses default",
			query: &view.ListPublicKeysRequest{
				Order:  dbclient.DESC,
				SortBy: "",
			},
			validate: func(t *testing.T, orderBy []string) {
				// Should still have createtime order
				assert.NotEmpty(t, orderBy)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orderBy := buildListPublicKeysOrderBy(tt.query, dbTags)
			tt.validate(t, orderBy)
		})
	}
}

// TestDeletePublicKeyHandler tests the DeletePublicKey HTTP handler
func TestDeletePublicKeyHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("invalid id returns error", func(t *testing.T) {
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
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/public-keys/invalid", nil)
		c.Params = gin.Params{{Key: "id", Value: "invalid"}}
		c.Set(common.UserId, mockUser.Name)

		h.DeletePublicKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}

// TestSetPublicKeyStatusHandler tests the SetPublicKeyStatus HTTP handler
func TestSetPublicKeyStatusHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("empty id returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		reqBody := view.SetPublicKeyStatusRequest{Status: true}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/public-keys//status", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: ""}}
		c.Set(common.UserId, mockUser.Name)

		h.SetPublicKeyStatus(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}

// TestSetPublicKeyDescriptionHandler tests the SetPublicKeyDescription HTTP handler
func TestSetPublicKeyDescriptionHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("empty id returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		reqBody := view.SetPublicKeyDescriptionRequest{Description: "Test"}
		body, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/public-keys//description", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: ""}}
		c.Set(common.UserId, mockUser.Name)

		h.SetPublicKeyDescription(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}
