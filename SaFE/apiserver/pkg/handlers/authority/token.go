/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	TokenExpire  = "The user's token has expired, please login again"
	InvalidToken = "The user's token is invalid, please login first"

	TokenDelim = ":"
)

type TokenItem struct {
	UserId   string
	UserType string
	Expire   int64
}

// ParseCookie parses the input data.
func ParseCookie(c *gin.Context) error {
	err := parseCookie(c)
	if err != nil {
		userId := c.GetHeader(common.UserId)
		// only for internal user
		if userId != "" && !commonconfig.IsUserTokenRequired() {
			c.Set(common.UserId, userId)
			return nil
		}
		return commonerrors.NewUnauthorized(err.Error())
	}
	return nil
}

// parseCookie extracts and validates the user token from the request cookie.
// It decrypts the token, checks expiration, and sets the user ID in the context.
// Returns an error if the token is missing, invalid, or expired.
func parseCookie(c *gin.Context) error {
	tokenStr, err := c.Cookie(CookieToken)
	if err != nil || tokenStr == "" {
		return fmt.Errorf("http: cookie %s not present", CookieToken)
	}
	token, err := validateToken(tokenStr)
	if err != nil {
		klog.ErrorS(err, "failed to validate user token", "token", tokenStr)
		return fmt.Errorf("%s", InvalidToken)
	}
	if commonconfig.GetUserTokenExpire() > 0 && time.Now().Unix() > token.Expire {
		return fmt.Errorf("%s", TokenExpire)
	}
	c.Set(common.UserId, token.UserId)
	return nil
}

// validateToken validates Token and returns an error if validation fails.
func validateToken(token string) (*TokenItem, error) {
	inst := crypto.NewCrypto()
	if inst == nil {
		return nil, commonerrors.NewInternalError("failed to new crypto")
	}
	token = stringutil.Base64Decode(token)
	tokenPlain, err := inst.Decrypt(token)
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
	return &TokenItem{
		UserId:   parts[0],
		Expire:   expire,
		UserType: parts[2],
	}, nil
}

// GenerateToken creates a new authentication token for a user.
// It constructs a token string with user ID, expiration time, and user type,
// then encrypts it using the crypto module if encryption is enabled.
// Returns the generated token string or an error if generation fails.
func GenerateToken(item TokenItem) (string, error) {
	if item.UserId == "" {
		return "", fmt.Errorf("invalid token item parameters")
	}
	tokenStr := item.UserId + TokenDelim + strconv.FormatInt(item.Expire, 10) + TokenDelim + item.UserType
	if !commonconfig.IsCryptoEnable() {
		return tokenStr, nil
	}
	inst := crypto.NewCrypto()
	if inst == nil {
		return "", commonerrors.NewInternalError("failed to new crypto")
	}
	return inst.Encrypt([]byte(tokenStr))
}
