/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package routers

import (
	"fmt"

	"github.com/gin-gonic/gin"

	customhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func initCustomRouters(e *gin.Engine, h *customhandler.Handler) {
	group := e.Group(common.PrimusRouterCustomRootPath, customhandler.Prepare())
	{
		group.POST("workloads", h.CreateWorkload)
		group.GET("workloads", h.ListWorkload)
		group.GET("workloads/:name", h.GetWorkload)
		group.DELETE("workloads/:name", h.DeleteWorkload)
		group.PATCH("workloads/:name", h.PatchWorkload)
		group.GET("workloads/:name/service", h.GetWorkloadService)
		group.GET(fmt.Sprintf("workloads/:name/pods/:%s/logs", types.PodId), h.GetWorkloadPodLog)

		group.POST("secrets", h.CreateSecret)
		group.GET("secrets", h.ListSecret)
		group.DELETE("secrets/:name", h.DeleteSecret)

		group.POST("nodes", h.CreateNode)
		group.DELETE("nodes/:name", h.DeleteNode)
		group.GET("nodes/:name/logs", h.GetNodePodLog)
		group.GET("nodes", h.ListNode)
		group.GET("nodes/:name", h.GetNode)
		group.PATCH("nodes/:name", h.PatchNode)

		group.POST("workspaces", h.CreateWorkspace)
		group.DELETE("workspaces/:name", h.DeleteWorkspace)
		group.PATCH("workspaces/:name", h.PatchWorkspace)
		group.GET("workspaces/:name", h.GetWorkspace)
		group.GET("workspaces", h.ListWorkspace)

		group.POST("clusters", h.CreateCluster)
		group.POST("clusters/:name/nodes/add", h.AddClusterNodes)
		group.POST("clusters/:name/nodes/remove", h.RemoveClusterNodes)
		group.DELETE("clusters/:name", h.DeleteCluster)
		group.PATCH("clusters/:name", h.PatchCluster)
		group.GET("clusters/:name/logs", h.GetClusterPodLog)
		group.GET("clusters", h.ListCluster)
		group.GET("clusters/:name", h.GetCluster)

		group.POST("nodeflavors", h.CreateNodeFlavor)
		group.DELETE("nodeflavors/:name", h.DeleteNodeFlavor)
		group.GET("nodeflavors", h.ListNodeFlavor)
		group.GET("nodeflavors/:name", h.GetNodeFlavor)
		group.GET("nodeflavors/:name/avail", h.GetNodeFlavorAvail)
	}
}
