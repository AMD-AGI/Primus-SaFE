// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	ldappkg "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth/ldap"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListAuthProviders lists all authentication providers
// GET /api/v1/admin/auth/providers
func ListAuthProviders(c *gin.Context) {
	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	providers, err := facade.GetAuthProvider().List(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := &ListAuthProvidersResponse{
		Providers: make([]*AuthProviderResponse, len(providers)),
	}

	for i, p := range providers {
		resp.Providers[i] = toAuthProviderResponse(p)
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// GetAuthProvider gets a single authentication provider by ID
// GET /api/v1/admin/auth/providers/:id
func GetAuthProvider(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	provider, err := facade.GetAuthProvider().GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Check if provider was actually found (GORM callback may return nil error with empty struct)
	if provider == nil || provider.ID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	resp := toAuthProviderDetailResponse(provider)
	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// CreateAuthProvider creates a new authentication provider
// POST /api/v1/admin/auth/providers
func CreateAuthProvider(c *gin.Context) {
	var req CreateAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate provider type
	if !isValidProviderType(req.Type) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider type"})
		return
	}

	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	// Check if provider with same name exists
	// Note: Due to GORM callback converting ErrRecordNotFound to nil,
	// we must also check if the returned struct has a valid ID
	existing, err := facade.GetAuthProvider().GetByName(ctx, req.Name)
	if err == nil && existing != nil && existing.ID != "" {
		c.JSON(http.StatusConflict, gin.H{"error": "provider with this name already exists"})
		return
	}

	// Convert config to ExtType
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config format"})
		return
	}
	var configMap model.ExtType
	if err := json.Unmarshal(configJSON, &configMap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config format"})
		return
	}

	now := time.Now()
	provider := &model.LensAuthProviders{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Type:      string(req.Type),
		Enabled:   req.Enabled,
		Config:    configMap,
		Status:    string(ProviderStatusActive),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := facade.GetAuthProvider().Create(ctx, provider); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := toAuthProviderResponse(provider)
	c.JSON(http.StatusCreated, rest.SuccessResp(c, resp))
}

// UpdateAuthProvider updates an existing authentication provider
// PUT /api/v1/admin/auth/providers/:id
func UpdateAuthProvider(c *gin.Context) {
	id := c.Param("id")
	var req UpdateAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	provider, err := facade.GetAuthProvider().GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Check if provider was actually found (GORM callback may return nil error with empty struct)
	if provider == nil || provider.ID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	// Update fields if provided
	if req.Name != nil {
		// Check if another provider with this name exists
		// Note: Must check existing.ID != "" due to GORM callback issue
		existing, err := facade.GetAuthProvider().GetByName(ctx, *req.Name)
		if err == nil && existing != nil && existing.ID != "" && existing.ID != id {
			c.JSON(http.StatusConflict, gin.H{"error": "provider with this name already exists"})
			return
		}
		provider.Name = *req.Name
	}
	if req.Enabled != nil {
		provider.Enabled = *req.Enabled
	}
	if req.Config != nil {
		configJSON, err := json.Marshal(req.Config)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config format"})
			return
		}
		var configMap model.ExtType
		if err := json.Unmarshal(configJSON, &configMap); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config format"})
			return
		}
		provider.Config = configMap
	}

	if err := facade.GetAuthProvider().Update(ctx, provider); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := toAuthProviderResponse(provider)
	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// DeleteAuthProvider deletes an authentication provider
// DELETE /api/v1/admin/auth/providers/:id
func DeleteAuthProvider(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	// Check if provider exists
	provider, err := facade.GetAuthProvider().GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Check if provider was actually found (GORM callback may return nil error with empty struct)
	if provider == nil || provider.ID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	if err := facade.GetAuthProvider().Delete(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"message": "provider deleted successfully"}))
}

