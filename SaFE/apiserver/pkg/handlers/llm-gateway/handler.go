/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"net/http"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commoncrypto "github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// Handler manages LLM Gateway API endpoints.
type Handler struct {
	accessController *authority.AccessController
	dbClient         dbclient.Interface
	litellmClient    *LiteLLMClient
	crypto           *commoncrypto.Crypto
}

// NewHandler creates a new LLM Gateway handler.
func NewHandler(accessController *authority.AccessController, dbClient dbclient.Interface) (*Handler, error) {
	endpoint := commonconfig.GetLLMGatewayEndpoint()
	adminKey := commonconfig.GetLLMGatewayAdminKey()
	teamID := commonconfig.GetLLMGatewayTeamID()

	if endpoint == "" || teamID == "" {
		klog.Warning("LLM Gateway: configuration incomplete (endpoint and teamID required), feature disabled")
		return nil, nil
	}
	if adminKey == "" {
		klog.Warning("LLM Gateway: litellm_admin_key not configured, calling LiteLLM API without authentication")
	}

	crypto := commoncrypto.NewCrypto()

	return &Handler{
		accessController: accessController,
		dbClient:         dbClient,
		litellmClient:    NewLiteLLMClient(endpoint, adminKey, teamID),
		crypto:           crypto,
	}, nil
}

// InitRoutes registers LLM Gateway routes on the Gin engine.
func InitRoutes(engine *gin.Engine, handler *Handler) {
	if handler == nil {
		klog.Info("LLM Gateway: handler is nil, routes not registered")
		return
	}

	// Management API (requires SaFE user auth)
	mgmt := engine.Group("/api/v1/llm-gateway")
	mgmt.Use(func(c *gin.Context) {
		if err := authority.ParseToken(c); err != nil {
			apiutils.AbortWithApiError(c, err)
			return
		}
		c.Next()
	})
	{
		mgmt.POST("/binding", handler.CreateBinding)
		mgmt.PUT("/binding", handler.UpdateBinding)
		mgmt.DELETE("/binding", handler.DeleteBinding)
		mgmt.GET("/binding", handler.GetBinding)
		// Resolve: called by LiteLLM custom_auth hook with user's SaFE API Key
		mgmt.GET("/resolve", handler.Resolve)
	}

	klog.Info("LLM Gateway: routes registered successfully")
}

// ── Request/Response types ────────────────────────────────────────────────

type CreateBindingRequest struct {
	ApimKey string `json:"apim_key" binding:"required"`
}

