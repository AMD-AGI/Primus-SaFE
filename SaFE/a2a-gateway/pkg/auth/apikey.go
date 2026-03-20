/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

const (
	apiKeyPrefix     = "ak-"
	contextKeyUserID = "userId"
)

// ApiKeyMiddleware validates SaFE API keys from the Authorization header.
func ApiKeyMiddleware(db dbclient.Interface) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if !strings.HasPrefix(token, apiKeyPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key format"})
			return
		}

		hashedKey := authority.HashApiKey(token, authority.GetApiKeySecret())
		apiKey, err := db.GetApiKeyByKey(c.Request.Context(), hashedKey)
		if err != nil || apiKey == nil {
			klog.V(4).InfoS("api key validation failed", "error", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}

		if apiKey.Deleted {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "api key has been revoked"})
			return
		}

		c.Set(contextKeyUserID, apiKey.UserId)
		c.Next()
	}
}

// GetUserID extracts the user ID set by the auth middleware.
func GetUserID(c *gin.Context) string {
	v, _ := c.Get(contextKeyUserID)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
