/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"net/http"

	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// ── Budget Request/Response types ─────────────────────────────────────────

type SetBudgetRequest struct {
	MaxBudget float64 `json:"max_budget" binding:"required,gt=0"`
}

type BudgetResponse struct {
	UserEmail      string   `json:"user_email"`
	Spend          float64  `json:"spend"`
	MaxBudget      *float64 `json:"max_budget"`
	Remaining      *float64 `json:"remaining"`
	BudgetExceeded bool     `json:"budget_exceeded"`
	UsagePercent   *float64 `json:"usage_percent"`
	Message        string   `json:"message,omitempty"`
}

// ── Budget Handlers ───────────────────────────────────────────────────────

// GetBudget handles GET /api/v1/llm-gateway/budget
func (h *Handler) GetBudget(c *gin.Context) {
	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("unable to identify user email"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "GetBudget: DB query failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}
	if existing == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("no APIM Key bound yet"))
		return
	}

	keyInfo, err := h.litellmClient.GetKeyInfo(c.Request.Context(), existing.LiteLLMKeyHash)
	if err != nil {
		klog.ErrorS(err, "GetBudget: LiteLLM query failed", "email", email)
		c.JSON(http.StatusBadGateway, gin.H{"errorMessage": "budget data temporarily unavailable, please try again later"})
		return
	}

	c.JSON(http.StatusOK, buildBudgetResponse(email, keyInfo, ""))
}

// SetBudget handles PUT /api/v1/llm-gateway/budget
func (h *Handler) SetBudget(c *gin.Context) {
	var req SetBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("max_budget is required and must be > 0"))
		return
	}

	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("unable to identify user email"))
		return
	}
	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "SetBudget: DB query failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}
	if existing == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("no APIM Key bound yet"))
		return
	}

	budget := req.MaxBudget
	if err := h.litellmClient.UpdateKeyBudget(c.Request.Context(), existing.LiteLLMKeyHash, &budget); err != nil {
		klog.ErrorS(err, "SetBudget: LiteLLM update failed", "email", email, "max_budget", budget)
		c.JSON(http.StatusBadGateway, gin.H{"errorMessage": "failed to update budget, please try again later"})
		return
	}

	keyInfo, err := h.litellmClient.GetKeyInfo(c.Request.Context(), existing.LiteLLMKeyHash)
	if err != nil {
		klog.ErrorS(err, "SetBudget: LiteLLM query failed after update", "email", email)
		c.JSON(http.StatusOK, BudgetResponse{
			UserEmail: email,
			MaxBudget: &budget,
			Message:   "Budget updated successfully",
		})
		return
	}

	klog.Infof("LLM Gateway: budget set to $%.2f for %s", budget, email)
	c.JSON(http.StatusOK, buildBudgetResponse(email, keyInfo, "Budget updated successfully"))
}

// RemoveBudget handles DELETE /api/v1/llm-gateway/budget
func (h *Handler) RemoveBudget(c *gin.Context) {
	email := h.getUserEmail(c)
	if email == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("unable to identify user email"))
		return
	}

	existing, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "RemoveBudget: DB query failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}
	if existing == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("no APIM Key bound yet"))
		return
	}

	if err := h.litellmClient.UpdateKeyBudget(c.Request.Context(), existing.LiteLLMKeyHash, nil); err != nil {
		klog.ErrorS(err, "RemoveBudget: LiteLLM update failed", "email", email)
		c.JSON(http.StatusBadGateway, gin.H{"errorMessage": "failed to remove budget limit, please try again later"})
		return
	}

	keyInfo, err := h.litellmClient.GetKeyInfo(c.Request.Context(), existing.LiteLLMKeyHash)
	if err != nil {
		klog.ErrorS(err, "RemoveBudget: LiteLLM query failed after update", "email", email)
		c.JSON(http.StatusOK, BudgetResponse{
			UserEmail: email,
			Message:   "Budget limit removed",
		})
		return
	}

	klog.Infof("LLM Gateway: budget removed for %s", email)
	c.JSON(http.StatusOK, buildBudgetResponse(email, keyInfo, "Budget limit removed"))
}

// ── Budget helpers ────────────────────────────────────────────────────────

func buildBudgetResponse(email string, info *KeyInfoData, message string) BudgetResponse {
	resp := BudgetResponse{
		UserEmail: email,
		Spend:     info.Spend,
		MaxBudget: info.MaxBudget,
		Message:   message,
	}

	if info.MaxBudget != nil {
		remaining := *info.MaxBudget - info.Spend
		resp.Remaining = &remaining
		resp.BudgetExceeded = info.Spend >= *info.MaxBudget
		if *info.MaxBudget > 0 {
			pct := (info.Spend / *info.MaxBudget) * 100
			resp.UsagePercent = &pct
		}
	}

	return resp
}
