// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth/audit"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth/ldap"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth/session"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// LoginRequest represents the login request body
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	Username  string `json:"username,omitempty"`
	Email     string `json:"email,omitempty"`
	IsAdmin   bool   `json:"is_admin,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

// SessionCookieName is the name of the session cookie
const SessionCookieName = "lens_session"

// Login handles user login
// POST /api/auth/login
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, LoginResponse{
			Success: false,
			Message: "Invalid request: username and password are required",
		})
		return
	}

	ctx := c.Request.Context()
	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	// Get current auth mode
	authMode, err := auth.GetCurrentAuthMode(ctx)
	if err != nil {
		log.Errorf("Failed to get auth mode: %v", err)
		c.JSON(http.StatusInternalServerError, LoginResponse{
			Success: false,
			Message: "Internal server error",
		})
		return
	}

	var userID, email string
	var isAdmin bool
	var authType string

	switch authMode {
	case auth.AuthModeLocal:
		// Local authentication
		userID, isAdmin, err = authenticateLocal(ctx, req.Username, req.Password)
		authType = audit.AuthTypeLocal
	case auth.AuthModeLDAP:
		// LDAP authentication
		userID, email, isAdmin, err = authenticateLDAP(ctx, req.Username, req.Password)
		authType = audit.AuthTypeLDAP
	case auth.AuthModeSaFE:
		// SaFE mode - should redirect to SaFE login
		c.JSON(http.StatusBadRequest, LoginResponse{
			Success: false,
			Message: "SaFE authentication mode is enabled. Please login through SaFE.",
		})
		return
	default:
		c.JSON(http.StatusInternalServerError, LoginResponse{
			Success: false,
			Message: "Unknown authentication mode",
		})
		return
	}

	if err != nil {
		// Record failed login attempt
		audit.GetService().RecordLoginFailed(ctx, req.Username, authType, ipAddress, userAgent, err.Error())

		c.JSON(http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "Invalid username or password",
		})
		return
	}

	// Create session
	sessionMgr := session.GetManager()
	sessionInfo, err := sessionMgr.Create(ctx, &session.CreateOptions{
		UserID:     userID,
		Username:   req.Username,
		Email:      email,
		UserAgent:  userAgent,
		IPAddress:  ipAddress,
		SyncSource: "local",
	})
	if err != nil {
		log.Errorf("Failed to create session: %v", err)
		c.JSON(http.StatusInternalServerError, LoginResponse{
			Success: false,
			Message: "Failed to create session",
		})
		return
	}

	// Record successful login
	audit.GetService().RecordLogin(ctx, req.Username, userID, authType, ipAddress, userAgent)

	// Set session cookie
	setSessionCookie(c, sessionInfo.Token, sessionInfo.ExpiresAt)

	c.JSON(http.StatusOK, LoginResponse{
		Success:   true,
		Message:   "Login successful",
		UserID:    userID,
		Username:  req.Username,
		Email:     email,
		IsAdmin:   isAdmin,
		ExpiresAt: sessionInfo.ExpiresAt.Unix(),
	})
}

// Logout handles user logout
// POST /api/auth/logout
func Logout(c *gin.Context) {
	ctx := c.Request.Context()
	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	// Get session token from cookie
	token, err := c.Cookie(SessionCookieName)
	if err != nil || token == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Already logged out",
		})
		return
	}

	// Validate and get session info for audit
	sessionMgr := session.GetManager()
	sessionInfo, _ := sessionMgr.Validate(ctx, token)

	// Revoke session
	if err := sessionMgr.Revoke(ctx, token, "User logout"); err != nil {
		log.Warnf("Failed to revoke session: %v", err)
		// Continue anyway to clear cookie
	}

	// Record logout
	if sessionInfo != nil {
		audit.GetService().RecordLogout(ctx, sessionInfo.Username, sessionInfo.UserID, ipAddress, userAgent)
	}

	// Clear session cookie
	clearSessionCookie(c)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logout successful",
	})
}

// GetCurrentUser returns the current logged-in user info
// GET /api/auth/me
func GetCurrentUser(c *gin.Context) {
	// Get session info from context (set by middleware)
	sessionInfo, exists := c.Get("session")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Not authenticated",
		})
		return
	}

	info := sessionInfo.(*session.SessionInfo)
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"user_id":  info.UserID,
		"username": info.Username,
		"email":    info.Email,
		"is_admin": info.IsAdmin,
	})
}

// RefreshSession refreshes the current session
// POST /api/auth/refresh
func RefreshSession(c *gin.Context) {
	ctx := c.Request.Context()

	// Get session token from cookie
	token, err := c.Cookie(SessionCookieName)
	if err != nil || token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "No session found",
		})
		return
	}

	// Refresh session
	sessionMgr := session.GetManager()
	sessionInfo, err := sessionMgr.Refresh(ctx, token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Session expired or invalid",
		})
		return
	}

	// Update cookie with new expiration
	setSessionCookie(c, token, sessionInfo.ExpiresAt)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Session refreshed",
		"expires_at": sessionInfo.ExpiresAt.Unix(),
	})
}

// authenticateLocal authenticates user against local database
func authenticateLocal(ctx context.Context, username, password string) (string, bool, error) {
	userFacade := cpdb.GetFacade().GetUser()
	user, err := userFacade.GetByUsername(ctx, username)
	if err != nil {
		return "", false, auth.ErrUserNotFound
	}

	// Verify password
	if !auth.VerifyPassword(password, user.PasswordHash) {
		return "", false, auth.ErrInvalidCredentials
	}

	// Check if user is active
	if user.Status != string(auth.UserStatusActive) {
		return "", false, auth.ErrUserDisabled
	}

	// Update last login
	userFacade.UpdateLastLogin(ctx, user.ID)

	return user.ID, user.IsAdmin, nil
}

// authenticateLDAP authenticates user against LDAP
func authenticateLDAP(ctx context.Context, username, password string) (string, string, bool, error) {
	// Get LDAP manager
	ldapMgr := ldap.GetManager()
	if ldapMgr == nil {
		return "", "", false, auth.ErrLDAPNotConfigured
	}

	// Authenticate against LDAP
	creds := &ldap.Credentials{
		Username: username,
		Password: password,
	}
	result, _, err := ldapMgr.Authenticate(ctx, creds)
	if err != nil {
		return "", "", false, err
	}

	if !result.Success {
		return "", "", false, auth.ErrInvalidCredentials
	}

	// Extract user info from result
	var email, displayName string
	var isAdmin bool
	if result.User != nil {
		email = result.User.Email
		displayName = result.User.DisplayName
		isAdmin = result.User.IsAdmin
	}

	// Ensure user exists in local database
	userFacade := cpdb.GetFacade().GetUser()
	user, err := userFacade.GetByUsername(ctx, username)
	if err != nil {
		// Create user if not exists
		user, err = userFacade.CreateFromLDAP(ctx, username, email, displayName, isAdmin)
		if err != nil {
			return "", "", false, err
		}
	}

	// Update last login time
	userFacade.UpdateLastLogin(ctx, user.ID)

	return user.ID, user.Email, user.IsAdmin, nil
}

// setSessionCookie sets the session cookie
func setSessionCookie(c *gin.Context, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}

	c.SetCookie(
		SessionCookieName,
		token,
		maxAge,
		"/",
		"",    // domain - empty means current domain
		true,  // secure - require HTTPS
		true,  // httpOnly - prevent JS access
	)
}

// clearSessionCookie clears the session cookie
func clearSessionCookie(c *gin.Context) {
	c.SetCookie(
		SessionCookieName,
		"",
		-1, // negative maxAge to delete
		"/",
		"",
		true,
		true,
	)
}
