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

	proxy, err := newLLMProxy(endpoint)
	if err != nil {
		return nil, err
	}

	crypto := commoncrypto.NewCrypto()

	return &Handler{
		accessController: accessController,
		dbClient:         dbClient,
		litellmClient:    NewLiteLLMClient(endpoint, adminKey, teamID),
		crypto:           crypto,
		proxy:            proxy,
	}, nil
}

// InitRoutes registers LLM Gateway routes on the Gin engine.
func InitRoutes(engine *gin.Engine, handler *Handler) {
	if handler == nil {
		klog.Info("LLM Gateway: handler is nil, routes not registered")
		return
	}

	authMiddleware := func(c *gin.Context) {
		if err := authority.ParseToken(c); err != nil {
			apiutils.AbortWithApiError(c, err)
			return
		}
		c.Next()
	}

	// Management API (requires SaFE user auth via Cookie or API Key)
	mgmt := engine.Group("/api/v1/llm-gateway")
	mgmt.Use(authMiddleware)
	{
		mgmt.POST("/binding", handler.CreateBinding)
		mgmt.PUT("/binding", handler.UpdateBinding)
		mgmt.DELETE("/binding", handler.DeleteBinding)
		mgmt.GET("/binding", handler.GetBinding)
		mgmt.GET("/usage", handler.GetUsage)
		mgmt.GET("/summary", handler.GetSummary)
		mgmt.GET("/budget", handler.GetBudget)
		mgmt.PUT("/budget", handler.SetBudget)
		mgmt.DELETE("/budget", handler.RemoveBudget)
		mgmt.GET("/tags/usage", handler.GetTagUsage)
	}

	// LLM reverse proxy: /api/v1/llm-proxy/* -> LiteLLM
	// Authenticates SaFE API Key, resolves Virtual Key, and forwards the request.
	// User calls: /api/llm-proxy/v1/chat/completions
	// Web proxy forwards as: /api/v1/llm-proxy/v1/chat/completions
	// Director strips /api/v1/llm-proxy, forwards /v1/chat/completions to LiteLLM.
	llmProxy := engine.Group(llmProxyPrefix)
	llmProxy.Use(authMiddleware)
	llmProxy.Any("/*proxyPath", handler.ProxyLLMRequest)

	klog.Info("LLM Gateway: routes registered successfully (management + LLM proxy)")
}

// ── Handlers ──────────────────────────────────────────────────────────────

