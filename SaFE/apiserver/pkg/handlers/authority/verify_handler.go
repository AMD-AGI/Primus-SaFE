/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// VerifyTokenRequest represents the request body for token verification
type VerifyTokenRequest struct {
	// Raw cookie header string from user request (e.g., "Token=xxx; UserType=sso")
	Cookie string `json:"cookie,omitempty"`
	// Alternative: Authorization header value (e.g., "Bearer xxx" or "Bearer ak-xxx")
	Authorization string `json:"authorization,omitempty"`
	// User type when using authorization header
	UserType string `json:"userType,omitempty"`
	// API key for direct authentication (e.g., "ak-xxx")
	ApiKey string `json:"apiKey,omitempty"`
	// Original client IP, used for API key whitelist validation
	ClientIP string `json:"clientIP,omitempty"`
	// If true, returns the user's platform API key (GetOrCreate)
	IncludePlatformKey bool `json:"includePlatformKey,omitempty"`
	// If true, returns the user's LiteLLM virtual key (decrypted)
	IncludeVirtualKey bool `json:"includeVirtualKey,omitempty"`
}

// VerifyTokenResponse represents the response body for token verification
type VerifyTokenResponse struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email,omitempty"`
	Exp         int64  `json:"exp"`
	Type        string `json:"type"`
	ApiKeyId    int64  `json:"apiKeyId,omitempty"`
	PlatformKey string `json:"platformKey,omitempty"`
	VirtualKey  string `json:"virtualKey,omitempty"`
}

// VerifyToken validates a user token and returns user information
// This endpoint is only accessible to internal services with valid X-Internal-Token
func VerifyToken(c *gin.Context) {
	// Validate internal service authentication
	internalAuth := InternalAuthInstance()
	if internalAuth == nil {
		klog.Error("internal auth not initialized")
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "internal auth not available",
		})
		return
	}

	internalToken := c.GetHeader(InternalAuthTokenHeader)
	if !internalAuth.Validate(internalToken) {
		klog.Warning("invalid internal token for verify request")
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": "internal authentication required",
		})
		return
	}

	// Parse request body
	var req VerifyTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		klog.ErrorS(err, "failed to parse verify token request")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "invalid request body",
		})
		return
	}

	// Authenticate: Cookie > ApiKey > Authorization (auto-detect ak- prefix)
	var userInfo *UserInfo
	var rawToken, userType string
	var err error

	if req.Cookie != "" {
		rawToken, userType, err = parseCookieString(req.Cookie)
		if err != nil {
			klog.ErrorS(err, "failed to parse cookie string")
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":    http.StatusBadRequest,
				"message": "invalid cookie format",
			})
			return
		}
	} else if req.ApiKey != "" {
		userInfo, err = verifyApiKey(c, req.ApiKey, req.ClientIP)
		if err != nil {
			return
		}
		userType = UserTypeApiKey
	} else if req.Authorization != "" {
		if apiKey := ExtractApiKeyFromRequest(req.Authorization); apiKey != "" {
			userInfo, err = verifyApiKey(c, apiKey, req.ClientIP)
			if err != nil {
				return
			}
			userType = UserTypeApiKey
		} else {
			rawToken = extractBearerToken(req.Authorization)
			userType = req.UserType
			if userType == "" {
				userType = string(v1.DefaultUserType)
			}
		}
	} else {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "cookie, apiKey, or authorization is required",
		})
		return
	}

	// For cookie/bearer-token paths, validate the token to get userInfo
	if userInfo == nil {
		if rawToken == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": ErrInvalidToken,
			})
			return
		}

		var tokenInstance TokenInterface
		if userType == string(v1.SSOUserType) {
			if !commonconfig.IsSSOEnable() {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"code":    http.StatusBadRequest,
					"message": "SSO is not enabled",
				})
				return
			}
			tokenInstance = SSOInstance()
		} else {
			tokenInstance = DefaultTokenInstance()
		}

		if tokenInstance == nil {
			klog.Error("token validator not available")
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":    http.StatusInternalServerError,
				"message": "token validator not available",
			})
			return
		}

		userInfo, err = tokenInstance.Validate(c.Request.Context(), rawToken)
		if err != nil {
			klog.ErrorS(err, "failed to validate user token")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": ErrInvalidToken,
			})
			return
		}
	}

	resp := VerifyTokenResponse{
		Id:       userInfo.Id,
		Name:     userInfo.Name,
		Email:    userInfo.Email,
		Exp:      userInfo.Exp,
		Type:     userType,
		ApiKeyId: userInfo.ApiKeyId,
	}

	// Optionally include platform API key (GetOrCreate)
	if req.IncludePlatformKey {
		apiKeyToken := ApiKeyTokenInstance()
		if apiKeyToken != nil {
			platformKey, err := apiKeyToken.GetOrCreatePlatformKey(c.Request.Context(), userInfo.Id, userInfo.Name)
			if err != nil {
				klog.ErrorS(err, "failed to get/create platform key", "userId", userInfo.Id)
			} else {
				resp.PlatformKey = platformKey
			}
		} else {
			klog.Warning("API key auth not initialized, cannot provide platform key")
		}
	}

	// Optionally include LiteLLM virtual key (decrypted from DB)
	if req.IncludeVirtualKey && resp.Email != "" {
		apiKeyToken := ApiKeyTokenInstance()
		if apiKeyToken != nil {
			virtualKey, err := apiKeyToken.GetVirtualKeyByEmail(c.Request.Context(), resp.Email)
			if err != nil {
				klog.ErrorS(err, "failed to get virtual key", "email", resp.Email)
			} else if virtualKey != "" {
				resp.VirtualKey = virtualKey
			}
		} else {
			klog.Warning("API key auth not initialized, cannot provide virtual key")
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": resp,
	})
}

// verifyApiKey validates an API key and returns user info.
// Sends error response to gin context on failure.
func verifyApiKey(c *gin.Context, apiKey, clientIP string) (*UserInfo, error) {
	apiKeyToken := ApiKeyTokenInstance()
	if apiKeyToken == nil {
		klog.Error("API key auth not initialized")
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "API key authentication not available",
		})
		return nil, commonerrors.NewInternalError("API key auth not initialized")
	}

	userInfo, err := apiKeyToken.ValidateApiKey(c.Request.Context(), apiKey, clientIP)
	if err != nil {
		klog.ErrorS(err, "failed to validate API key")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "invalid API key",
		})
		return nil, err
	}
	return userInfo, nil
}

// parseCookieString parses raw cookie string and extracts token and userType
func parseCookieString(cookieStr string) (token, userType string, err error) {
	cookies := strings.Split(cookieStr, ";")
	for _, cookie := range cookies {
		cookie = strings.TrimSpace(cookie)
		if cookie == "" {
			continue
		}

		parts := strings.SplitN(cookie, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case CookieToken:
			token = value
		case CookieUserType:
			userType = value
		}
	}

	if token == "" {
		return "", "", commonerrors.NewBadRequest("token not found in cookie")
	}

	return token, userType, nil
}

// extractBearerToken extracts the token from an Authorization header value
func extractBearerToken(authHeader string) string {
	if authHeader == "" {
		return ""
	}
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return parts[1]
}
