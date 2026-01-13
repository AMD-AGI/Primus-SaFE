// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	cpauth "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth/session"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

const (
	// Header for internal service authentication (deprecated with adapter)
	InternalAuthTokenHeader = "X-Internal-Token"
	// Cookie names used by SaFE
	CookieToken    = "Token"
	CookieUserType = "UserType"
	// Context keys for user info
	ContextKeyUserID    = "auth_user_id"
	ContextKeyUserName  = "auth_user_name"
	ContextKeyUserEmail = "auth_user_email"
	ContextKeyUserType  = "auth_user_type"
	ContextKeyIsAdmin   = "auth_user_is_admin"
)

// AdapterValidateRequest represents the request body for primus-safe-adapter validation
type AdapterValidateRequest struct {
	SessionID string `json:"session_id"`
}

// AdapterValidateResponse represents the response from primus-safe-adapter
type AdapterValidateResponse struct {
	Valid   bool   `json:"valid"`
	UserID  string `json:"user_id,omitempty"`
	Name    string `json:"name,omitempty"`
	Email   string `json:"email,omitempty"`
	IsAdmin bool   `json:"is_admin,omitempty"`
	Error   string `json:"error,omitempty"`
}

// VerifyTokenRequest represents the request body for legacy SaFE API token verification
// Deprecated: use AdapterValidateRequest instead
type VerifyTokenRequest struct {
	Cookie string `json:"cookie"`
}

// VerifyTokenResponse represents the response from legacy SaFE verify endpoint
// Deprecated: use AdapterValidateResponse instead
type VerifyTokenResponse struct {
	Code    int            `json:"code"`
	Message string         `json:"message,omitempty"`
	Data    *VerifyUserInfo `json:"data,omitempty"`
}

// VerifyUserInfo represents the user info returned from verification
type VerifyUserInfo struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Exp   int64  `json:"exp"`
	Type  string `json:"type"`
}

// HandleAuth returns a middleware that authenticates requests via primus-safe-adapter
// New architecture: Lens API -> primus-safe-adapter -> SaFE DB (direct query)
func HandleAuth(authConfig *config.AuthConfig) gin.HandlerFunc {
	httpClient := &http.Client{
		Timeout: authConfig.GetTimeout(),
	}

	adapterURL := authConfig.GetSafeAdapterURL()
	validateURL := strings.TrimSuffix(adapterURL, "/") + "/validate"

	// Check if we should use legacy SaFE API (backward compatibility)
	useLegacyAPI := authConfig.SafeAdapterURL == "" && authConfig.SafeAPIURL != ""
	var legacyVerifyURL string
	var internalToken string
	if useLegacyAPI {
		legacyVerifyURL = strings.TrimSuffix(authConfig.SafeAPIURL, "/") + "/api/v1/auth/verify"
		internalToken = authConfig.GetInternalToken()
		log.Warn("Auth middleware using legacy SaFE API, consider migrating to primus-safe-adapter")
	} else {
		log.Infof("Auth middleware using primus-safe-adapter at %s", validateURL)
	}

	return func(c *gin.Context) {
		// Check if path is excluded from authentication
		if authConfig.IsPathExcluded(c.Request.URL.Path) {
			c.Next()
			return
		}

		// Get cookie from request
		cookieHeader := c.Request.Header.Get("Cookie")
		if cookieHeader == "" {
			log.Debugf("Auth middleware: no cookie in request for path %s", c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "authentication required",
			})
			return
		}

		var userInfo *VerifyUserInfo
		var err error

		if useLegacyAPI {
			// Legacy path: call SaFE API directly
			userInfo, err = verifyTokenLegacy(httpClient, legacyVerifyURL, internalToken, cookieHeader)
		} else {
			// New path: call primus-safe-adapter
			userInfo, err = verifyTokenViaAdapter(httpClient, validateURL, cookieHeader)
		}

		if err != nil {
			log.Debugf("Auth middleware: token verification failed for path %s: %v", c.Request.URL.Path, err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "authentication failed",
			})
			return
		}

		// Store user info in context for downstream handlers
		c.Set(ContextKeyUserID, userInfo.Id)
		c.Set(ContextKeyUserName, userInfo.Name)
		c.Set(ContextKeyUserEmail, userInfo.Email)
		c.Set(ContextKeyUserType, userInfo.Type)

		c.Next()
	}
}