// CreateBinding handles POST /api/v1/llm-gateway/binding
func (h *Handler) CreateBinding(c *gin.Context) {
	var req CreateBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("apim_key is required"))
		return
	}

	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("unable to identify user email"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "CreateBinding: DB query failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}
	if existing != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewAlreadyExist("APIM Key already bound for "+email+", use PUT to update"))
		return
	}

	apimKeyHash := dbclient.ComputeApimKeyHash(req.ApimKey)

	encryptedApimKey, err := h.crypto.Encrypt([]byte(req.ApimKey))
	if err != nil {
		klog.ErrorS(err, "CreateBinding: failed to encrypt APIM Key", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}

	if err := h.litellmClient.CreateUser(c.Request.Context(), email); err != nil {
		klog.ErrorS(err, "CreateBinding: LiteLLM create user failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("LLM service unavailable, please try again later"))
		return
	}

	litellmResp, err := h.litellmClient.CreateKey(c.Request.Context(), email, req.ApimKey)
	if err != nil {
		klog.ErrorS(err, "CreateBinding: LiteLLM create key failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("LLM service unavailable, please try again later"))
		return
	}

	encryptedVKey, err := h.crypto.Encrypt([]byte(litellmResp.Key))
	if err != nil {
		klog.ErrorS(err, "CreateBinding: failed to encrypt Virtual Key", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
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
		klog.ErrorS(err, "CreateBinding: DB save failed, rolling back LiteLLM key", "email", email)
		_ = h.litellmClient.DeleteKey(c.Request.Context(), litellmResp.TokenID, email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to save binding, please try again later"))
		return
	}

	klog.Infof("LLM Gateway: binding created for %s", email)
	c.JSON(http.StatusCreated, BindingResponse{
		UserEmail:  email,
		KeyAlias:   email,
		HasAPIMKey: true,
		VirtualKey: litellmResp.Key,
		CreatedAt:  binding.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// UpdateBinding handles PUT /api/v1/llm-gateway/binding
func (h *Handler) UpdateBinding(c *gin.Context) {
	var req CreateBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("apim_key is required"))
		return
	}

	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("unable to identify user email"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "UpdateBinding: DB query failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}
	if existing == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("no APIM Key bound yet, please bind first"))
		return
	}

	newApimKeyHash := dbclient.ComputeApimKeyHash(req.ApimKey)

	oldApimKey, _ := h.crypto.Decrypt(existing.ApimKey)

	if err := h.litellmClient.UpdateKeyMetadata(c.Request.Context(), existing.LiteLLMKeyHash, req.ApimKey, email); err != nil {
		klog.ErrorS(err, "UpdateBinding: LiteLLM update key failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("LLM service unavailable, please try again later"))
		return
	}

	encryptedApimKey, err := h.crypto.Encrypt([]byte(req.ApimKey))
	if err != nil {
		klog.ErrorS(err, "UpdateBinding: failed to encrypt APIM Key", "email", email)
		if oldApimKey != "" {
			_ = h.litellmClient.UpdateKeyMetadata(c.Request.Context(), existing.LiteLLMKeyHash, oldApimKey, email)
		}
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}
	existing.ApimKey = encryptedApimKey
	existing.ApimKeyHash = newApimKeyHash
	if err := h.dbClient.UpdateLLMBinding(c.Request.Context(), existing); err != nil {
		klog.ErrorS(err, "UpdateBinding: DB save failed, rolling back LiteLLM key", "email", email)
		if oldApimKey != "" {
			_ = h.litellmClient.UpdateKeyMetadata(c.Request.Context(), existing.LiteLLMKeyHash, oldApimKey, email)
		}
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to update binding, please try again later"))
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
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("unable to identify user email"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "DeleteBinding: DB query failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}
	if existing == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("no APIM Key bound for "+email))
		return
	}

	if err := h.litellmClient.DeleteKey(c.Request.Context(), existing.LiteLLMKeyHash, existing.KeyAlias); err != nil {
		klog.ErrorS(err, "DeleteBinding: LiteLLM delete key failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("LLM service unavailable, please try again later"))
		return
	}

	if err := h.dbClient.DeleteLLMBinding(c.Request.Context(), email); err != nil {
		klog.ErrorS(err, "DeleteBinding: DB delete failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to delete binding, please try again later"))
		return
	}

	klog.Infof("LLM Gateway: binding deleted for %s", email)
	c.Status(http.StatusNoContent)
}

// GetBinding handles GET /api/v1/llm-gateway/binding
func (h *Handler) GetBinding(c *gin.Context) {
	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("unable to identify user email"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "GetBinding: DB query failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}

	if existing == nil {
		c.JSON(http.StatusOK, BindingResponse{
			UserEmail:  email,
			HasAPIMKey: false,
		})
		return
	}

	var apimKeyHint string
	if plainKey, err := h.crypto.Decrypt(existing.ApimKey); err == nil && plainKey != "" {
		apimKeyHint = maskKey(plainKey)
	}

	c.JSON(http.StatusOK, BindingResponse{
		UserEmail:   email,
		KeyAlias:    existing.KeyAlias,
		HasAPIMKey:  true,
		ApimKeyHint: apimKeyHint,
		CreatedAt:   existing.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   existing.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// GetSummary handles GET /api/v1/llm-gateway/summary
// Returns cumulative total spend for the user (not tied to a date range).
func (h *Handler) GetSummary(c *gin.Context) {
	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("unable to identify user email"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "GetSummary: DB query failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}
	if existing == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("no APIM Key bound yet, summary unavailable"))
		return
	}

	userInfo, err := h.litellmClient.GetUserInfo(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "GetSummary: LiteLLM query failed", "email", email)
		c.JSON(http.StatusBadGateway, gin.H{"errorMessage": "summary data temporarily unavailable, please try again later"})
		return
	}

	c.JSON(http.StatusOK, SummaryResponse{
		UserEmail:  email,
		TotalSpend: userInfo.UserInfo.Spend,
		ModelSpend: userInfo.UserInfo.ModelSpend,
	})
}

// GetUsage handles GET /api/v1/llm-gateway/usage?start_date=...&end_date=...
func (h *Handler) GetUsage(c *gin.Context) {
	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("unable to identify user email"))
		return
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate == "" || endDate == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("start_date and end_date are required, format: YYYY-MM-DD"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "GetUsage: DB query failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}
	if existing == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("no APIM Key bound yet, usage data unavailable"))
		return
	}

	activity, err := h.litellmClient.GetUserDailyActivity(c.Request.Context(), email, startDate, endDate)
	if err != nil {
		klog.ErrorS(err, "GetUsage: LiteLLM query failed", "email", email)
		c.JSON(http.StatusBadGateway, gin.H{"errorMessage": "usage data temporarily unavailable, please try again later"})
		return
	}

	totalTokens := activity.Metadata.TotalPromptTokens + activity.Metadata.TotalCompletionTokens

	daily := make([]UsageDailyEntry, 0, len(activity.Results))
	for _, r := range activity.Results {
		entry := UsageDailyEntry{
			Date:               r.Date,
			Spend:              r.Metrics.Spend,
			PromptTokens:       r.Metrics.PromptTokens,
			CompletionTokens:   r.Metrics.CompletionTokens,
			TotalTokens:        r.Metrics.TotalTokens,
			APIRequests:        r.Metrics.APIRequests,
			SuccessfulRequests: r.Metrics.SuccessfulRequests,
			FailedRequests:     r.Metrics.FailedRequests,
		}
		if r.Breakdown != nil && len(r.Breakdown.Models) > 0 {
			entry.Models = make(map[string]UsageModelData, len(r.Breakdown.Models))
			for model, mwm := range r.Breakdown.Models {
				m := mwm.Metrics
				entry.Models[model] = UsageModelData{
					Spend:              m.Spend,
					PromptTokens:       m.PromptTokens,
					CompletionTokens:   m.CompletionTokens,
					APIRequests:        m.APIRequests,
					SuccessfulRequests: m.SuccessfulRequests,
					FailedRequests:     m.FailedRequests,
				}
			}
		}
		daily = append(daily, entry)
	}

	c.JSON(http.StatusOK, UsageResponse{
		UserEmail:               email,
		TotalSpend:              activity.Metadata.TotalSpend,
		TotalPromptTokens:       activity.Metadata.TotalPromptTokens,
		TotalCompletionTokens:   activity.Metadata.TotalCompletionTokens,
		TotalTokens:             totalTokens,
		TotalAPIRequests:        activity.Metadata.TotalAPIRequests,
		TotalSuccessfulRequests: activity.Metadata.TotalSuccessfulRequests,
		TotalFailedRequests:     activity.Metadata.TotalFailedRequests,
		Daily:                   daily,
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
	if h.accessController != nil {
		user, err := h.accessController.GetRequestUser(c.Request.Context(), userId)
		if err != nil {
			klog.V(4).InfoS("LLM Gateway: failed to get user, falling back to userName",
				"userId", userId, "error", err)
		} else {
			if email := v1.GetUserEmail(user); email != "" {
				return email
			}
		}
	}

	// Fallback: userName
	if name := c.GetString(common.UserName); name != "" {
		return name
	}
	return userId
}

// maskKey returns a masked version of a key, showing the first 4 and last 4 characters.
// e.g. "abcdefghijklmnop" → "abcd********mnop"
func maskKey(key string) string {
	if len(key) <= 8 {
		return key[:2] + "****"
	}
	return key[:4] + "********" + key[len(key)-4:]
}
