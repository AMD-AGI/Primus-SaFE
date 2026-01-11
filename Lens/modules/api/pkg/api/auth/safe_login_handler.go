// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	cpauth "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// AuthConfigResponse represents the auth configuration response
type AuthConfigResponse struct {
	Mode        string          `json:"mode"`
	Initialized bool            `json:"initialized"`
	Safe        *SafeConfigInfo `json:"safe,omitempty"`
}

// SafeConfigInfo represents Safe config info for frontend
type SafeConfigInfo struct {
	LoginURL string `json:"login_url,omitempty"`
}

// GetAuthConfig returns the current authentication configuration
// GET /api/v1/auth/config
// This endpoint does not require authentication
func GetAuthConfig(c *gin.Context) {
	ctx := c.Request.Context()
	configService := cpauth.GetAuthConfigService()

	config, err := configService.GetFullConfig(ctx)
	if err != nil {
		log.Errorf("Failed to get auth config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get authentication configuration",
		})
		return
	}

	response := AuthConfigResponse{
		Mode:        string(config.Mode),
		Initialized: config.Initialized,
	}

	// Include Safe config info (only login_url for frontend)
	if config.Safe != nil && config.Safe.LoginURL != "" {
		response.Safe = &SafeConfigInfo{
			LoginURL: config.Safe.LoginURL,
		}
	}

	c.JSON(http.StatusOK, response)
}

// LoginRedirect handles the login redirect based on auth mode
// GET /api/v1/auth/login
// This endpoint does not require authentication
// For Safe mode: returns 302 redirect to SaFE login page
// For LDAP/Local mode: returns JSON indicating form login is required
func LoginRedirect(c *gin.Context) {
	ctx := c.Request.Context()
	configService := cpauth.GetAuthConfigService()

	// 1. Get auth mode
	mode, err := configService.GetAuthMode(ctx)
	if err != nil {
		log.Errorf("Failed to get auth mode: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get authentication configuration",
		})
		return
	}

	// 2. Get redirect parameter (where to go after login)
	redirect := c.Query("redirect")
	if redirect == "" {
		redirect = "/lens/"
	}

	// Validate redirect to prevent open redirect vulnerability
	redirect = sanitizeRedirect(redirect)

	// 3. Handle based on mode
	switch mode {
	case cpauth.AuthModeSaFE:
		handleSafeLoginRedirect(c, ctx, configService, redirect)

	case cpauth.AuthModeLDAP, cpauth.AuthModeLocal, cpauth.AuthModeNone:
		// These modes require form login
		c.JSON(http.StatusOK, gin.H{
			"mode":          string(mode),
			"form_required": true,
			"redirect":      redirect,
		})

	case cpauth.AuthModeSSO:
		// TODO: Standard OIDC flow
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": "Standard OIDC authentication not yet implemented",
		})

	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Unknown authentication mode: %s", mode),
		})
	}
}

// handleSafeLoginRedirect handles Safe mode login redirect
func handleSafeLoginRedirect(c *gin.Context, ctx context.Context, configService *cpauth.AuthConfigService, redirect string) {
	// Get Safe configuration
	safeConfig, err := configService.GetSafeConfig(ctx)
	if err != nil || safeConfig.LoginURL == "" {
		log.Errorf("Safe login URL not configured: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Safe authentication not properly configured",
		})
		return
	}

	// Build callback URL: /lens/sso-bridge?target=/lens/dashboard
	callbackURL := fmt.Sprintf("%s?target=%s",
		safeConfig.CallbackPath,
		url.QueryEscape(redirect))

	// Build SaFE login URL with callback
	// https://tw325.primus-safe.amd.com/login?redirect=/lens/sso-bridge?target=/lens/dashboard
	loginURL := fmt.Sprintf("%s?redirect=%s",
		safeConfig.LoginURL,
		url.QueryEscape(callbackURL))

	log.Debugf("Safe login redirect: %s", loginURL)

	// Return 302 redirect
	c.Redirect(http.StatusFound, loginURL)
}

// sanitizeRedirect validates and sanitizes the redirect URL
// to prevent open redirect vulnerabilities
func sanitizeRedirect(redirect string) string {
	// Only allow relative paths starting with /
	if redirect == "" {
		return "/lens/"
	}

	// Parse the URL
	parsed, err := url.Parse(redirect)
	if err != nil {
		return "/lens/"
	}

	// Reject absolute URLs (with scheme or host)
	if parsed.Scheme != "" || parsed.Host != "" {
		return "/lens/"
	}

	// Reject path traversal attempts
	if len(redirect) > 0 && redirect[0] != '/' {
		return "/lens/"
	}

	// Reject double slashes (potential protocol-relative URL)
	if len(redirect) > 1 && redirect[0] == '/' && redirect[1] == '/' {
		return "/lens/"
	}

	return redirect
}
