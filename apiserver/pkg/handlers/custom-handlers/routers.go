/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"fmt"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func InitCustomRouters(e *gin.Engine, h *Handler) {
	group := e.Group(common.PrimusRouterCustomRootPath, authority.Authorize(), authority.Prepare())
	{
		group.POST("workloads", h.CreateWorkload)
		group.GET("workloads", h.ListWorkload)
		group.GET(fmt.Sprintf("workloads/:%s", common.Name), h.GetWorkload)
		group.DELETE(fmt.Sprintf("workloads/:%s", common.Name), h.DeleteWorkload)
		group.POST(fmt.Sprintf("workloads/:%s/stop", common.Name), h.StopWorkload)
		group.PATCH(fmt.Sprintf("workloads/:%s", common.Name), h.PatchWorkload)
		group.GET(fmt.Sprintf("workloads/:%s/service", common.Name), h.GetWorkloadService)
		group.GET(fmt.Sprintf("workloads/:%s/pods/:%s/logs", common.Name, common.PodId), h.GetWorkloadPodLog)
		group.GET(fmt.Sprintf("workloads/:%s/pods/:%s/containers", common.Name, common.PodId), h.GetWorkloadPodContainers)

		group.POST("secrets", h.CreateSecret)
		group.GET("secrets", h.ListSecret)
		group.GET(fmt.Sprintf("secrets/:%s", common.Name), h.GetSecret)
		group.PATCH(fmt.Sprintf("secrets/:%s", common.Name), h.PatchSecret)
		group.DELETE(fmt.Sprintf("secrets/:%s", common.Name), h.DeleteSecret)

		group.GET("faults", h.ListFault)
		group.DELETE(fmt.Sprintf("faults/:%s", common.Name), h.DeleteFault)
		group.POST(fmt.Sprintf("faults/:%s/stop", common.Name), h.StopFault)

		group.POST("nodetemplates", h.CreateNodeTemplate)
		group.DELETE("nodetemplates/:name", h.DeleteNodeTemplate)
		group.GET("nodetemplates", h.ListNodeTemplate)

		group.POST("nodes", h.CreateNode)
		group.DELETE(fmt.Sprintf("nodes/:%s", common.Name), h.DeleteNode)
		group.GET(fmt.Sprintf("nodes/:%s/logs", common.Name), h.GetNodePodLog)
		group.GET("nodes", h.ListNode)
		group.GET(fmt.Sprintf("nodes/:%s", common.Name), h.GetNode)
		group.PATCH(fmt.Sprintf("nodes/:%s", common.Name), h.PatchNode)
		group.POST(fmt.Sprintf("nodes/:%s/restart", common.Name), h.RestartNode)

		group.POST("workspaces", h.CreateWorkspace)
		group.DELETE(fmt.Sprintf("workspaces/:%s", common.Name), h.DeleteWorkspace)
		group.PATCH(fmt.Sprintf("workspaces/:%s", common.Name), h.PatchWorkspace)
		group.GET(fmt.Sprintf("workspaces/:%s", common.Name), h.GetWorkspace)
		group.GET("workspaces", h.ListWorkspace)
		group.POST(fmt.Sprintf("workspaces/:%s/nodes", common.Name), h.ProcessWorkspaceNodes)

		group.POST("clusters", h.CreateCluster)
		group.POST(fmt.Sprintf("clusters/:%s/nodes", common.Name), h.ProcessClusterNodes)
		group.DELETE(fmt.Sprintf("clusters/:%s", common.Name), h.DeleteCluster)
		group.PATCH(fmt.Sprintf("clusters/:%s", common.Name), h.PatchCluster)
		group.GET(fmt.Sprintf("clusters/:%s/logs", common.Name), h.GetClusterPodLog)

		group.POST("nodeflavors", h.CreateNodeFlavor)
		group.DELETE(fmt.Sprintf("nodeflavors/:%s", common.Name), h.DeleteNodeFlavor)
		group.GET("nodeflavors", h.ListNodeFlavor)
		group.GET(fmt.Sprintf("nodeflavors/:%s", common.Name), h.GetNodeFlavor)
		group.GET(fmt.Sprintf("nodeflavors/:%s/avail", common.Name), h.GetNodeFlavorAvail)
		group.PATCH(fmt.Sprintf("nodeflavors/:%s", common.Name), h.PatchNodeFlavor)

		group.POST("opsjobs", h.CreateOpsJob)
		group.GET("opsjobs", h.ListOpsJob)
		group.GET("opsjobs/:name", h.GetOpsJob)
		group.DELETE("opsjobs/:name", h.DeleteOpsJob)

		group.DELETE("users/:name", h.DeleteUser)
		group.GET("users", h.ListUser)
		group.PATCH("users/:name", h.PatchUser)
		group.GET("users/:name", h.GetUser)

		group.POST(fmt.Sprintf("service/:%s/logs", common.Name), h.GetServiceLog)
		group.POST(fmt.Sprintf("workloads/:%s/logs", common.Name), h.GetWorkloadLog)
		group.POST(fmt.Sprintf("workloads/:%s/logs/:%s/context", common.Name, types.DocId), h.GetWorkloadLogContext)

		group.POST("publickeys", h.CreatePublicKey)
		group.DELETE("publickeys/:id", h.DeletePublicKey)
		group.PATCH("publickeys/:id/status", h.SetPublicKeyStatus)
		group.PATCH("publickeys/:id/description", h.SetPublicKeyDescription)
		group.GET("publickeys", h.ListPublicKeys)
	}

	noAuthGroup := e.Group(common.PrimusRouterCustomRootPath, authority.Prepare())
	{
		noAuthGroup.GET("clusters", h.ListCluster)
		noAuthGroup.GET(fmt.Sprintf("clusters/:%s", common.Name), h.GetCluster)

		noAuthGroup.POST(fmt.Sprintf("login"), h.Login)
		noAuthGroup.POST(fmt.Sprintf("logout"), h.Logout)

		noAuthGroup.GET(fmt.Sprintf("/envs"), h.GetEnvs)
		noAuthGroup.POST("users", h.CreateUser)
	}
}
