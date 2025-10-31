/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"fmt"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middle"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// InitCustomRouters initializes and registers all custom API routes with the Gin engine.
// It sets up two route groups: authenticated routes (requiring authorization and preprocessing) and
// public routes (requiring only preprocessing). Each group includes endpoints for managing
// workloads, secrets, faults, nodes, workspaces, clusters, users, flavors, jobs, logs, and public keys.
func InitCustomRouters(e *gin.Engine, h *Handler) {
	// Custom API requires authentication and preprocessing.
	group := e.Group(common.PrimusRouterCustomRootPath, middle.Authorize(), middle.Preprocess())
	{
		group.POST("workloads", h.CreateWorkload)
		group.POST("workloads/clone", h.CloneWorkloads)
		group.POST("workloads/delete", h.DeleteWorkloads)
		group.POST("workloads/stop", h.StopWorkloads)
		group.POST(fmt.Sprintf("workloads/:%s/stop", common.Name), h.StopWorkload)
		group.DELETE(fmt.Sprintf("workloads/:%s", common.Name), h.DeleteWorkload)
		group.PATCH(fmt.Sprintf("workloads/:%s", common.Name), h.PatchWorkload)
		group.GET("workloads", h.ListWorkload)
		group.GET(fmt.Sprintf("workloads/:%s", common.Name), h.GetWorkload)
		group.GET(fmt.Sprintf("workloads/:%s/service", common.Name), h.GetWorkloadService)
		group.GET(fmt.Sprintf("workloads/:%s/pods/:%s/logs", common.Name, common.PodId), h.GetWorkloadPodLog)
		group.GET(fmt.Sprintf("workloads/:%s/pods/:%s/containers", common.Name, common.PodId), h.GetWorkloadPodContainers)

		group.POST("secrets", h.CreateSecret)
		group.DELETE(fmt.Sprintf("secrets/:%s", common.Name), h.DeleteSecret)
		group.PATCH(fmt.Sprintf("secrets/:%s", common.Name), h.PatchSecret)
		group.GET("secrets", h.ListSecret)
		group.GET(fmt.Sprintf("secrets/:%s", common.Name), h.GetSecret)

		group.POST(fmt.Sprintf("faults/:%s/stop", common.Name), h.StopFault)
		group.DELETE(fmt.Sprintf("faults/:%s", common.Name), h.DeleteFault)
		group.GET("faults", h.ListFault)

		group.POST("nodetemplates", h.CreateNodeTemplate)
		group.DELETE(fmt.Sprintf("nodetemplates/:%s", common.Name), h.DeleteNodeTemplate)
		group.GET("nodetemplates", h.ListNodeTemplate)

		group.POST("nodes", h.CreateNode)
		group.POST("nodes/delete", h.DeleteNodes)
		group.DELETE(fmt.Sprintf("nodes/:%s", common.Name), h.DeleteNode)
		group.PATCH(fmt.Sprintf("nodes/:%s", common.Name), h.PatchNode)
		group.GET(fmt.Sprintf("nodes/:%s/logs", common.Name), h.GetNodePodLog)
		group.GET(fmt.Sprintf("nodes/:%s/reboot/logs", common.Name), h.ListNodeRebootLog)
		group.GET("nodes", h.ListNode)
		group.GET(fmt.Sprintf("nodes/:%s", common.Name), h.GetNode)
		group.GET("nodes/export", h.ExportNode)

		group.POST("workspaces", h.CreateWorkspace)
		group.POST(fmt.Sprintf("workspaces/:%s/nodes", common.Name), h.ProcessWorkspaceNodes)
		group.DELETE(fmt.Sprintf("workspaces/:%s", common.Name), h.DeleteWorkspace)
		group.PATCH(fmt.Sprintf("workspaces/:%s", common.Name), h.PatchWorkspace)
		group.GET(fmt.Sprintf("workspaces/:%s", common.Name), h.GetWorkspace)
		group.GET("workspaces", h.ListWorkspace)

		group.POST("clusters", h.CreateCluster)
		group.POST(fmt.Sprintf("clusters/:%s/nodes", common.Name), h.ProcessClusterNodes)
		group.DELETE(fmt.Sprintf("clusters/:%s", common.Name), h.DeleteCluster)
		group.PATCH(fmt.Sprintf("clusters/:%s", common.Name), h.PatchCluster)
		group.GET(fmt.Sprintf("clusters/:%s/logs", common.Name), h.GetClusterPodLog)

		group.POST("nodeflavors", h.CreateNodeFlavor)
		group.DELETE(fmt.Sprintf("nodeflavors/:%s", common.Name), h.DeleteNodeFlavor)
		group.PATCH(fmt.Sprintf("nodeflavors/:%s", common.Name), h.PatchNodeFlavor)
		group.GET("nodeflavors", h.ListNodeFlavor)
		group.GET(fmt.Sprintf("nodeflavors/:%s", common.Name), h.GetNodeFlavor)
		group.GET(fmt.Sprintf("nodeflavors/:%s/avail", common.Name), h.GetNodeFlavorAvail)

		group.POST("opsjobs", h.CreateOpsJob)
		group.POST(fmt.Sprintf("opsjobs/:%s/stop", common.Name), h.StopOpsJob)
		group.DELETE(fmt.Sprintf("opsjobs/:%s", common.Name), h.DeleteOpsJob)
		group.GET("opsjobs", h.ListOpsJob)
		group.GET(fmt.Sprintf("opsjobs/:%s", common.Name), h.GetOpsJob)

		group.DELETE(fmt.Sprintf("users/:%s", common.Name), h.DeleteUser)
		group.PATCH(fmt.Sprintf("users/:%s", common.Name), h.PatchUser)
		group.GET("users", h.ListUser)
		group.GET(fmt.Sprintf("users/:%s", common.Name), h.GetUser)

		group.POST(fmt.Sprintf("service/:%s/logs", common.Name), h.GetServiceLog)
		group.POST(fmt.Sprintf("workloads/:%s/logs", common.Name), h.GetWorkloadLog)
		group.POST(fmt.Sprintf("workloads/:%s/logs/:%s/context", common.Name, types.DocId), h.GetWorkloadLogContext)

		group.POST("publickeys", h.CreatePublicKey)
		group.DELETE("publickeys/:id", h.DeletePublicKey)
		group.PATCH("publickeys/:id/status", h.SetPublicKeyStatus)
		group.PATCH("publickeys/:id/description", h.SetPublicKeyDescription)
		group.GET("publickeys", h.ListPublicKeys)

		group.GET(fmt.Sprintf("clusters/:%s/addons", common.Name), h.ListAddon)
		group.POST(fmt.Sprintf("clusters/:%s/addons", common.Name), h.CreateAddon)
		group.DELETE(fmt.Sprintf("clusters/:%s/addons/:%s", common.Name, common.AddonName), h.DeleteAddon)
		group.PATCH(fmt.Sprintf("clusters/:%s/addons/:%s", common.Name, common.AddonName), h.PatchAddon)
		group.GET(fmt.Sprintf("clusters/:%s/addons/:%s", common.Name, common.AddonName), h.GetAddon)

		group.GET("addontemplates", h.ListAddonTemplate)
		group.GET(fmt.Sprintf("addontemplates/:%s", common.Name), h.GetAddonTemplate)
	}

	// Custom API without authentication
	noAuthGroup := e.Group(common.PrimusRouterCustomRootPath, middle.Preprocess())
	{
		noAuthGroup.GET("clusters", h.ListCluster)
		noAuthGroup.GET(fmt.Sprintf("clusters/:%s", common.Name), h.GetCluster)

		noAuthGroup.POST(fmt.Sprintf("login"), h.Login)
		noAuthGroup.POST(fmt.Sprintf("logout"), h.Logout)

		noAuthGroup.POST("users", h.CreateUser)

		noAuthGroup.GET(fmt.Sprintf("/envs"), h.GetEnvs)
	}
}
