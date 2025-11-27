/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// defaultToken implements TokenInterface for local user authentication
// using username/password credentials
type defaultToken struct {
	client.Client
}

var (
	defaultTokenInitOnce sync.Once
	defaultTokenInstance *defaultToken
)

// NewDefaultToken creates and returns a singleton instance of defaultToken
// implementing the TokenInterface for local user authentication
func NewDefaultToken(cli client.Client) *defaultToken {
	defaultTokenInitOnce.Do(func() {
		defaultTokenInstance = initializeDefaultToken(cli)
	})
	return defaultTokenInstance
}

// DefaultTokenInstance returns the singleton instance of defaultToken
func DefaultTokenInstance() *defaultToken {
	return defaultTokenInstance
}

// initializeDefaultToken initializes and returns a new defaultToken instance
func initializeDefaultToken(cli client.Client) *defaultToken {
	return &defaultToken{
		Client: cli,
	}
}

// Login authenticates a user with username and password, and generates a new token
// Implements TokenInterface.Login method
func (t *defaultToken) Login(ctx context.Context, input TokenInput) (*v1.User, *TokenResponse, error) {
	if input.Username == "" {
		return nil, nil, commonerrors.NewBadRequest("the userName is empty")
	}
	userId := commonuser.GenerateUserIdByName(input.Username)
	user, err := getUserById(ctx, t.Client, userId)
	if err != nil {
		return nil, nil, commonerrors.NewUserNotRegistered(input.Username)
	}
	if user.Spec.Password != stringutil.Base64Encode(input.Password) {
		return nil, nil, commonerrors.NewUnauthorized("the password is incorrect")
	}

	result := &TokenResponse{}
	// Set expiration time based on configuration
	if commonconfig.GetUserTokenExpire() < 0 {
		result.Expire = -1
	} else {
		result.Expire = time.Now().Unix() + int64(commonconfig.GetUserTokenExpire())
	}

	// Generate and encode token
	result.Token, err = generateDefaultToken(userId, result.Expire)
	if err != nil {
		klog.ErrorS(err, "failed to generate user token")
		return nil, nil, err
	}
	result.Token = stringutil.Base64Encode(result.Token)
	return user, result, nil
}

// Validate validates a token string and extracts user information
// Implements TokenInterface.Validate method
func (t *defaultToken) Validate(_ context.Context, rawToken string) (*UserInfo, error) {
	inst := crypto.NewCrypto()
	if inst == nil {
		return nil, commonerrors.NewInternalError("failed to new crypto")
	}
	rawToken = stringutil.Base64Decode(rawToken)
	tokenPlain, err := inst.Decrypt(rawToken)
	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}

	parts := strings.Split(tokenPlain, TokenDelim)
	if len(parts) != 3 {
		klog.Errorf("invalid user token, tokenPlain: %s, current len: %d", tokenPlain, len(parts))
		return nil, fmt.Errorf("invalid token")
	}
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("invalid token")
		}
	}
	expire, err := strconv.ParseInt(parts[1], 10, 0)
	if err != nil {
		klog.ErrorS(err, "failed to parse token expire", "user", parts[0], "expire", parts[1])
		return nil, fmt.Errorf("invalid token")
	}
	if commonconfig.GetUserTokenExpire() > 0 && time.Now().Unix() > expire {
		return nil, fmt.Errorf("%s", ErrTokenExpire)
	}
	return &UserInfo{
		Id:  parts[0],
		Exp: expire,
	}, nil
}

// generateDefaultToken generates an authentication token for a user with optional encryption.
// The token contains user ID, expiration time, and user type.
// Returns the token string or an error if generation fails.
func generateDefaultToken(userId string, expire int64) (string, error) {
	if userId == "" {
		return "", fmt.Errorf("invalid token item parameters")
	}
	tokenStr := userId + TokenDelim + strconv.FormatInt(expire, 10) + TokenDelim + string(v1.DefaultUserType)
	if !commonconfig.IsCryptoEnable() {
		return tokenStr, nil
	}
	inst := crypto.NewCrypto()
	if inst == nil {
		return "", commonerrors.NewInternalError("failed to new crypto")
	}
	return inst.Encrypt([]byte(tokenStr))
}

// getUserById retrieves a user resource by ID from the system.
// Returns an error if the user doesn't exist or the ID is empty.
func getUserById(ctx context.Context, cli client.Client, userId string) (*v1.User, error) {
	if userId == "" {
		return nil, commonerrors.NewBadRequest("the userId is empty")
	}
	user := &v1.User{}
	err := cli.Get(ctx, client.ObjectKey{Name: userId}, user)
	if err != nil {
		klog.ErrorS(err, "failed to get user")
		return nil, err
	}
	return user, nil
}
