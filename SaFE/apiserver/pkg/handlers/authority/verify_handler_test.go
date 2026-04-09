/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const testInternalToken = "test-internal-secret-token"

// verifyResponse is a helper struct for parsing VerifyToken JSON response
type verifyResponse struct {
	Code int                 `json:"code"`
	Data VerifyTokenResponse `json:"data"`
}

// callVerifyToken creates a gin test context, sets up the request, and calls the handler
func callVerifyToken(t *testing.T, req VerifyTokenRequest, internalToken string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	body, err := json.Marshal(req)
	assert.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/verify", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if internalToken != "" {
		c.Request.Header.Set(InternalAuthTokenHeader, internalToken)
	}

	VerifyToken(c)
	return w
}

// setupInternalAuth sets up a test InternalAuth and returns a cleanup function
func setupInternalAuth(t *testing.T) func() {
	t.Helper()
	old := internalAuthInstance
	internalAuthInstance = &InternalAuth{token: testInternalToken}
	return func() { internalAuthInstance = old }
}

// setupDefaultToken creates a defaultToken with crypto disabled for testing
func setupDefaultToken(t *testing.T) func() {
	t.Helper()
	commonconfig.SetValue("crypto.enable", "false")
	commonconfig.SetValue("user.token.expire", "-1")

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	old := defaultTokenInstance
	defaultTokenInstance = &defaultToken{Client: cli}
	return func() {
		defaultTokenInstance = old
		commonconfig.SetValue("crypto.enable", "")
		commonconfig.SetValue("user.token.expire", "")
	}
}

// generateTestToken generates a valid encoded default token for testing
func generateTestToken(t *testing.T, userId, username string) string {
	t.Helper()
	expire := time.Now().Add(time.Hour).Unix()
	raw, err := generateDefaultToken(userId, expire, username)
	assert.NoError(t, err)
	return stringutil.Base64Encode(raw)
}

// --- VerifyToken handler tests ---

func TestVerifyToken_InternalAuthNotInitialized(t *testing.T) {
	old := internalAuthInstance
	internalAuthInstance = nil
	defer func() { internalAuthInstance = old }()

	w := callVerifyToken(t, VerifyTokenRequest{Cookie: "Token=xxx"}, "any-token")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestVerifyToken_InvalidInternalToken(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	w := callVerifyToken(t, VerifyTokenRequest{Cookie: "Token=xxx"}, "wrong-token")
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestVerifyToken_MissingInternalToken(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	w := callVerifyToken(t, VerifyTokenRequest{Cookie: "Token=xxx"}, "")
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestVerifyToken_InvalidRequestBody(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/verify", bytes.NewReader([]byte("not-json")))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set(InternalAuthTokenHeader, testInternalToken)
	VerifyToken(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVerifyToken_NoAuthProvided(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	w := callVerifyToken(t, VerifyTokenRequest{}, testInternalToken)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "cookie, apiKey, or authorization is required")
}

func TestVerifyToken_CookieAuth_Success(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()
	cleanupToken := setupDefaultToken(t)
	defer cleanupToken()

	token := generateTestToken(t, "user-100", "alice")

	t.Run("without userType in cookie", func(t *testing.T) {
		w := callVerifyToken(t, VerifyTokenRequest{
			Cookie: "Token=" + token,
		}, testInternalToken)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp verifyResponse
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.Code)
		assert.Equal(t, "user-100", resp.Data.Id)
		assert.Equal(t, "alice", resp.Data.Name)
		assert.Empty(t, resp.Data.Type, "userType not in cookie so Type should be empty")
	})

	t.Run("with userType=default in cookie", func(t *testing.T) {
		w := callVerifyToken(t, VerifyTokenRequest{
			Cookie: "Token=" + token + "; userType=default",
		}, testInternalToken)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp verifyResponse
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "user-100", resp.Data.Id)
		assert.Equal(t, string(v1.DefaultUserType), resp.Data.Type)
	})
}

func TestVerifyToken_CookieAuth_InvalidFormat(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	w := callVerifyToken(t, VerifyTokenRequest{
		Cookie: "NoTokenHere=value",
	}, testInternalToken)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid cookie format")
}

func TestVerifyToken_CookieAuth_InvalidToken(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()
	cleanupToken := setupDefaultToken(t)
	defer cleanupToken()

	w := callVerifyToken(t, VerifyTokenRequest{
		Cookie: "Token=invalid-token-value",
	}, testInternalToken)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyToken_CookieAuth_SSONotEnabled(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	commonconfig.SetValue("sso.enable", "false")
	defer commonconfig.SetValue("sso.enable", "")

	w := callVerifyToken(t, VerifyTokenRequest{
		Cookie: "Token=some-token; userType=sso",
	}, testInternalToken)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "SSO is not enabled")
}

func TestVerifyToken_ApiKey_Success(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	oldApiKey := apiKeyTokenInstance
	apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB}
	defer func() { apiKeyTokenInstance = oldApiKey }()

	testKey := "ak-test-verify-key-001"
	now := time.Now().UTC()
	mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testKey, nil)).Return(&dbclient.ApiKey{
		Id:             42,
		UserId:         "user-ak-1",
		UserName:       "charlie",
		Deleted:        false,
		ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
		Whitelist:      "[]",
	}, nil)

	w := callVerifyToken(t, VerifyTokenRequest{
		ApiKey:   testKey,
		ClientIP: "10.0.0.1",
	}, testInternalToken)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp verifyResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, "user-ak-1", resp.Data.Id)
	assert.Equal(t, "charlie", resp.Data.Name)
	assert.Equal(t, UserTypeApiKey, resp.Data.Type)
	assert.Equal(t, int64(42), resp.Data.ApiKeyId)
}

