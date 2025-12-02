package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/logs"
)

// GetPatternMatchers 获取所有框架的 pattern matcher 信息（用于调试）
// @Summary 获取 Pattern Matcher 列表
// @Description 返回所有已初始化的 pattern matcher 的详细信息，包括各种匹配模式
// @Tags Debug
// @Produce json
// @Success 200 {object} map[string]interface{} "Pattern matcher 信息列表"
// @Router /debug/pattern-matchers [get]
func GetPatternMatchers(c *gin.Context) {
	matchersInfo := logs.GetPatternMatchersInfo()

	if len(matchersInfo) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message":   "Pattern matchers not initialized yet",
			"frameworks": []string{},
			"total":     0,
		})
		return
	}

	// Calculate total patterns across all frameworks
	totalPatterns := 0
	for _, info := range matchersInfo {
		totalPatterns += info.TotalPatterns
	}

	logrus.Debugf("Returning pattern matcher info for %d frameworks with %d total patterns",
		len(matchersInfo), totalPatterns)

	c.JSON(http.StatusOK, gin.H{
		"frameworks":      matchersInfo,
		"total_frameworks": len(matchersInfo),
		"total_patterns":  totalPatterns,
	})
}

// GetFrameworkList 获取可用的框架列表（用于调试）
// @Summary 获取框架列表
// @Description 返回所有已初始化的框架名称列表
// @Tags Debug
// @Produce json
// @Success 200 {object} map[string]interface{} "框架列表"
// @Router /debug/frameworks [get]
func GetFrameworkList(c *gin.Context) {
	frameworks := logs.GetFrameworkList()

	c.JSON(http.StatusOK, gin.H{
		"frameworks": frameworks,
		"total":      len(frameworks),
	})
}

// GetPatternMatcherByFramework 获取指定框架的 pattern matcher 信息（用于调试）
// @Summary 获取指定框架的 Pattern Matcher
// @Description 返回指定框架的 pattern matcher 详细信息
// @Tags Debug
// @Produce json
// @Param framework path string true "框架名称"
// @Success 200 {object} map[string]interface{} "Pattern matcher 信息"
// @Failure 404 {object} map[string]interface{} "框架不存在"
// @Router /debug/pattern-matchers/{framework} [get]
func GetPatternMatcherByFramework(c *gin.Context) {
	framework := c.Param("framework")

	if framework == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "framework parameter is required",
		})
		return
	}

	matchersInfo := logs.GetPatternMatchersInfo()
	info, exists := matchersInfo[framework]

	if !exists {
		availableFrameworks := logs.GetFrameworkList()
		c.JSON(http.StatusNotFound, gin.H{
			"error":                "framework not found",
			"framework":           framework,
			"available_frameworks": availableFrameworks,
		})
		return
	}

	logrus.Debugf("Returning pattern matcher info for framework %s with %d patterns",
		framework, info.TotalPatterns)

	c.JSON(http.StatusOK, gin.H{
		"framework": framework,
		"info":      info,
	})
}

