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
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
	apimetrics "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
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
			apimetrics.AuthFailuresTotal.WithLabelValues("api_key").Inc()
			return commonerrors.NewUnauthorized(err.Error())
		}
		return nil
	}

	// An exchanged token (a JWT issued by this server for a service client acting
	// for a user) is recognised by its type and issuer, not by its shape.
	if ie := identityexchange.Default(); ie != nil {
		if bearer := getBearerToken(c); bearer != "" && ie.IsExchangeToken(bearer) {
			if err := parseExchangeToken(c, ie, bearer); err != nil {
				apimetrics.AuthFailuresTotal.WithLabelValues("exchange").Inc()
				return err
			}
			return nil
		}
	}

	// Fall back to regular token authentication
	err := parseTokenFromRequest(c)
	if err != nil {
		reason := "invalid"
		if strings.Contains(err.Error(), "not present") {
			reason = "missing"
		}
		apimetrics.AuthFailuresTotal.WithLabelValues(reason).Inc()
		return commonerrors.NewUnauthorized(err.Error())
	}
	return nil
}

// parseExchangeToken authenticates a request made by a service client with an
// exchanged token. The user is the token subject and the client is recorded as
// the actor. The capabilities in identityexchange.Forbidden are refused here, for
// every caller of ParseToken, whatever the token's scope.
func parseExchangeToken(c *gin.Context, ie *identityexchange.Service, raw string) error {
	principal, err := ie.ValidateAccessToken(c.Request.Context(), raw, "")
	if err != nil {
		// The reason is logged, the token is not.
		klog.ErrorS(err, "failed to validate exchanged token", "path", c.Request.URL.Path)
		return commonerrors.NewUnauthorized(ErrInvalidToken)
	}
	if capability, forbidden := identityexchange.Forbidden(c.Request.Method, c.Request.URL.Path); forbidden {
		klog.Warningf("exchanged token refused: capability %s, client %s, user %s", capability, principal.ClientId, principal.UserId)
		return commonerrors.NewForbidden("this operation is not available with an exchanged token")
	}
	if !identityexchange.ScopeAllows(principal.Scopes, c.Request.Method) {
		return commonerrors.NewForbidden("the token scope does not allow this operation")
	}
	c.Set(common.UserId, principal.UserId)
	c.Set(common.UserName, principal.UserName)
	c.Set(common.UserType, identityexchange.UserType)
	c.Set(common.ActorClientId, principal.ClientId)
	c.Set(common.TokenJti, principal.Jti)
	klog.Infof("exchanged token accepted for user %s (client %s, jti %s)", principal.UserId, principal.ClientId, principal.Jti)
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
	c.Set(common.ApiKeyId, userInfo.ApiKeyId)
	klog.Infof("API key authentication successful for user: %s (name: %s), apiKeyId: %d", userInfo.Id, userInfo.Name, userInfo.ApiKeyId)
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
		if !commonconfig.IsSSOEnable() {
			return commonerrors.NewNotFoundWithMessage("SSO authentication not initialized")
		}
		tokenInstance = SSOInstance()
	} else {
		tokenInstance = DefaultTokenInstance()
	}
	if tokenInstance == nil {
		return commonerrors.NewInternalError("failed to get token instance")
	}
	userInfo, err := tokenInstance.Validate(c.Request.Context(), rawToken)
	if err != nil {
		klog.ErrorS(err, "failed to validate user token", "userType", userType)
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