func TestVerifyToken_ApiKey_Invalid(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	oldApiKey := apiKeyTokenInstance
	apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB}
	defer func() { apiKeyTokenInstance = oldApiKey }()

	testKey := "ak-bad-key"
	mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testKey, nil)).Return(nil, assert.AnError)

	w := callVerifyToken(t, VerifyTokenRequest{
		ApiKey: testKey,
	}, testInternalToken)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "invalid API key")
}

func TestVerifyToken_ApiKey_NotInitialized(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	oldApiKey := apiKeyTokenInstance
	apiKeyTokenInstance = nil
	defer func() { apiKeyTokenInstance = oldApiKey }()

	w := callVerifyToken(t, VerifyTokenRequest{
		ApiKey: "ak-some-key",
	}, testInternalToken)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "API key authentication not available")
}

func TestVerifyToken_AuthorizationApiKey_Success(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	oldApiKey := apiKeyTokenInstance
	apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB}
	defer func() { apiKeyTokenInstance = oldApiKey }()

	testKey := "ak-bearer-key-002"
	now := time.Now().UTC()
	mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testKey, nil)).Return(&dbclient.ApiKey{
		Id:             99,
		UserId:         "user-ak-2",
		UserName:       "diana",
		Deleted:        false,
		ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
		Whitelist:      "[]",
	}, nil)

	w := callVerifyToken(t, VerifyTokenRequest{
		Authorization: "Bearer " + testKey,
		ClientIP:      "192.168.1.50",
	}, testInternalToken)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp verifyResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "user-ak-2", resp.Data.Id)
	assert.Equal(t, UserTypeApiKey, resp.Data.Type)
	assert.Equal(t, int64(99), resp.Data.ApiKeyId)
}

func TestVerifyToken_AuthorizationBearerToken_Success(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()
	cleanupToken := setupDefaultToken(t)
	defer cleanupToken()

	token := generateTestToken(t, "user-300", "eve")
	w := callVerifyToken(t, VerifyTokenRequest{
		Authorization: "Bearer " + token,
	}, testInternalToken)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp verifyResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "user-300", resp.Data.Id)
	assert.Equal(t, "eve", resp.Data.Name)
	assert.Equal(t, string(v1.DefaultUserType), resp.Data.Type)
}

func TestVerifyToken_AuthorizationBearerToken_WithUserType(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()
	cleanupToken := setupDefaultToken(t)
	defer cleanupToken()

	token := generateTestToken(t, "user-400", "frank")
	w := callVerifyToken(t, VerifyTokenRequest{
		Authorization: "Bearer " + token,
		UserType:      "default",
	}, testInternalToken)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp verifyResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "user-400", resp.Data.Id)
	assert.Equal(t, "default", resp.Data.Type)
}

