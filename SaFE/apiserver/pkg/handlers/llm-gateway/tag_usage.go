/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"encoding/json"
	"net/http"
	"strings"

	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// ── Tag Usage Response types ──────────────────────────────────────────────

type TagUsageResponse struct {
	UserEmail     string         `json:"user_email"`
	StartDate     string         `json:"start_date"`
	EndDate       string         `json:"end_date"`
	TotalSpend    float64        `json:"total_spend"`
	TotalRequests int64          `json:"total_requests"`
	Tags          []TagUsageItem `json:"tags"`
}

type TagUsageItem struct {
	TagName          *string `json:"tag_name"`
	Spend            float64 `json:"spend"`
	APIRequests      int64   `json:"api_requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
}

// ── Tag Usage Handler ─────────────────────────────────────────────────────

// GetTagUsage handles GET /api/v1/llm-gateway/tags/usage?start_date=...&end_date=...
func (h *Handler) GetTagUsage(c *gin.Context) {
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
		klog.ErrorS(err, "GetTagUsage: DB query failed", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("service temporarily unavailable, please try again later"))
		return
	}
	if existing == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("no APIM Key bound yet"))
		return
	}

	logsResp, err := h.litellmClient.GetSpendLogs(c.Request.Context(), email, startDate, endDate, 100)
	if err != nil {
		klog.ErrorS(err, "GetTagUsage: LiteLLM query failed", "email", email)
		c.JSON(http.StatusBadGateway, gin.H{"errorMessage": "tag usage data temporarily unavailable, please try again later"})
		return
	}

	result := aggregateByTag(logsResp.Data)

	c.JSON(http.StatusOK, TagUsageResponse{
		UserEmail:     email,
		StartDate:     startDate,
		EndDate:       endDate,
		TotalSpend:    result.totalSpend,
		TotalRequests: result.totalRequests,
		Tags:          result.tags,
	})
}

// ── Tag aggregation logic ─────────────────────────────────────────────────

type tagAggResult struct {
	totalSpend    float64
	totalRequests int64
	tags          []TagUsageItem
}

type tagAccum struct {
	spend            float64
	requests         int64
	promptTokens     int64
	completionTokens int64
}

func aggregateByTag(logs []SpendLogEntry) tagAggResult {
	tagMap := make(map[string]*tagAccum)
	var totalSpend float64
	var totalRequests int64

	for i := range logs {
		log := &logs[i]
		totalSpend += log.Spend
		totalRequests++

		tags := parseRequestTags(log.RequestTags)
		customTags := filterCustomTags(tags)

		if len(customTags) == 0 {
			customTags = []string{""}
		}

		for _, tag := range customTags {
			accum, ok := tagMap[tag]
			if !ok {
				accum = &tagAccum{}
				tagMap[tag] = accum
			}
			accum.spend += log.Spend
			accum.requests++
			accum.promptTokens += log.PromptTokens
			accum.completionTokens += log.CompletionTokens
		}
	}

	items := make([]TagUsageItem, 0, len(tagMap))
	for tag, accum := range tagMap {
		item := TagUsageItem{
			Spend:            accum.spend,
			APIRequests:      accum.requests,
			PromptTokens:     accum.promptTokens,
			CompletionTokens: accum.completionTokens,
		}
		if tag == "" {
			item.TagName = nil
		} else {
			t := tag
			item.TagName = &t
		}
		items = append(items, item)
	}

	return tagAggResult{
		totalSpend:    totalSpend,
		totalRequests: totalRequests,
		tags:          items,
	}
}

func parseRequestTags(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}

	var tags []string
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil
	}
	return tags
}

// filterCustomTags removes LiteLLM auto-generated tags (User-Agent headers).
func filterCustomTags(tags []string) []string {
	var custom []string
	for _, tag := range tags {
		if strings.HasPrefix(tag, "User-Agent:") {
			continue
		}
		custom = append(custom, tag)
	}
	return custom
}