// verifyTokenViaAdapter validates the session via primus-safe-adapter
// This is the new recommended approach: adapter queries SaFE DB directly
func verifyTokenViaAdapter(client *http.Client, validateURL, cookieHeader string) (*VerifyUserInfo, error) {
	// Extract session ID (Token cookie) from cookie header
	sessionID := extractCookieValue(cookieHeader, CookieToken)
	if sessionID == "" {
		return nil, &AuthError{Code: 401, Message: "Token cookie not found"}
	}

	// Build request for adapter
	reqBody := AdapterValidateRequest{
		SessionID: sessionID,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, validateURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var adapterResp AdapterValidateResponse
	if err := json.Unmarshal(respBody, &adapterResp); err != nil {
		return nil, err
	}

	if !adapterResp.Valid {
		errMsg := adapterResp.Error
		if errMsg == "" {
			errMsg = "session invalid"
		}
		return nil, &AuthError{Code: 401, Message: errMsg}
	}

	// Extract user type from cookie (for backward compatibility)
	userType := extractCookieValue(cookieHeader, CookieUserType)
	if userType == "" {
		userType = "sso" // Default to SSO for SaFE users
	}

	return &VerifyUserInfo{
		Id:    adapterResp.UserID,
		Name:  adapterResp.Name,
		Email: adapterResp.Email,
		Type:  userType,
	}, nil
}

// verifyTokenLegacy calls the legacy SaFE verify endpoint to validate the cookie
// Deprecated: use verifyTokenViaAdapter instead
func verifyTokenLegacy(client *http.Client, verifyURL, internalToken, cookie string) (*VerifyUserInfo, error) {
	reqBody := VerifyTokenRequest{
		Cookie: cookie,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, verifyURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if internalToken != "" {
		req.Header.Set(InternalAuthTokenHeader, internalToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var verifyResp VerifyTokenResponse
	if err := json.Unmarshal(respBody, &verifyResp); err != nil {
		return nil, err
	}

	if verifyResp.Code != 0 || verifyResp.Data == nil {
		return nil, &AuthError{
			Code:    verifyResp.Code,
			Message: verifyResp.Message,
		}
	}

	return verifyResp.Data, nil
}

// extractCookieValue extracts a specific cookie value from the cookie header string
func extractCookieValue(cookieHeader, name string) string {
	cookies := strings.Split(cookieHeader, ";")
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

		if key == name {
			return value
		}
	}
	return ""
}

// AuthError represents an authentication error
type AuthError struct {
	Code    int
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

// Helper functions to get user info from context

// GetUserID returns the authenticated user's ID from context
func GetUserID(c *gin.Context) string {
	if val, exists := c.Get(ContextKeyUserID); exists {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return ""
}

// GetUserName returns the authenticated user's name from context
func GetUserName(c *gin.Context) string {
	if val, exists := c.Get(ContextKeyUserName); exists {
		if name, ok := val.(string); ok {
			return name
		}
	}
	return ""
}

// GetUserEmail returns the authenticated user's email from context
func GetUserEmail(c *gin.Context) string {
	if val, exists := c.Get(ContextKeyUserEmail); exists {
		if email, ok := val.(string); ok {
			return email
		}
	}
	return ""
}

// GetUserType returns the authenticated user's type from context
func GetUserType(c *gin.Context) string {
	if val, exists := c.Get(ContextKeyUserType); exists {
		if userType, ok := val.(string); ok {
			return userType
		}
	}
	return ""
}

// AuthMiddlewareConfig is used to create auth middleware with custom settings
// Deprecated: use config.AuthConfig instead
type AuthMiddlewareConfig struct {
	SafeAPIURL    string
	InternalToken string
	Timeout       time.Duration
	ExcludePaths  []string
}

// HandleDynamicAuth returns a middleware that authenticates requests based on database configuration
// This middleware dynamically reads auth mode from database and validates accordingly:
// - Safe mode: validates Token cookie via primus-safe-adapter
// - Local/LDAP mode: validates lens_session cookie via session manager
// - None mode: skip authentication
func HandleDynamicAuth(excludePaths []string) gin.HandlerFunc {
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	return func(c *gin.Context) {
		// Check if path is excluded from authentication
		if isPathExcluded(c.Request.URL.Path, excludePaths) {
			c.Next()
			return
		}

		ctx := c.Request.Context()
		configService := cpauth.GetAuthConfigService()

		// Get auth mode from database (cached)
		mode, err := configService.GetAuthMode(ctx)
		if err != nil {
			log.Warnf("Failed to get auth mode: %v, skipping auth", err)
			c.Next()
			return
		}

		// Skip auth if mode is none
		if mode == cpauth.AuthModeNone {
			c.Next()
			return
		}

		// Handle based on mode
		switch mode {
		case cpauth.AuthModeSaFE:
			if !validateSafeSession(c, ctx, configService, httpClient) {
				return // Already aborted with 401
			}

		case cpauth.AuthModeLocal, cpauth.AuthModeLDAP:
			if !validateLocalSession(c, ctx) {
				return // Already aborted with 401
			}

		case cpauth.AuthModeSSO:
			// TODO: Standard OIDC validation
			log.Warn("SSO mode authentication not yet implemented")
			c.Next()
			return

		default:
			log.Warnf("Unknown auth mode: %s, skipping auth", mode)
			c.Next()
			return
		}

		c.Next()
	}
}

// validateSafeSession validates Safe mode session via primus-safe-adapter
func validateSafeSession(c *gin.Context, ctx context.Context, configService *cpauth.AuthConfigService, httpClient *http.Client) bool {
	// Get Token cookie (set by SaFE)
	token := extractCookieValue(c.Request.Header.Get("Cookie"), CookieToken)
	if token == "" {
		log.Debug("No Token cookie found for Safe mode auth")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "authentication required",
		})
		return false
	}

	// Get adapter URL from config
	safeConfig, err := configService.GetSafeConfig(ctx)
	if err != nil || safeConfig.AdapterURL == "" {
		log.Errorf("Safe adapter URL not configured: %v", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "authentication service not configured",
		})
		return false
	}

	// Validate via adapter
	validateURL := strings.TrimSuffix(safeConfig.AdapterURL, "/") + "/validate"
	userInfo, err := verifyTokenViaAdapter(httpClient, validateURL, c.Request.Header.Get("Cookie"))
	if err != nil {
		log.Debugf("Safe session validation failed: %v", err)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "session invalid or expired",
		})
		return false
	}

	// Set user info in context
	c.Set(ContextKeyUserID, userInfo.Id)
	c.Set(ContextKeyUserName, userInfo.Name)
	c.Set(ContextKeyUserEmail, userInfo.Email)
	c.Set(ContextKeyUserType, userInfo.Type)

	return true
}

