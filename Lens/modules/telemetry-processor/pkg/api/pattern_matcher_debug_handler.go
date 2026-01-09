// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/logs"
)

// GetPatternMatchers gets pattern matcher information for all frameworks (for debugging)
// @Summary Get Pattern Matcher List
// @Description Returns detailed information about all initialized pattern matchers, including various matching patterns
// @Tags Debug
// @Produce json
// @Success 200 {object} map[string]interface{} "Pattern matcher information list"
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

// GetFrameworkList gets available framework list (for debugging)
// @Summary Get Framework List
// @Description Returns list of all initialized framework names
// @Tags Debug
// @Produce json
// @Success 200 {object} map[string]interface{} "Framework list"
// @Router /debug/frameworks [get]
func GetFrameworkList(c *gin.Context) {
	frameworks := logs.GetFrameworkList()

	c.JSON(http.StatusOK, gin.H{
		"frameworks": frameworks,
		"total":      len(frameworks),
	})
}

// GetPatternMatcherByFramework gets pattern matcher information for specified framework (for debugging)
// @Summary Get Pattern Matcher for Specified Framework
// @Description Returns detailed pattern matcher information for specified framework
// @Tags Debug
// @Produce json
// @Param framework path string true "Framework name"
// @Success 200 {object} map[string]interface{} "Pattern matcher information"
// @Failure 404 {object} map[string]interface{} "Framework does not exist"
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