// TestAuthProvider tests the connection to an authentication provider
// POST /api/v1/admin/auth/providers/:id/test
func TestAuthProvider(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	provider, err := facade.GetAuthProvider().GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Check if provider was actually found (GORM callback may return nil error with empty struct)
	if provider == nil || provider.ID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	// Test based on provider type
	var testResult *TestAuthProviderResponse
	switch ProviderType(provider.Type) {
	case ProviderTypeLDAP:
		testResult = testLDAPProvider(provider)
	case ProviderTypeOIDC:
		testResult = testOIDCProvider(provider)
	default:
		testResult = &TestAuthProviderResponse{
			Success: false,
			Message: "unsupported provider type for testing",
		}
	}

	// Update provider status based on test result
	status := string(ProviderStatusActive)
	lastError := ""
	if !testResult.Success {
		status = string(ProviderStatusError)
		lastError = testResult.Message
	}
	_ = facade.GetAuthProvider().UpdateStatus(ctx, id, status, lastError)

	c.JSON(http.StatusOK, rest.SuccessResp(c, testResult))
}

// testLDAPProvider tests LDAP provider connection
func testLDAPProvider(provider *model.LensAuthProviders) *TestAuthProviderResponse {
	ldapProvider, err := ldappkg.NewProviderFromMap(provider.Config)
	if err != nil {
		return &TestAuthProviderResponse{
			Success: false,
			Message: fmt.Sprintf("failed to create LDAP provider: %v", err),
		}
	}
	defer ldapProvider.Close()

	result, err := ldapProvider.TestConnection(context.Background())
	if err != nil {
		return &TestAuthProviderResponse{
			Success: false,
			Message: fmt.Sprintf("test failed: %v", err),
		}
	}

	return &TestAuthProviderResponse{
		Success: result.Success,
		Message: result.Message,
		Details: result.Details,
	}
}

// testOIDCProvider tests OIDC provider connection
func testOIDCProvider(provider *model.LensAuthProviders) *TestAuthProviderResponse {
	config := provider.Config
	endpoint, _ := config["endpoint"].(string)
	clientID, _ := config["clientId"].(string)

	if endpoint == "" {
		return &TestAuthProviderResponse{
			Success: false,
			Message: "OIDC endpoint is not configured",
		}
	}

	// TODO: Implement actual OIDC discovery test
	// For now, return a placeholder response

	return &TestAuthProviderResponse{
		Success: true,
		Message: "OIDC configuration validated (connection test will be available after OIDC provider implementation)",
		Details: map[string]interface{}{
			"endpoint": endpoint,
			"clientId": clientID,
		},
	}
}

// Helper functions

func isValidProviderType(t ProviderType) bool {
	switch t {
	case ProviderTypeLDAP, ProviderTypeOIDC, ProviderTypeSafe:
		return true
	default:
		return false
	}
}

func toAuthProviderResponse(p *model.LensAuthProviders) *AuthProviderResponse {
	resp := &AuthProviderResponse{
		ID:        p.ID,
		Name:      p.Name,
		Type:      ProviderType(p.Type),
		Enabled:   p.Enabled,
		Status:    ProviderStatus(p.Status),
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
	if !p.LastCheckAt.IsZero() {
		resp.LastCheckAt = &p.LastCheckAt
	}
	if p.LastError != "" {
		resp.LastError = p.LastError
	}
	return resp
}

func toAuthProviderDetailResponse(p *model.LensAuthProviders) *AuthProviderDetailResponse {
	base := toAuthProviderResponse(p)
	
	// Mask sensitive fields in config
	config := make(map[string]interface{})
	for k, v := range p.Config {
		if k == "bindPassword" || k == "clientSecret" {
			if v != nil && v != "" {
				config[k] = "********"
			} else {
				config[k] = ""
			}
		} else {
			config[k] = v
		}
	}
	
	return &AuthProviderDetailResponse{
		AuthProviderResponse: *base,
		Config:               config,
	}
}
