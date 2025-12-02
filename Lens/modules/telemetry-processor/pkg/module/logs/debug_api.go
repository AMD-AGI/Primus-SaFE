package logs

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// DebugLogMatchRequest 日志匹配测试请求
type DebugLogMatchRequest struct {
	Framework string `json:"framework" binding:"required"` // 框架名称，如 "megatron", "primus"
	Log       string `json:"log" binding:"required"`       // 原始日志内容
}

// DebugLogMatchResponse 日志匹配测试响应
type DebugLogMatchResponse struct {
	Framework   string            `json:"framework"`
	LogLength   int               `json:"log_length"`
	CleanedLog  string            `json:"cleaned_log"`
	Matched     bool              `json:"matched"`
	PatternName string            `json:"pattern_name,omitempty"`
	Groups      map[string]string `json:"groups,omitempty"`
	GroupsCount int               `json:"groups_count"`
	Performance interface{}       `json:"performance,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// DebugTestLogMatch 测试日志匹配和性能数据转换
// POST /api/v1/debug/test-log-match
//
// Request Body:
//
//	{
//	  "framework": "megatron",
//	  "log": "[[32m20251202 09:12:08[0m]... iteration 126/ 5000 ..."
//	}
//
// Response:
//
//	{
//	  "framework": "megatron",
//	  "matched": true,
//	  "pattern_name": "megatron_performance_v1",
//	  "groups": {
//	    "CurrentIteration": "126",
//	    "MemUsage": "153.81",
//	    ...
//	  },
//	  "groups_count": 17,
//	  "performance": {
//	    "current_iteration": 126,
//	    "mem_usages": 153.81,
//	    ...
//	  }
//	}
func DebugTestLogMatch(ctx *gin.Context) {
	var req DebugLogMatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", err))
		return
	}

	response := &DebugLogMatchResponse{
		Framework: req.Framework,
		LogLength: len(req.Log),
	}

	// Clean ANSI codes
	cleanLog := stripAnsiCodes(req.Log)
	response.CleanedLog = cleanLog

	// Get pattern matcher for framework
	matcher, ok := patternMatchers[req.Framework]
	if !ok {
		response.Error = "framework not found or not loaded"
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(),
			http.StatusNotFound, response.Error, nil))
		return
	}

	// Try to match performance pattern
	result := matcher.MatchPerformance(cleanLog)
	if !result.Matched {
		response.Matched = false
		response.Error = "log does not match any performance pattern"
		ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
		return
	}

	// Pattern matched successfully
	response.Matched = true
	response.PatternName = result.Pattern
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

// DebugListFrameworks 列出所有已加载的框架
// GET /api/v1/debug/frameworks
func DebugListFrameworks(ctx *gin.Context) {
	frameworks := GetFrameworkList()

	result := gin.H{
		"total":      len(frameworks),
		"frameworks": frameworks,
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), result))
}

// DebugFrameworkPatterns 查看指定框架的所有模式信息
// GET /api/v1/debug/frameworks/:name/patterns
func DebugFrameworkPatterns(ctx *gin.Context) {
	frameworkName := ctx.Param("name")

	matchers := GetPatternMatchersInfo()
	info, ok := matchers[frameworkName]
	if !ok {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(),
			http.StatusNotFound, "framework not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), info))
}
