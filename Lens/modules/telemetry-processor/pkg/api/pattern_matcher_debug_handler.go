// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/logs"
)

// GetPatternMatchers returns debug information about the global pattern registry
// @Summary Get Global Pattern Registry Info
// @Description Returns information about all loaded patterns in the global registry
// @Tags Debug
// @Produce json
// @Success 200 {object} map[string]interface{} "Pattern registry information"
// @Router /debug/pattern-matchers [get]
func GetPatternMatchers(c *gin.Context) {
	info := logs.GetGlobalRegistryDebugInfo()

	c.JSON(http.StatusOK, gin.H{
		"global_registry": info,
	})
}

// GetFrameworkList returns the global pattern registry info
// @Summary Get Pattern Registry Info
// @Description Returns the global pattern registry debug information
// @Tags Debug
// @Produce json
// @Success 200 {object} map[string]interface{} "Pattern registry info"
// @Router /debug/frameworks [get]
func GetFrameworkList(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"global_registry": logs.GetGlobalRegistryDebugInfo(),
	})
}

// GetPatternMatcherByFramework is kept for API compatibility but returns global registry info
// @Summary Get Pattern Info
// @Description Returns global pattern registry debug information
// @Tags Debug
// @Produce json
// @Param framework path string true "Framework name (informational)"
// @Success 200 {object} map[string]interface{} "Pattern info"
// @Router /debug/pattern-matchers/{framework} [get]
func GetPatternMatcherByFramework(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"global_registry": logs.GetGlobalRegistryDebugInfo(),
	})
}
