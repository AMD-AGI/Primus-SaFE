/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
)

const (
	TokenExpire  = "The user's token has expired, please login again"
	InvalidToken = "The user's token is invalid, please login first"
)

func ParseCookie(c *gin.Context) error {
	if !commonconfig.IsEnableUserAuthority() {
		return nil
	}
	err := parseCookie(c)
	if err != nil {
		return commonerrors.NewUnauthorized(err.Error())
	}
	return nil
}

func parseCookie(c *gin.Context) error {
	token, err := c.Cookie(CookieToken)
	if err != nil || token == "" {
		return fmt.Errorf("http: cookie %s not present", CookieToken)
	}
	tokenExpire, err := c.Cookie(CookieTokenExpire)
	if err != nil || tokenExpire == "" {
		return fmt.Errorf("http: cookie %s not present", CookieTokenExpire)
	}
	expire, err := strconv.ParseInt(tokenExpire, 10, 0)
	if err != nil {
		return err
	}
	if commonconfig.GetUserTokenExpire() >= 0 && time.Now().Unix() > expire {
		return fmt.Errorf("%s", TokenExpire)
	}
	userId, err := validateToken(token, expire)
	if err != nil {
		klog.ErrorS(err, "failed to validate user token", "token", token, "expire", expire)
		return fmt.Errorf("%s", InvalidToken)
	}
	tokenType, _ := c.Cookie(CookieTokenType)
	c.Set(common.UserId, userId)
	c.Set(common.UserType, tokenType)
	return nil
}

func validateToken(token string, expire int64) (string, error) {
	inst := crypto.NewCrypto()
	if inst == nil {
		return "", commonerrors.NewInternalError("failed to new crypto")
	}

	tokenPlain, err := inst.Decrypt(token)
	if err != nil {
		return "", fmt.Errorf("fail to decrypt token")
	}

	lastIndex := strings.LastIndex(tokenPlain, "-")
	if lastIndex == -1 {
		return "", fmt.Errorf("invalid token")
	}
	userId := token[:lastIndex]
	if userId == "" {
		return "", fmt.Errorf("invalid token")
	}
	expireDecode, err := strconv.ParseInt(token[lastIndex+1:], 10, 0)
	if err != nil || expireDecode != expire {
		return "", fmt.Errorf("invalid token")
	}
	return userId, nil
}

func BuildToken(userId string, expire int64) (string, error) {
	tokenStr := userId + "-" + strconv.FormatInt(expire, 10)
	if !commonconfig.IsCryptoEnable() {
		return tokenStr, nil
	}
	inst := crypto.NewCrypto()
	if inst == nil {
		return "", commonerrors.NewInternalError("failed to new crypto")
	}
	return inst.Encrypt([]byte(tokenStr))
}
