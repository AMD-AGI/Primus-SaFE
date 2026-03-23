/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

const untaggedFilterValue = "__untagged__"

// ── Tag Usage Handler ─────────────────────────────────────────────────────

// GetTagUsage handles GET /api/v1/llm-gateway/tags/usage?start_date=...&end_date=...&timezone=...&tag=...&page=1&page_size=20
//
// Optional "tag" parameter filters logs to a specific tag.
// Use tag=__untagged__ to show only requests without custom tags.
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

	loc, err := resolveTimezone(c.Query("timezone"))
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest(err.Error()))
		return
	}

	tagFilters := splitTags(c.Query("tag"))

	page := parseIntParam(c.Query("page"), 1)
	pageSize := parseIntParam(c.Query("page_size"), defaultTagPageSize)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultTagPageSize
	}
	if pageSize > maxTagPageSize {
		pageSize = maxTagPageSize
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

	adjStart, adjEnd := expandDateRangeForTimezone(startDate, endDate, loc)
	allLogs, err := h.litellmClient.GetAllSpendLogs(c.Request.Context(), email, adjStart, adjEnd, maxSpendLogPages)
	if err != nil {
		klog.ErrorS(err, "GetTagUsage: LiteLLM query failed", "email", email)
		c.JSON(http.StatusBadGateway, gin.H{"errorMessage": "tag usage data temporarily unavailable, please try again later"})
		return
	}

	allLogs = filterLogsByLocalDate(allLogs, startDate, endDate, loc)

	if len(tagFilters) > 0 {
		allLogs = filterLogsByTags(allLogs, tagFilters)
	}

	result := aggregateByTag(allLogs, loc, tagFilters)

	sort.Slice(result.tags, func(i, j int) bool {
		return result.tags[i].Spend > result.tags[j].Spend
	})

	sort.Slice(result.daily, func(i, j int) bool {
		return result.daily[i].Date < result.daily[j].Date
	})

	total := len(result.tags)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	c.JSON(http.StatusOK, TagUsageResponse{
		UserEmail:               email,
		StartDate:               startDate,
		EndDate:                 endDate,
		TotalSpend:              result.totalSpend,
		TotalRequests:           result.totalRequests,
		TotalSuccessfulRequests: result.totalSuccessful,
		TotalFailedRequests:     result.totalFailed,
		TotalTokens:             result.totalTokens,
		Daily:                   result.daily,
		Tags:                    result.tags[start:end],
		Page:                    page,
		PageSize:                pageSize,
		Total:                   total,
		TotalPages:              totalPages,
	})
}

func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

// ── Tag filtering logic ───────────────────────────────────────────────────

// splitTags splits a comma-separated tag parameter into individual tags,
// trimming whitespace and ignoring empty segments.
func splitTags(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}

// filterLogsByTag keeps only log entries that match a single tag (retained for tests).
func filterLogsByTag(logs []SpendLogEntry, tag string) []SpendLogEntry {
	return filterLogsByTags(logs, []string{tag})
}

// filterLogsByTags keeps log entries matching ANY of the given tags (union).
// Use untaggedFilterValue ("__untagged__") to match entries with no custom tags.
func filterLogsByTags(logs []SpendLogEntry, tags []string) []SpendLogEntry {
	wantSet := make(map[string]struct{}, len(tags))
	wantUntagged := false
	for _, t := range tags {
		if t == untaggedFilterValue {
			wantUntagged = true
		} else {
			wantSet[t] = struct{}{}
		}
	}

	filtered := make([]SpendLogEntry, 0, len(logs)/2)
	for i := range logs {
		customTags := filterCustomTags(parseRequestTags(logs[i].RequestTags))
		if wantUntagged && len(customTags) == 0 {
			filtered = append(filtered, logs[i])
			continue
		}
		for _, ct := range customTags {
			if _, ok := wantSet[ct]; ok {
				filtered = append(filtered, logs[i])
				break
			}
		}
	}
	return filtered
}

// ── Tag aggregation logic ─────────────────────────────────────────────────

type tagAggResult struct {
	totalSpend    float64
	totalRequests int64
	totalSuccessful int64
	totalFailed     int64
	totalTokens     int64
	daily         []TagUsageDailyEntry
	tags          []TagUsageItem
}

type tagAccum struct {
	spend            float64
	requests         int64
	successful       int64
	failed           int64
	promptTokens     int64
	completionTokens int64
}

func isSuccessStatus(status string) bool {
	return status == "success" || status == "Success"
}

// aggregateByTag aggregates spend logs by tag. When tagFilters is non-empty,
// only the specified tags are included in the tag breakdown (filtered aggregation).
// When tagFilters is empty/nil, all tags are aggregated.
func aggregateByTag(logs []SpendLogEntry, loc *time.Location, tagFilters []string) tagAggResult {
	var filterSet map[string]struct{}
	if len(tagFilters) > 0 {
		filterSet = make(map[string]struct{}, len(tagFilters))
		for _, t := range tagFilters {
			if t == untaggedFilterValue {
				filterSet[""] = struct{}{}
			} else {
				filterSet[t] = struct{}{}
			}
		}
	}

	tagMap := make(map[string]*tagAccum)
	dailyMap := make(map[string]float64)
	var totalSpend float64
	var totalRequests, totalSuccessful, totalFailed, totalTokens int64

	for i := range logs {
		log := &logs[i]
		totalSpend += log.Spend
		totalRequests++
		tokens := log.PromptTokens + log.CompletionTokens
		totalTokens += tokens

		if isSuccessStatus(log.Status) {
			totalSuccessful++
		} else {
			totalFailed++
		}

		if t := parseTimestamp(log.StartTime); !t.IsZero() {
			day := t.In(loc).Format(dateLayout)
			dailyMap[day] += log.Spend
		}

		tags := parseRequestTags(log.RequestTags)
		customTags := filterCustomTags(tags)

		if len(customTags) == 0 {
			customTags = []string{""}
		}

		for _, tag := range customTags {
			if filterSet != nil {
				if _, ok := filterSet[tag]; !ok {
					continue
				}
			}
			accum, ok := tagMap[tag]
			if !ok {
				accum = &tagAccum{}
				tagMap[tag] = accum
			}
			accum.spend += log.Spend
			accum.requests++
			accum.promptTokens += log.PromptTokens
			accum.completionTokens += log.CompletionTokens
			if isSuccessStatus(log.Status) {
				accum.successful++
			} else {
				accum.failed++
			}
		}
	}

	items := make([]TagUsageItem, 0, len(tagMap))
	for tag, accum := range tagMap {
		item := TagUsageItem{
			Spend:              accum.spend,
			APIRequests:        accum.requests,
			SuccessfulRequests: accum.successful,
			FailedRequests:     accum.failed,
			PromptTokens:       accum.promptTokens,
			CompletionTokens:   accum.completionTokens,
		}
		if tag == "" {
			item.TagName = nil
		} else {
			t := tag
			item.TagName = &t
		}
		items = append(items, item)
	}

	daily := make([]TagUsageDailyEntry, 0, len(dailyMap))
	for date, spend := range dailyMap {
		daily = append(daily, TagUsageDailyEntry{Date: date, Spend: spend})
	}

	return tagAggResult{
		totalSpend:    totalSpend,
		totalRequests: totalRequests,
		totalSuccessful: totalSuccessful,
		totalFailed:     totalFailed,
		totalTokens:     totalTokens,
		daily:         daily,
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
