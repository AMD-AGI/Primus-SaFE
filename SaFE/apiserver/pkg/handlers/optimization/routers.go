/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middleware"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// InitRoutes wires Model Optimization endpoints onto the Gin engine. The
// handler may be nil (feature disabled at startup); in that case the routes
// are skipped entirely so stale clients receive a 404 rather than a panic.
func InitRoutes(engine *gin.Engine, handler *Handler) {
	if handler == nil {
		klog.Info("model optimization: handler is nil, routes not registered")
		return
	}

	// All routes sit under /api/v1/optimization and inherit the same auth +
	// preprocessing middleware used by Model Square, Playground, etc.
	group := engine.Group(
		common.PrimusRouterCustomRootPath+"/optimization",
		middleware.Authorize(),
		middleware.Preprocess(),
	)
	{
		group.POST("/tasks", middleware.Audit("optimization"), handler.CreateTask)
		group.POST("/tasks/batch", middleware.Audit("optimization"), handler.BatchCreateTasks)
		group.GET("/tasks", handler.ListTasks)
		group.GET("/tasks/:id", handler.GetTask)
		group.GET("/tasks/:id/artifacts", handler.ListArtifacts)
		group.GET("/tasks/:id/artifacts/download", handler.DownloadArtifact)
		group.POST("/tasks/:id/interrupt", middleware.Audit("optimization"), handler.InterruptTask)
		group.POST("/tasks/:id/retry", middleware.Audit("optimization"), handler.RetryTask)
		group.POST("/tasks/:id/apply", middleware.Audit("optimization"), handler.ApplyTask)
		group.DELETE("/tasks/:id", middleware.Audit("optimization"), handler.DeleteTask)
	}

	// SSE endpoint is on a separate group without Audit (high frequency) and
	// without request body size limits from Preprocess.
	stream := engine.Group(
		common.PrimusRouterCustomRootPath+"/optimization",
		middleware.Authorize(),
	)
	{
		stream.GET("/tasks/:id/events", handler.StreamEvents)
	}

	klog.Info("model optimization: routes registered successfully")
}
