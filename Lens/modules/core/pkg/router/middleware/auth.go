package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

const (
	// Header for internal service authentication
	InternalAuthTokenHeader = "X-Internal-Token"
	// Context keys for user info
	ContextKeyUserID    = "auth_user_id"
	ContextKeyUserName  = "auth_user_name"
	ContextKeyUserEmail = "auth_user_email"
	ContextKeyUserType  = "auth_user_type"
)

// VerifyTokenRequest represents the request body for token verification
type VerifyTokenRequest struct {
	Cookie string `json:"cookie"`
}

// VerifyTokenResponse represents the response from SaFE verify endpoint
type VerifyTokenResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message,omitempty"`
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

// HandleAuth returns a middleware that authenticates requests via SaFE API
func HandleAuth(authConfig *config.AuthConfig) gin.HandlerFunc {
	httpClient := &http.Client{
		Timeout: authConfig.GetTimeout(),
	}

	verifyURL := strings.TrimSuffix(authConfig.SafeAPIURL, "/") + "/api/v1/auth/verify"
	internalToken := authConfig.GetInternalToken()

	return func(c *gin.Context) {
		// Check if path is excluded from authentication
		if authConfig.IsPathExcluded(c.Request.URL.Path) {
			c.Next()
			return
		}

		// Get cookie from request
		cookieHeader := c.Request.Header.Get("Cookie")
		if cookieHeader == "" {
			log.Warnf("Auth middleware: no cookie in request for path %s", c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "authentication required",
			})
			return
		}

		// Call SaFE verify endpoint
		userInfo, err := verifyToken(httpClient, verifyURL, internalToken, cookieHeader)
		if err != nil {
			log.Warnf("Auth middleware: token verification failed for path %s: %v", c.Request.URL.Path, err)
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

// verifyToken calls the SaFE verify endpoint to validate the cookie
func verifyToken(client *http.Client, verifyURL, internalToken, cookie string) (*VerifyUserInfo, error) {
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

// AuthError represents an authentication error from SaFE API
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
type AuthMiddlewareConfig struct {
	SafeAPIURL    string
	InternalToken string
	Timeout       time.Duration
	ExcludePaths  []string
}
