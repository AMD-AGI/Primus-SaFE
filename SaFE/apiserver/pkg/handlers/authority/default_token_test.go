/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"sync"
	"testing"
	"time"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func TestGenerateDefaultToken(t *testing.T) {
	commonconfig.SetValue("crypto.enable", "false")
	defer commonconfig.SetValue("crypto.enable", "")

	tests := []struct {
		name      string
		userId    string
		expire    int64
		username  string
		wantError bool
	}{
		{
			name:      "valid token generation",
			userId:    "user123",
			expire:    1234567890,
			username:  "testuser",
			wantError: false,
		},
		{
			name:      "empty userId should fail",
			userId:    "",
			expire:    1234567890,
			username:  "testuser",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := generateDefaultToken(tt.userId, tt.expire, tt.username)
			if tt.wantError {
				assert.Error(t, err)
				assert.Empty(t, token)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, token)
			}
		})
	}
}

func TestGetUserById(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	testUser := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "user123"},
		Spec: v1.UserSpec{
			Type: v1.DefaultUserType,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testUser).
		Build()

	tests := []struct {
		name      string
		userId    string
		wantError bool
	}{
		{
			name:      "existing user",
			userId:    "user123",
			wantError: false,
		},
		{
			name:      "non-existing user",
			userId:    "nonexistent",
			wantError: true,
		},
		{
			name:      "empty userId",
			userId:    "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := getUserById(context.Background(), fakeClient, tt.userId)
			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.userId, user.Name)
			}
		})
	}
}

func TestLogin(t *testing.T) {
	scheme := runtime.NewScheme()
	commonconfig.SetValue("crypto.enable", "false")
	defer commonconfig.SetValue("crypto.enable", "")
	_ = v1.AddToScheme(scheme)

	username := "testuser"
	password := "testpassword"
	userId := commonuser.GenerateUserIdByName(username)

	testUser := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: userId},
		Spec: v1.UserSpec{
			Type:     v1.DefaultUserType,
			Password: stringutil.Base64Encode(password),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testUser).
		Build()

	token := &defaultToken{Client: fakeClient}

	tests := []struct {
		name      string
		input     TokenInput
		wantError bool
		errorMsg  string
	}{
		{
			name: "successful login",
			input: TokenInput{
				Username: username,
				Password: password,
			},
			wantError: false,
		},
		{
			name: "empty username",
			input: TokenInput{
				Username: "",
				Password: password,
			},
			wantError: true,
			errorMsg:  "the userName is empty",
		},
		{
			name: "wrong password",
			input: TokenInput{
				Username: username,
				Password: "wrongpassword",
			},
			wantError: true,
			errorMsg:  "the password is incorrect",
		},
		{
			name: "non-existing user",
			input: TokenInput{
				Username: "nonexistent",
				Password: password,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, resp, err := token.Login(context.Background(), tt.input)
			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, resp)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.NotNil(t, resp)
				assert.NotEmpty(t, resp.Token)
			}
		})
	}
}

func TestNewDefaultToken(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Reset singleton for test
	defaultTokenInitOnce = sync.Once{}
	defaultTokenInstance = nil

	token := NewDefaultToken(fakeClient)
	assert.NotNil(t, token)

	// Verify singleton pattern
	token2 := NewDefaultToken(fakeClient)
	assert.Equal(t, token, token2)
}

func TestDefaultTokenInstance(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Reset singleton for test
	defaultTokenInitOnce = sync.Once{}
	defaultTokenInstance = nil

	// Before initialization, should return nil
	inst := DefaultTokenInstance()
	assert.Nil(t, inst)

	// After initialization
	NewDefaultToken(fakeClient)
	inst = DefaultTokenInstance()
	assert.NotNil(t, inst)
}

func TestValidate(t *testing.T) {
	// Disable crypto for testing
	commonconfig.SetValue("crypto.enable", "false")
	defer commonconfig.SetValue("crypto.enable", "")

	// Disable token expiration check for most tests
	commonconfig.SetValue("user.token.expire", "-1")
	defer commonconfig.SetValue("user.token.expire", "")

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	token := &defaultToken{Client: fakeClient}

	// Generate a valid token for testing
	userId := "user123"
	username := "testuser"
	expire := time.Now().Add(time.Hour).Unix()
	validToken, err := generateDefaultToken(userId, expire, username)
	assert.NoError(t, err)
	validTokenEncoded := stringutil.Base64Encode(validToken)

	tests := []struct {
		name      string
		rawToken  string
		wantError bool
		errorMsg  string
		checkUser bool
		userId    string
		userName  string
	}{
		{
			name:      "valid token",
			rawToken:  validTokenEncoded,
			wantError: false,
			checkUser: true,
			userId:    userId,
			userName:  username,
		},
		{
			name:      "invalid base64",
			rawToken:  "not-valid-base64!!!",
			wantError: true,
			errorMsg:  "invalid token",
		},
		{
			name:      "invalid token format - wrong parts count",
			rawToken:  stringutil.Base64Encode("part1:part2"),
			wantError: true,
			errorMsg:  "invalid token",
		},
		{
			name:      "invalid token format - empty part",
			rawToken:  stringutil.Base64Encode("part1::part3:part4"),
			wantError: true,
			errorMsg:  "invalid token",
		},
		{
			name:      "invalid expire format",
			rawToken:  stringutil.Base64Encode("user123:not-a-number:default:testuser"),
			wantError: true,
			errorMsg:  "invalid token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userInfo, err := token.Validate(context.Background(), tt.rawToken)
			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, userInfo)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, userInfo)
				if tt.checkUser {
					assert.Equal(t, tt.userId, userInfo.Id)
					assert.Equal(t, tt.userName, userInfo.Name)
				}
			}
		})
	}
}

func TestValidateExpiredToken(t *testing.T) {
	// Disable crypto for testing
	commonconfig.SetValue("crypto.enable", "false")
	defer commonconfig.SetValue("crypto.enable", "")

	// Enable token expiration check
	commonconfig.SetValue("user.token_expire", "3600")
	defer commonconfig.SetValue("user.token.expire", "")

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	token := &defaultToken{Client: fakeClient}

	// Generate an expired token
	expiredTime := time.Now().Add(-time.Hour).Unix()
	expiredToken, err := generateDefaultToken("user123", expiredTime, "testuser")
	assert.NoError(t, err)
	expiredTokenEncoded := stringutil.Base64Encode(expiredToken)

	// Validate should fail with expired token
	userInfo, err := token.Validate(context.Background(), expiredTokenEncoded)
	assert.Error(t, err)
	assert.Nil(t, userInfo)
	assert.Contains(t, err.Error(), ErrTokenExpire)
}
