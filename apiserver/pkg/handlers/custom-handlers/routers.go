/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func InitCustomRouters(e *gin.Engine, h *Handler) {
	group := e.Group(common.PrimusRouterCustomRootPath, Prepare())
	{
		group.POST("workloads", h.CreateWorkload)
		group.GET("workloads", h.ListWorkload)
		group.GET(fmt.Sprintf("workloads/:%s", types.Name), h.GetWorkload)
		group.DELETE(fmt.Sprintf("workloads/:%s", types.Name), h.DeleteWorkload)
		group.POST(fmt.Sprintf("workloads/:%s/stop", types.Name), h.StopWorkload)
		group.PATCH(fmt.Sprintf("workloads/:%s", types.Name), h.PatchWorkload)
		group.GET(fmt.Sprintf("workloads/:%s/service", types.Name), h.GetWorkloadService)
		group.GET(fmt.Sprintf("workloads/:%s/pods/:%s/logs", types.Name, types.PodId), h.GetWorkloadPodLog)

		group.POST("secrets", h.CreateSecret)
		group.GET("secrets", h.ListSecret)
		group.DELETE(fmt.Sprintf("secrets/:%s", types.Name), h.DeleteSecret)

		group.GET("faults", h.ListFault)

		group.POST("nodetemplates", h.CreateNodeTemplate)
		group.DELETE("nodetemplates/:name", h.DeleteNodeTemplate)
		group.GET("nodetemplates", h.ListNodeTemplate)

		group.POST("nodes", h.CreateNode)
		group.DELETE(fmt.Sprintf("nodes/:%s", types.Name), h.DeleteNode)
		group.GET(fmt.Sprintf("nodes/:%s/logs", types.Name), h.GetNodePodLog)
		group.GET("nodes", h.ListNode)
		group.GET(fmt.Sprintf("nodes/:%s", types.Name), h.GetNode)
		group.PATCH(fmt.Sprintf("nodes/:%s", types.Name), h.PatchNode)
		group.POST(fmt.Sprintf("nodes/:%s/restart", types.Name), h.RestartNode)

		group.POST("workspaces", h.CreateWorkspace)
		group.DELETE(fmt.Sprintf("workspaces/:%s", types.Name), h.DeleteWorkspace)
		group.PATCH(fmt.Sprintf("workspaces/:%s", types.Name), h.PatchWorkspace)
		group.GET(fmt.Sprintf("workspaces/:%s", types.Name), h.GetWorkspace)
		group.GET("workspaces", h.ListWorkspace)
		group.POST(fmt.Sprintf("workspaces/:%s/nodes", types.Name), h.ProcessWorkspaceNodes)

		group.POST("clusters", h.CreateCluster)
		group.POST(fmt.Sprintf("clusters/:%s/nodes", types.Name), h.ProcessClusterNodes)
		group.DELETE(fmt.Sprintf("clusters/:%s", types.Name), h.DeleteCluster)
		group.PATCH(fmt.Sprintf("clusters/:%s", types.Name), h.PatchCluster)
		group.GET(fmt.Sprintf("clusters/:%s/logs", types.Name), h.GetClusterPodLog)
		group.GET("clusters", h.ListCluster)
		group.GET(fmt.Sprintf("clusters/:%s", types.Name), h.GetCluster)

		group.POST("nodeflavors", h.CreateNodeFlavor)
		group.DELETE(fmt.Sprintf("nodeflavors/:%s", types.Name), h.DeleteNodeFlavor)
		group.GET("nodeflavors", h.ListNodeFlavor)
		group.GET(fmt.Sprintf("nodeflavors/:%s", types.Name), h.GetNodeFlavor)
		group.GET(fmt.Sprintf("nodeflavors/:%s/avail", types.Name), h.GetNodeFlavorAvail)

		group.POST("opsjobs", h.CreateOpsJob)
		group.GET("opsjobs", h.ListOpsJob)
		group.GET("opsjobs/:name", h.GetOpsJob)
		group.DELETE("opsjobs/:name", h.DeleteOpsJob)

		group.POST("users", h.CreateUser)
		group.DELETE("users/:name", h.DeleteUser)
		group.GET("users", h.ListUser)
		group.PATCH("users/:name", h.PatchUser)
		group.GET("users/:name", h.GetUser)

		group.POST(fmt.Sprintf("service/:%s/logs", types.Name), h.ListServiceLog)
		group.POST(fmt.Sprintf("workloads/:%s/logs", types.Name), h.ListWorkloadLog)
		group.POST(fmt.Sprintf("workloads/:%s/logs/:%s/context", types.Name, types.DocId), h.ListWorkloadLogContext)
	}

	noAuthGroup := e.Group(common.PrimusRouterCustomRootPath)
	{
		noAuthGroup.POST(fmt.Sprintf("login"), h.Login)
		noAuthGroup.POST(fmt.Sprintf("logout"), h.Logout)
	}
}
