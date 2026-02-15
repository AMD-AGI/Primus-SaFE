// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package logs

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// DebugLogMatchRequest log match test request
type DebugLogMatchRequest struct {
	Log string `json:"log" binding:"required"` // Raw log content
}

// DebugLogMatchResponse log match test response
type DebugLogMatchResponse struct {
	LogLength   int               `json:"log_length"`
	CleanedLog  string            `json:"cleaned_log"`
	Matched     bool              `json:"matched"`
	PatternID   int64             `json:"pattern_id,omitempty"`
	PatternStr  string            `json:"pattern,omitempty"`
	Framework   string            `json:"framework,omitempty"`
	Groups      map[string]string `json:"groups,omitempty"`
	GroupsCount int               `json:"groups_count"`
	Performance interface{}       `json:"performance,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// DebugTestLogMatch tests log matching against the global pattern registry
// POST /api/v1/debug/test-log-match
func DebugTestLogMatch(ctx *gin.Context) {
	var req DebugLogMatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", err))
		return
	}

	response := &DebugLogMatchResponse{
		LogLength: len(req.Log),
	}

	// Clean ANSI codes
	cleanLog := stripAnsiCodes(req.Log)
	response.CleanedLog = cleanLog

	if globalRegistry == nil {
		response.Error = "global pattern registry not initialized"
		ctx.JSON(http.StatusServiceUnavailable, rest.ErrorResp(ctx.Request.Context(),
			http.StatusServiceUnavailable, response.Error, nil))
		return
	}

	// Try to match performance pattern via global registry
	result := globalRegistry.MatchPerformance(cleanLog)
	if !result.Matched {
		response.Matched = false
		response.Error = "log does not match any performance pattern"
		ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
		return
	}

	// Pattern matched successfully
	response.Matched = true
	response.PatternID = result.PatternID
	response.PatternStr = result.Pattern
	response.Framework = result.Framework
	response.Groups = result.Groups
	response.GroupsCount = len(result.Groups)

	// Convert groups to performance data
	if len(result.Groups) > 0 {
		perf, err := ConvertGroupsToPerformance(result.Groups)
		if err != nil {
			response.Error = "failed to convert groups to performance: " + err.Error()
			logrus.Warnf("[DebugTestLogMatch] Conversion failed: %v", err)
		} else {
			response.Performance = perf
			logrus.Infof("[DebugTestLogMatch] Successfully converted %d groups to performance data", len(result.Groups))
		}
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
}

// DebugListFrameworks lists global registry info
// GET /api/v1/debug/frameworks
func DebugListFrameworks(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), GetGlobalRegistryDebugInfo()))
}

// DebugFrameworkPatterns returns global pattern registry debug info
// GET /api/v1/debug/frameworks/:name/patterns
func DebugFrameworkPatterns(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), GetGlobalRegistryDebugInfo()))
}
