/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestParseToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Now().UTC()

	t.Run("api key authentication success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockDB := mock_client.NewMockInterface(ctrl)

		// Set up singleton
		oldInstance := apiKeyTokenInstance
		apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB}
		defer func() { apiKeyTokenInstance = oldInstance }()

		testApiKey := "ak-valid-test-key-for-parse"
		mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testApiKey, nil)).Return(&dbclient.ApiKey{
			Id:             1,
			UserId:         "user-parse-123",
			ApiKey:         HashApiKey(testApiKey, nil),
			Deleted:        false,
			ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
			Whitelist:      "[]",
		}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
		c.Request.Header.Set("Authorization", "Bearer "+testApiKey)
		c.Request.RemoteAddr = "192.168.1.100:12345"

		err := ParseToken(c)
		assert.NoError(t, err)
		assert.Equal(t, "user-parse-123", c.GetString(common.UserId))
	})

	t.Run("api key authentication failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockDB := mock_client.NewMockInterface(ctrl)

		oldInstance := apiKeyTokenInstance
		apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB}
		defer func() { apiKeyTokenInstance = oldInstance }()

		testApiKey := "ak-invalid-key-parse"
		mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testApiKey, nil)).Return(nil, assert.AnError)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
		c.Request.Header.Set("Authorization", "Bearer "+testApiKey)
		c.Request.RemoteAddr = "192.168.1.100:12345"

		err := ParseToken(c)
		assert.Error(t, err)
	})

	t.Run("no token present returns error", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

		err := ParseToken(c)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token not present")
	})

	t.Run("regular token with no token instance", func(t *testing.T) {
		// Clear token instances
		oldDefault := defaultTokenInstance
		oldSSO := ssoInstance
		defaultTokenInstance = nil
		ssoInstance = nil
		defer func() {
			defaultTokenInstance = oldDefault
			ssoInstance = oldSSO
		}()

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
		c.Request.Header.Set("Authorization", "Bearer some-regular-token")

		err := ParseToken(c)
		assert.Error(t, err)
	})
}

func TestParseApiKeyFromRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Now().UTC()

	t.Run("api key token instance not initialized", func(t *testing.T) {
		// Reset singleton for testing
		oldInstance := apiKeyTokenInstance
		apiKeyTokenInstance = nil
		defer func() { apiKeyTokenInstance = oldInstance }()

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

		err := parseApiKeyFromRequest(c, "ak-test-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})

	t.Run("valid api key sets user info in context", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockDB := mock_client.NewMockInterface(ctrl)

		// Create test instance
		oldInstance := apiKeyTokenInstance
		apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB}
		defer func() { apiKeyTokenInstance = oldInstance }()

		testApiKey := "ak-valid-test-key"
		mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testApiKey, nil)).Return(&dbclient.ApiKey{
			Id:             1,
			UserId:         "user-123",
			ApiKey:         HashApiKey(testApiKey, nil),
			Deleted:        false,
			ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
			Whitelist:      "[]",
		}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
		c.Request.RemoteAddr = "192.168.1.100:12345"

		err := parseApiKeyFromRequest(c, testApiKey)
		assert.NoError(t, err)
		assert.Equal(t, "user-123", c.GetString(common.UserId))
		assert.Equal(t, UserTypeApiKey, c.GetString(common.UserType))
	})

	t.Run("invalid api key returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockDB := mock_client.NewMockInterface(ctrl)

		oldInstance := apiKeyTokenInstance
		apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB}
		defer func() { apiKeyTokenInstance = oldInstance }()

		testApiKey := "ak-invalid-key"
		mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testApiKey, nil)).Return(nil, assert.AnError)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
		c.Request.RemoteAddr = "192.168.1.100:12345"

		err := parseApiKeyFromRequest(c, testApiKey)
		assert.Error(t, err)
	})
}

func TestExtractTokenAndUserType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("token from cookie", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
		c.Request.AddCookie(&http.Cookie{Name: CookieToken, Value: "test-token"})
		c.Request.AddCookie(&http.Cookie{Name: common.UserType, Value: "default"})

		token, userType, err := extractTokenAndUserType(c)
		assert.NoError(t, err)
		assert.Equal(t, "test-token", token)
		assert.Equal(t, "default", userType)
	})

	t.Run("token from bearer header", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
		c.Request.Header.Set("Authorization", "Bearer test-bearer-token")
		c.Request.Header.Set("UserType", "sso")

		token, userType, err := extractTokenAndUserType(c)
		assert.NoError(t, err)
		assert.Equal(t, "test-bearer-token", token)
		assert.Equal(t, "sso", userType)
	})

	t.Run("no token returns error", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

		_, _, err := extractTokenAndUserType(c)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token not present")
	})
}

func TestGetBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		authHeader string
		expected   string
	}{
		{
			name:       "valid bearer token",
			authHeader: "Bearer test-token-123",
			expected:   "test-token-123",
		},
		{
			name:       "bearer lowercase",
			authHeader: "bearer test-token-456",
			expected:   "test-token-456",
		},
		{
			name:       "empty header",
			authHeader: "",
			expected:   "",
		},
		{
			name:       "invalid format - no space",
			authHeader: "Bearertoken",
			expected:   "",
		},
		{
			name:       "invalid format - wrong prefix",
			authHeader: "Basic dXNlcjpwYXNz",
			expected:   "",
		},
		{
			name:       "too many parts",
			authHeader: "Bearer token extra",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsp := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rsp)
			c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				c.Request.Header.Set("Authorization", tt.authHeader)
			}

			result := getBearerToken(c)
			assert.Equal(t, tt.expected, result)
		})
	}
}
