/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"strings"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	ErrTokenExpire  = "The user's token has expired, please login again"
	ErrInvalidToken = "The user's token is invalid, please login first"
	TokenDelim      = ":"
)

type TokenItem struct {
	UserId   string
	UserType string
	Expire   int64
}

// ParseToken parses the token from request (cookie or header)
// It first tries API Key authentication, then falls back to regular token authentication
func ParseToken(c *gin.Context) error {
	// First, try to authenticate with API key from Authorization: Bearer header
	authHeader := c.GetHeader("Authorization")
	apiKey := ExtractApiKeyFromRequest(authHeader)

	if apiKey != "" {
		// Authenticate using API key
		err := parseApiKeyFromRequest(c, apiKey)
		if err != nil {
			return commonerrors.NewUnauthorized(err.Error())
		}
		return nil
	}

	// Fall back to regular token authentication
	err := parseTokenFromRequest(c)
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

// parseApiKeyFromRequest validates the API key and sets user info in context
func parseApiKeyFromRequest(c *gin.Context, apiKey string) error {
	apiKeyToken := ApiKeyTokenInstance()
	if apiKeyToken == nil {
		return commonerrors.NewInternalError("API key authentication not initialized")
	}

	// Get client IP for whitelist check
	clientIP := c.ClientIP()

	userInfo, err := apiKeyToken.ValidateApiKey(c.Request.Context(), apiKey, clientIP)
	if err != nil {
		klog.ErrorS(err, "failed to validate API key")
		return err
	}

	c.Set(common.UserId, userInfo.Id)
	c.Set(common.UserName, userInfo.Name)
	c.Set(common.UserType, UserTypeApiKey)
	klog.Infof("API key authentication successful for user: %s (name: %s)", userInfo.Id, userInfo.Name)
	return nil
}

// parseTokenFromRequest extracts and validates the user token from the request cookie or header.
// It decrypts the token, checks expiration, and sets the user ID in the context.
// Returns an error if the token is missing, invalid, or expired.
func parseTokenFromRequest(c *gin.Context) error {
	rawToken, userType, err := extractTokenAndUserType(c)
	if err != nil {
		return err
	}

	var tokenInstance TokenInterface
	if userType == string(v1.SSOUserType) {
		tokenInstance = SSOInstance()
	} else {
		tokenInstance = DefaultTokenInstance()
	}
	if tokenInstance == nil {
		return commonerrors.NewInternalError("failed to get token instance")
	}
	userInfo, err := tokenInstance.Validate(c.Request.Context(), rawToken)
	if err != nil {
		klog.ErrorS(err, "failed to validate user token", "token", rawToken)
		return commonerrors.NewUnauthorized(ErrInvalidToken)
	}
	c.Set(common.UserId, userInfo.Id)
	c.Set(common.UserName, userInfo.Name)
	c.Set(common.UserType, userType)
	klog.Infof("User %s (name: %s) of type %s validated successfully", userInfo.Id, userInfo.Name, userType)
	return nil
}

// extractTokenAndUserType retrieves token and user type from request
// Tries cookie first, falls back to Bearer token and header for user type
func extractTokenAndUserType(c *gin.Context) (string, string, error) {
	rawToken, err := c.Cookie(CookieToken)
	userType, _ := c.Cookie(common.UserType)
	if err != nil {
		rawToken = getBearerToken(c)
		userType = c.GetHeader("UserType")
	}
	if rawToken == "" {
		return "", "", commonerrors.NewUnauthorized("token not present")
	}
	return rawToken, userType, nil
}

// getBearerToken extracts the Bearer token from the Authorization header.
// Returns the token string if valid, otherwise returns an empty string.
func getBearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return parts[1]
}