type BindingResponse struct {
	UserEmail  string `json:"user_email"`
	KeyAlias   string `json:"key_alias"`
	HasAPIMKey bool   `json:"has_apim_key"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type ResolveResponse struct {
	UserEmail  string `json:"user_email"`
	VirtualKey string `json:"virtual_key"`
}

// ── Handlers ──────────────────────────────────────────────────────────────

// CreateBinding handles POST /api/v1/llm-gateway/binding
func (h *Handler) CreateBinding(c *gin.Context) {
	var req CreateBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request: "+err.Error()))
		return
	}

	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("user email not found"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to check existing binding: "+err.Error()))
		return
	}
	if existing != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewAlreadyExist("binding already exists for "+email+", use PUT to update"))
		return
	}

	apimKeyHash := dbclient.ComputeApimKeyHash(req.ApimKey)

	encryptedApimKey, err := h.crypto.Encrypt([]byte(req.ApimKey))
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to encrypt APIM Key"))
		return
	}

	litellmResp, err := h.litellmClient.CreateServiceAccountKey(c.Request.Context(), email, req.ApimKey)
	if err != nil {
		klog.ErrorS(err, "failed to create LiteLLM key", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to create Virtual Key in LiteLLM: "+err.Error()))
		return
	}

	encryptedVKey, err := h.crypto.Encrypt([]byte(litellmResp.Key))
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to encrypt Virtual Key"))
		return
	}

	binding := &dbclient.LLMGatewayUserBinding{
		UserEmail:         email,
		ApimKey:           encryptedApimKey,
		ApimKeyHash:       apimKeyHash,
		LiteLLMVirtualKey: encryptedVKey,
		LiteLLMKeyHash:    litellmResp.TokenID,
		KeyAlias:          email,
	}

	if err := h.dbClient.CreateLLMBinding(c.Request.Context(), binding); err != nil {
		_ = h.litellmClient.DeleteKey(c.Request.Context(), litellmResp.TokenID)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to save binding: "+err.Error()))
		return
	}

	klog.Infof("LLM Gateway: binding created for %s", email)
	c.JSON(http.StatusCreated, BindingResponse{
		UserEmail:  email,
		KeyAlias:   email,
		HasAPIMKey: true,
		CreatedAt:  binding.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// UpdateBinding handles PUT /api/v1/llm-gateway/binding
func (h *Handler) UpdateBinding(c *gin.Context) {
	var req CreateBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request: "+err.Error()))
		return
	}

	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("user email not found"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to query binding: "+err.Error()))
		return
	}
	if existing == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("no binding found for "+email))
		return
	}

	newApimKeyHash := dbclient.ComputeApimKeyHash(req.ApimKey)

	oldApimKey, _ := h.crypto.Decrypt(existing.ApimKey)

	if err := h.litellmClient.UpdateKeyMetadata(c.Request.Context(), existing.LiteLLMKeyHash, req.ApimKey, email); err != nil {
		klog.ErrorS(err, "failed to update LiteLLM key metadata", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to update LiteLLM key: "+err.Error()))
		return
	}

	encryptedApimKey, err := h.crypto.Encrypt([]byte(req.ApimKey))
	if err != nil {
		if oldApimKey != "" {
			_ = h.litellmClient.UpdateKeyMetadata(c.Request.Context(), existing.LiteLLMKeyHash, oldApimKey, email)
		}
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to encrypt APIM Key"))
		return
	}
	existing.ApimKey = encryptedApimKey
	existing.ApimKeyHash = newApimKeyHash
	if err := h.dbClient.UpdateLLMBinding(c.Request.Context(), existing); err != nil {
		if oldApimKey != "" {
			_ = h.litellmClient.UpdateKeyMetadata(c.Request.Context(), existing.LiteLLMKeyHash, oldApimKey, email)
		}
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to update binding: "+err.Error()))
		return
	}

	klog.Infof("LLM Gateway: binding updated for %s", email)
	c.JSON(http.StatusOK, BindingResponse{
		UserEmail:  email,
		KeyAlias:   email,
		HasAPIMKey: true,
		UpdatedAt:  existing.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// DeleteBinding handles DELETE /api/v1/llm-gateway/binding
func (h *Handler) DeleteBinding(c *gin.Context) {
	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("user email not found"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to query binding: "+err.Error()))
		return
	}
	if existing == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("no binding found for "+email))
		return
	}

	if err := h.litellmClient.DeleteKey(c.Request.Context(), existing.LiteLLMKeyHash); err != nil {
		klog.ErrorS(err, "failed to delete LiteLLM key", "email", email)
	}

	if err := h.dbClient.DeleteLLMBinding(c.Request.Context(), email); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to delete binding: "+err.Error()))
		return
	}

	klog.Infof("LLM Gateway: binding deleted for %s", email)
	c.Status(http.StatusNoContent)
}

// GetBinding handles GET /api/v1/llm-gateway/binding
func (h *Handler) GetBinding(c *gin.Context) {
	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("user email not found"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to query binding: "+err.Error()))
		return
	}

	if existing == nil {
		c.JSON(http.StatusOK, BindingResponse{
			UserEmail:  email,
			HasAPIMKey: false,
		})
		return
	}

	c.JSON(http.StatusOK, BindingResponse{
		UserEmail:  email,
		KeyAlias:   existing.KeyAlias,
		HasAPIMKey: true,
		CreatedAt:  existing.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  existing.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// Resolve handles GET /api/v1/internal/llm-gateway/resolve
func (h *Handler) Resolve(c *gin.Context) {
	email := h.getUserEmail(c)
	if email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unable to identify user"})
		return
	}

	binding, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "LLM Gateway resolve: DB error", "email", email)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if binding == nil {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "No APIM Key binding found. Please upload your APIM Key on the SaFE platform first.",
		})
		return
	}

	virtualKey, err := h.crypto.Decrypt(binding.LiteLLMVirtualKey)
	if err != nil {
		klog.ErrorS(err, "LLM Gateway resolve: failed to decrypt VKey", "email", email)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, ResolveResponse{
		UserEmail:  email,
		VirtualKey: virtualKey,
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────

// getUserEmail retrieves the user's email by looking up the K8s User CR
// via AccessController.GetRequestUser (same pattern as resources/cd-handlers).
// Falls back to userName if User CR lookup fails or email annotation is not set.
func (h *Handler) getUserEmail(c *gin.Context) string {
	userId := c.GetString(common.UserId)
	if userId == "" {
		return ""
	}

	// Look up User CR to get the real email
	user, err := h.accessController.GetRequestUser(c.Request.Context(), userId)
	if err != nil {
		klog.V(4).InfoS("LLM Gateway: failed to get user, falling back to userName",
			"userId", userId, "error", err)
	} else {
		if email := v1.GetUserEmail(user); email != "" {
			return email
		}
	}

	// Fallback: userName
	if name := c.GetString(common.UserName); name != "" {
		return name
	}
	return userId
}
