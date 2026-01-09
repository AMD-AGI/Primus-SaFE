/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	mockdb "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/agiledragon/gomonkey/v2"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
)

// createMockIDToken creates an oidc.IDToken with claims set using unsafe
func createMockIDToken() *oidc.IDToken {
	userClaims := map[string]interface{}{
		"sub":   "user-sub-123",
		"name":  "Test User",
		"email": "test@example.com",
		"exp":   float64(time.Now().Add(time.Hour).Unix()),
	}
	claimsBytes, _ := json.Marshal(userClaims)

	// Create IDToken and set unexported 'claims' field using unsafe
	idToken := &oidc.IDToken{
		Subject: "user-sub-123",
		Expiry:  time.Now().Add(time.Hour),
	}

	// Use reflection to find and set the unexported 'claims' field
	val := reflect.ValueOf(idToken).Elem()
	claimsField := val.FieldByName("claims")
	if claimsField.IsValid() {
		// Make the field settable using unsafe
		claimsFieldPtr := unsafe.Pointer(claimsField.UnsafeAddr())
		*(*[]byte)(claimsFieldPtr) = claimsBytes
	}

	return idToken
}

func TestGenerateSSOUserId(t *testing.T) {
	tests := []struct {
		name     string
		sub      string
		email    string
		expected bool // whether result should be non-empty
	}{
		{
			name:     "email takes priority",
			sub:      "sub123",
			email:    "test@example.com",
			expected: true,
		},
		{
			name:     "both empty returns empty",
			sub:      "",
			email:    "",
			expected: false,
		},
		{
			name:     "only email",
			sub:      "",
			email:    "test@example.com",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSSOUserId(tt.sub, tt.email)
			if tt.expected {
				assert.NotEmpty(t, result)
			} else {
				assert.Empty(t, result)
			}
		})
	}
}

func TestGenerateSSOUserIdConsistency(t *testing.T) {
	// Same input should produce same output
	email := "test@example.com"
	id1 := generateSSOUserId("", email)
	id2 := generateSSOUserId("", email)
	assert.Equal(t, id1, id2)

	// Different inputs should produce different outputs
	id3 := generateSSOUserId("", "other@example.com")
	assert.NotEqual(t, id1, id3)
}

func TestSSOInstance(t *testing.T) {
	// Reset singleton for test
	ssoInitOnce = sync.Once{}
	originalInstance := ssoInstance
	ssoInstance = nil
	defer func() {
		ssoInstance = originalInstance
	}()

	// Before initialization, should return nil
	inst := SSOInstance()
	assert.Nil(t, inst)
}

func TestSynchronizeUser(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	tests := []struct {
		name         string
		existingUser *v1.User
		userInfo     *UserInfo
		expectCreate bool
		expectUpdate bool
	}{
		{
			name:         "create new user",
			existingUser: nil,
			userInfo: &UserInfo{
				Id:    "newuser123",
				Name:  "New User",
				Email: "new@example.com",
			},
			expectCreate: true,
			expectUpdate: false,
		},
		{
			name: "update existing user name",
			existingUser: &v1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name: "existinguser",
					Annotations: map[string]string{
						v1.UserNameAnnotation:  "Old Name",
						v1.UserEmailAnnotation: "old@example.com",
					},
				},
				Spec: v1.UserSpec{Type: v1.SSOUserType},
			},
			userInfo: &UserInfo{
				Id:    "existinguser",
				Name:  "New Name",
				Email: "old@example.com",
			},
			expectCreate: false,
			expectUpdate: true,
		},
		{
			name: "no update when unchanged",
			existingUser: &v1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name: "existinguser",
					Annotations: map[string]string{
						v1.UserNameAnnotation:  "Same Name",
						v1.UserEmailAnnotation: "same@example.com",
					},
				},
				Spec: v1.UserSpec{Type: v1.SSOUserType},
			},
			userInfo: &UserInfo{
				Id:    "existinguser",
				Name:  "Same Name",
				Email: "same@example.com",
			},
			expectCreate: false,
			expectUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.existingUser != nil {
				builder = builder.WithObjects(tt.existingUser)
			}
			fakeClient := builder.Build()

			token := &ssoToken{Client: fakeClient}
			user, err := token.synchronizeUser(context.Background(), tt.userInfo)

			assert.NoError(t, err)
			assert.NotNil(t, user)
			assert.Equal(t, tt.userInfo.Id, user.Name)
		})
	}
}

func TestLoginValidation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	token := &ssoToken{
		Client:     fakeClient,
		httpClient: nil, // Will fail validation
	}

	// Test empty code
	_, _, err := token.Login(context.Background(), TokenInput{Code: ""})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no code in request")

	// Test nil httpClient
	_, _, err = token.Login(context.Background(), TokenInput{Code: "test-code"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "http client is nil")
}

func TestLoginSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	// Disable DB for this test
	commonconfig.SetValue("db.enable", "false")
	defer commonconfig.SetValue("db.enable", "")

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create non-nil mock objects using reflection (to avoid nil pointer issues)
	mockProvider := reflect.New(reflect.TypeOf(oidc.Provider{})).Interface().(*oidc.Provider)
	mockVerifier := reflect.New(reflect.TypeOf(oidc.IDTokenVerifier{})).Interface().(*oidc.IDTokenVerifier)

	token := &ssoToken{
		Client:       fakeClient,
		httpClient:   httpclient.NewClient(),
		clientId:     "test-client-id",
		clientSecret: "test-secret",
		redirectURI:  "http://localhost/callback",
		provider:     mockProvider,
		verifier:     mockVerifier,
	}

	// Mock oauth2 token with id_token
	mockOAuth2Token := &oauth2.Token{
		AccessToken:  "mock-access-token",
		TokenType:    "Bearer",
		RefreshToken: "mock-refresh-token",
		Expiry:       time.Now().Add(time.Hour),
	}
	mockOAuth2Token = mockOAuth2Token.WithExtra(map[string]interface{}{
		"id_token": "mock-id-token",
	})

	// Create mock IDToken with claims already set
	mockIDToken := createMockIDToken()

	// 1. Mock (*oidc.Provider).Endpoint
	patches := gomonkey.ApplyMethod(reflect.TypeOf(mockProvider), "Endpoint",
		func(_ *oidc.Provider) oauth2.Endpoint {
			return oauth2.Endpoint{
				AuthURL:  "http://mock/auth",
				TokenURL: "http://mock/token",
			}
		})
	defer patches.Reset()

	// 2. Mock (*oauth2.Config).Exchange to return mock token
	patches.ApplyMethod(reflect.TypeOf(&oauth2.Config{}), "Exchange",
		func(_ *oauth2.Config, _ context.Context, _ string, _ ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
			return mockOAuth2Token, nil
		})

	// 3. Mock (*oidc.IDTokenVerifier).Verify to return mock ID token with claims
	patches.ApplyMethod(reflect.TypeOf(mockVerifier), "Verify",
		func(_ *oidc.IDTokenVerifier, _ context.Context, _ string) (*oidc.IDToken, error) {
			return mockIDToken, nil
		})

	// Execute
	user, resp, err := token.Login(context.Background(), TokenInput{Code: "test-code"})

	// Verify
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.NotNil(t, resp)
	assert.Equal(t, "mock-id-token", resp.Token)
}

func TestLoginExchangeError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create non-nil mock provider
	mockProvider := reflect.New(reflect.TypeOf(oidc.Provider{})).Interface().(*oidc.Provider)

	token := &ssoToken{
		Client:       fakeClient,
		httpClient:   httpclient.NewClient(),
		clientId:     "test-client-id",
		clientSecret: "test-secret",
		redirectURI:  "http://localhost/callback",
		provider:     mockProvider,
	}

	// 1. Mock (*oidc.Provider).Endpoint
	patches := gomonkey.ApplyMethod(reflect.TypeOf(mockProvider), "Endpoint",
		func(_ *oidc.Provider) oauth2.Endpoint {
			return oauth2.Endpoint{
				AuthURL:  "http://mock/auth",
				TokenURL: "http://mock/token",
			}
		})
	defer patches.Reset()

	// 2. Mock (*oauth2.Config).Exchange to return error
	patches.ApplyMethod(reflect.TypeOf(&oauth2.Config{}), "Exchange",
		func(_ *oauth2.Config, _ context.Context, _ string, _ ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
			return nil, assert.AnError
		})

	// Execute
	user, resp, err := token.Login(context.Background(), TokenInput{Code: "invalid-code"})

	// Verify
	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "failed to get token")
}

func TestLoginNoIdToken(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create non-nil mock provider
	mockProvider := reflect.New(reflect.TypeOf(oidc.Provider{})).Interface().(*oidc.Provider)

	token := &ssoToken{
		Client:       fakeClient,
		httpClient:   httpclient.NewClient(),
		clientId:     "test-client-id",
		clientSecret: "test-secret",
		redirectURI:  "http://localhost/callback",
		provider:     mockProvider,
	}

	// Token without id_token
	mockOAuth2Token := &oauth2.Token{
		AccessToken: "mock-access-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}

	// 1. Mock (*oidc.Provider).Endpoint
	patches := gomonkey.ApplyMethod(reflect.TypeOf(mockProvider), "Endpoint",
		func(_ *oidc.Provider) oauth2.Endpoint {
			return oauth2.Endpoint{
				AuthURL:  "http://mock/auth",
				TokenURL: "http://mock/token",
			}
		})
	defer patches.Reset()

	// 2. Mock (*oauth2.Config).Exchange - return token without id_token
	patches.ApplyMethod(reflect.TypeOf(&oauth2.Config{}), "Exchange",
		func(_ *oauth2.Config, _ context.Context, _ string, _ ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
			return mockOAuth2Token, nil
		})

	// Execute
	user, resp, err := token.Login(context.Background(), TokenInput{Code: "test-code"})

	// Verify
	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "no id_token in token response")
}

func TestValidateWithDBDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	// Disable DB
	commonconfig.SetValue("db.enable", "false")
	defer commonconfig.SetValue("db.enable", "")

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create mock verifier
	mockVerifier := reflect.New(reflect.TypeOf(oidc.IDTokenVerifier{})).Interface().(*oidc.IDTokenVerifier)
	mockIDToken := createMockIDToken()

	token := &ssoToken{
		Client:   fakeClient,
		verifier: mockVerifier,
	}

	// Mock (*oidc.IDTokenVerifier).Verify
	patches := gomonkey.ApplyMethod(reflect.TypeOf(mockVerifier), "Verify",
		func(_ *oidc.IDTokenVerifier, _ context.Context, _ string) (*oidc.IDToken, error) {
			return mockIDToken, nil
		})
	defer patches.Reset()

	// Execute
	userInfo, err := token.Validate(context.Background(), "test-raw-token")

	// Verify
	assert.NoError(t, err)
	assert.NotNil(t, userInfo)
	assert.Equal(t, "test@example.com", userInfo.Email)
}

func TestValidateWithDBEnabled_TokenNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	// Enable DB
	commonconfig.SetValue("db.enable", "true")
	defer commonconfig.SetValue("db.enable", "")

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create gomock controller and mock dbClient
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDBClient := mockdb.NewMockInterface(ctrl)
	mockDBClient.EXPECT().
		SelectUserTokens(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.UserToken{}, nil)

	token := &ssoToken{
		Client:   fakeClient,
		dbClient: mockDBClient,
	}

	// Execute
	userInfo, err := token.Validate(context.Background(), "invalid-session-id")

	// Verify
	assert.Error(t, err)
	assert.Nil(t, userInfo)
	assert.Contains(t, err.Error(), "token not present")
}

func TestValidateWithDBEnabled_TokenFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	// Enable DB
	commonconfig.SetValue("db.enable", "true")
	defer commonconfig.SetValue("db.enable", "")

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create mock verifier
	mockVerifier := reflect.New(reflect.TypeOf(oidc.IDTokenVerifier{})).Interface().(*oidc.IDTokenVerifier)
	mockIDToken := createMockIDToken()

	// Create gomock controller and mock dbClient
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDBClient := mockdb.NewMockInterface(ctrl)
	mockDBClient.EXPECT().
		SelectUserTokens(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.UserToken{
			{Token: "real-id-token", ExpireTime: time.Now().Add(time.Hour).Unix()},
		}, nil)

	token := &ssoToken{
		Client:   fakeClient,
		dbClient: mockDBClient,
		verifier: mockVerifier,
	}

	// Mock (*oidc.IDTokenVerifier).Verify
	patches := gomonkey.ApplyMethod(reflect.TypeOf(mockVerifier), "Verify",
		func(_ *oidc.IDTokenVerifier, _ context.Context, _ string) (*oidc.IDToken, error) {
			return mockIDToken, nil
		})
	defer patches.Reset()

	// Execute
	userInfo, err := token.Validate(context.Background(), "valid-session-id")

	// Verify
	assert.NoError(t, err)
	assert.NotNil(t, userInfo)
	assert.Equal(t, "test@example.com", userInfo.Email)
}

func TestInitializeSSOToken_MissingConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Ensure DB is disabled
	commonconfig.SetValue("db.enable", "false")
	defer commonconfig.SetValue("db.enable", "")

	// Test: all SSO config empty
	commonconfig.SetValue("sso.endpoint", "")
	commonconfig.SetValue("sso.client_id", "")
	commonconfig.SetValue("sso.client_secret", "")
	commonconfig.SetValue("sso.redirect_uri", "")
	defer func() {
		commonconfig.SetValue("sso.endpoint", "")
		commonconfig.SetValue("sso.client_id", "")
		commonconfig.SetValue("sso.client_secret", "")
		commonconfig.SetValue("sso.redirect_uri", "")
	}()

	_, err := initializeSSOToken(fakeClient)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find sso config")
}