func TestVerifyToken_AuthorizationBearerToken_InvalidToken(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()
	cleanupToken := setupDefaultToken(t)
	defer cleanupToken()

	w := callVerifyToken(t, VerifyTokenRequest{
		Authorization: "Bearer bad-token",
	}, testInternalToken)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyToken_AuthorizationBearerToken_EmptyToken(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	w := callVerifyToken(t, VerifyTokenRequest{
		Authorization: "InvalidFormat",
	}, testInternalToken)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyToken_CookiePriorityOverApiKey(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()
	cleanupToken := setupDefaultToken(t)
	defer cleanupToken()

	token := generateTestToken(t, "user-cookie", "cookie-user")
	w := callVerifyToken(t, VerifyTokenRequest{
		Cookie: "Token=" + token,
		ApiKey: "ak-should-be-ignored",
	}, testInternalToken)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp verifyResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "user-cookie", resp.Data.Id)
}

func TestVerifyToken_ApiKeyPriorityOverAuthorization(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	oldApiKey := apiKeyTokenInstance
	apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB}
	defer func() { apiKeyTokenInstance = oldApiKey }()

	testKey := "ak-priority-key"
	now := time.Now().UTC()
	mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testKey, nil)).Return(&dbclient.ApiKey{
		Id:             10,
		UserId:         "user-apikey",
		UserName:       "apikey-user",
		Deleted:        false,
		ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
		Whitelist:      "[]",
	}, nil)

	w := callVerifyToken(t, VerifyTokenRequest{
		ApiKey:        testKey,
		Authorization: "Bearer should-be-ignored",
	}, testInternalToken)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp verifyResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "user-apikey", resp.Data.Id)
	assert.Equal(t, UserTypeApiKey, resp.Data.Type)
}

// --- parseCookieString tests ---

func TestParseCookieString(t *testing.T) {
	tests := []struct {
		name      string
		cookie    string
		wantToken string
		wantType  string
		wantErr   bool
	}{
		{
			name:      "token only",
			cookie:    "Token=my-token-value",
			wantToken: "my-token-value",
			wantType:  "",
		},
		{
			name:      "token and userType",
			cookie:    "Token=my-token; userType=sso",
			wantToken: "my-token",
			wantType:  "sso",
		},
		{
			name:      "extra whitespace",
			cookie:    "  Token = abc123 ;  userType = default  ",
			wantToken: "abc123",
			wantType:  "default",
		},
		{
			name:      "extra cookies ignored",
			cookie:    "Token=val; other=ignored; userType=sso; foo=bar",
			wantToken: "val",
			wantType:  "sso",
		},
		{
			name:    "no token key",
			cookie:  "userType=sso; other=value",
			wantErr: true,
		},
		{
			name:    "empty string",
			cookie:  "",
			wantErr: true,
		},
		{
			name:      "malformed entries skipped",
			cookie:    "Token=good; badentry; =nokey; userType=default",
			wantToken: "good",
			wantType:  "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, userType, err := parseCookieString(tt.cookie)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantToken, token)
				assert.Equal(t, tt.wantType, userType)
			}
		})
	}
}

// --- extractBearerToken tests ---

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"valid bearer", "Bearer my-token", "my-token"},
		{"lowercase bearer", "bearer my-token", "my-token"},
		{"empty", "", ""},
		{"no space", "Bearertoken", ""},
		{"wrong scheme", "Basic dXNlcjpwYXNz", ""},
		{"too many parts", "Bearer token extra", ""},
		{"only scheme", "Bearer", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBearerToken(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- verifyApiKey tests ---

func TestVerifyApiKey_NotInitialized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldApiKey := apiKeyTokenInstance
	apiKeyTokenInstance = nil
	defer func() { apiKeyTokenInstance = oldApiKey }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)

	userInfo, err := verifyApiKey(c, "ak-any-key", "10.0.0.1")
	assert.Error(t, err)
	assert.Nil(t, userInfo)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestVerifyApiKey_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	oldApiKey := apiKeyTokenInstance
	apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB}
	defer func() { apiKeyTokenInstance = oldApiKey }()

	testKey := "ak-verify-func-key"
	now := time.Now().UTC()
	mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testKey, nil)).Return(&dbclient.ApiKey{
		Id:             7,
		UserId:         "user-vk",
		UserName:       "vk-user",
		Deleted:        false,
		ExpirationTime: pq.NullTime{Time: now.Add(time.Hour), Valid: true},
		Whitelist:      "[]",
	}, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
	c.Request.RemoteAddr = "10.0.0.1:12345"

	userInfo, err := verifyApiKey(c, testKey, "10.0.0.1")
	assert.NoError(t, err)
	assert.NotNil(t, userInfo)
	assert.Equal(t, "user-vk", userInfo.Id)
	assert.Equal(t, int64(7), userInfo.ApiKeyId)
}

func TestVerifyApiKey_InvalidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	oldApiKey := apiKeyTokenInstance
	apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB}
	defer func() { apiKeyTokenInstance = oldApiKey }()

	testKey := "ak-bad-verify-key"
	mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testKey, nil)).Return(nil, assert.AnError)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
	c.Request.RemoteAddr = "10.0.0.1:12345"

	userInfo, err := verifyApiKey(c, testKey, "10.0.0.1")
	assert.Error(t, err)
	assert.Nil(t, userInfo)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