// validateLocalSession validates local/LDAP session via session manager
func validateLocalSession(c *gin.Context, ctx context.Context) bool {
	// Get lens_session cookie
	token, err := c.Cookie(session.SessionCookieName)
	if err != nil || token == "" {
		log.Debug("No lens_session cookie found")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "authentication required",
		})
		return false
	}

	// Validate session
	sessionMgr := session.GetManager()
	sessionInfo, err := sessionMgr.Validate(ctx, token)
	if err != nil {
		log.Debugf("Local session validation failed: %v", err)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "session invalid or expired",
		})
		return false
	}

	// Set user info in context
	c.Set(ContextKeyUserID, sessionInfo.UserID)
	c.Set(ContextKeyUserName, sessionInfo.Username)
	c.Set(ContextKeyUserEmail, sessionInfo.Email)
	c.Set(ContextKeyUserType, sessionInfo.AuthType)
	c.Set(ContextKeyIsAdmin, sessionInfo.IsAdmin)
	c.Set("session", sessionInfo)

	return true
}

// isPathExcluded checks if path should skip authentication
func isPathExcluded(path string, excludePaths []string) bool {
	for _, excludePath := range excludePaths {
		// Support wildcard suffix
		if strings.HasSuffix(excludePath, "*") {
			prefix := strings.TrimSuffix(excludePath, "*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		} else if path == excludePath {
			return true
		}
	}
	return false
}
