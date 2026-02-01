/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"fmt"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middleware"
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// InitCustomRouters initializes and registers all custom API routes with the Gin engine.
// Write operations have audit middleware added individually for clarity.
func InitCustomRouters(e *gin.Engine, h *Handler) {
	authGroup := e.Group(common.PrimusRouterCustomRootPath, middleware.Authorize(), middleware.Preprocess())
	{
		// ==================== Workloads ====================
		workloads := authGroup.Group("/workloads")
		{
			workloads.POST("", middleware.Audit("workload"), h.CreateWorkload)
			workloads.POST("/clone", middleware.Audit("workload", "clone"), h.CloneWorkloads)
			workloads.POST("/delete", middleware.Audit("workload", "delete"), h.DeleteWorkloads)
			workloads.POST("/stop", middleware.Audit("workload", "stop"), h.StopWorkloads)
			workloads.POST(fmt.Sprintf("/:%s/stop", common.Name), middleware.Audit("workload", "stop"), h.StopWorkload)
			workloads.DELETE(fmt.Sprintf("/:%s", common.Name), middleware.Audit("workload"), h.DeleteWorkload)
			workloads.PATCH(fmt.Sprintf("/:%s", common.Name), middleware.Audit("workload"), h.PatchWorkload)
			workloads.GET("", h.ListWorkload)
			workloads.GET(fmt.Sprintf("/:%s", common.Name), h.GetWorkload)
			workloads.GET(fmt.Sprintf("/:%s/service", common.Name), h.GetWorkloadService)
			workloads.GET(fmt.Sprintf("/:%s/pods/:%s/logs", common.Name, common.PodId), h.GetWorkloadPodLog)
			workloads.GET(fmt.Sprintf("/:%s/pods/:%s/containers", common.Name, common.PodId), h.GetWorkloadPodContainers)
			// POST but read-only (log query uses POST for complex body)
			workloads.POST(fmt.Sprintf("/:%s/logs", common.Name), h.GetWorkloadLog)
			workloads.POST(fmt.Sprintf("/:%s/logs/:%s/context", common.Name, view.DocId), h.GetWorkloadLogContext)
		}

		// ==================== Secrets ====================
		secrets := authGroup.Group("/secrets")
		{
			secrets.POST("", middleware.Audit("secret"), h.CreateSecret)
			secrets.DELETE(fmt.Sprintf("/:%s", common.Name), middleware.Audit("secret"), h.DeleteSecret)
			secrets.PATCH(fmt.Sprintf("/:%s", common.Name), middleware.Audit("secret"), h.PatchSecret)
			secrets.GET("", h.ListSecret)
			secrets.GET(fmt.Sprintf("/:%s", common.Name), h.GetSecret)
		}

		// ==================== Faults ====================
		faults := authGroup.Group("/faults")
		{
			faults.POST(fmt.Sprintf("/:%s/stop", common.Name), middleware.Audit("fault", "stop"), h.StopFault)
			faults.DELETE(fmt.Sprintf("/:%s", common.Name), middleware.Audit("fault"), h.DeleteFault)
			faults.GET("", h.ListFault)
		}

		// ==================== Node Templates ====================
		nodeTemplates := authGroup.Group("/nodetemplates")
		{
			nodeTemplates.POST("", middleware.Audit("nodetemplate"), h.CreateNodeTemplate)
			nodeTemplates.DELETE(fmt.Sprintf("/:%s", common.Name), middleware.Audit("nodetemplate"), h.DeleteNodeTemplate)
			nodeTemplates.GET("", h.ListNodeTemplate)
		}

		// ==================== Nodes ====================
		nodes := authGroup.Group("/nodes")
		{
			nodes.POST("", middleware.Audit("node"), h.CreateNode)
			nodes.POST("/delete", middleware.Audit("node", "delete"), h.DeleteNodes)
			nodes.POST("/retry", middleware.Audit("node", "retry"), h.RetryNodes)
			nodes.DELETE(fmt.Sprintf("/:%s", common.Name), middleware.Audit("node"), h.DeleteNode)
			nodes.PATCH(fmt.Sprintf("/:%s", common.Name), middleware.Audit("node"), h.PatchNode)
			nodes.GET(fmt.Sprintf("/:%s/logs", common.Name), h.GetNodePodLog)
			nodes.GET(fmt.Sprintf("/:%s/reboot/logs", common.Name), h.ListNodeRebootLog)
			nodes.GET("", h.ListNode)
			nodes.GET(fmt.Sprintf("/:%s", common.Name), h.GetNode)
			nodes.GET("/export", h.ExportNode)
		}

		// ==================== Workspaces ====================
		workspaces := authGroup.Group("/workspaces")
		{
			workspaces.POST("", middleware.Audit("workspace"), h.CreateWorkspace)
			workspaces.POST(fmt.Sprintf("/:%s/nodes", common.Name), middleware.Audit("workspace", "process-nodes"), h.ProcessWorkspaceNodes)
			workspaces.DELETE(fmt.Sprintf("/:%s", common.Name), middleware.Audit("workspace"), h.DeleteWorkspace)
			workspaces.PATCH(fmt.Sprintf("/:%s", common.Name), middleware.Audit("workspace"), h.PatchWorkspace)
			workspaces.GET(fmt.Sprintf("/:%s", common.Name), h.GetWorkspace)
			workspaces.GET("", h.ListWorkspace)
		}

		// ==================== Clusters ====================
		clusters := authGroup.Group("/clusters")
		{
			clusters.POST("", middleware.Audit("cluster"), h.CreateCluster)
			clusters.POST(fmt.Sprintf("/:%s/nodes", common.Name), middleware.Audit("cluster", "process-nodes"), h.ProcessClusterNodes)
			clusters.DELETE(fmt.Sprintf("/:%s", common.Name), middleware.Audit("cluster"), h.DeleteCluster)
			clusters.PATCH(fmt.Sprintf("/:%s", common.Name), middleware.Audit("cluster"), h.PatchCluster)
			clusters.GET(fmt.Sprintf("/:%s/logs", common.Name), h.GetClusterPodLog)
			clusters.GET(fmt.Sprintf("/:%s/addons", common.Name), h.ListAddon)
			clusters.POST(fmt.Sprintf("/:%s/addons", common.Name), middleware.Audit("addon"), h.CreateAddon)
			clusters.DELETE(fmt.Sprintf("/:%s/addons/:%s", common.Name, common.AddonName), middleware.Audit("addon"), h.DeleteAddon)
			clusters.PATCH(fmt.Sprintf("/:%s/addons/:%s", common.Name, common.AddonName), middleware.Audit("addon"), h.PatchAddon)
			clusters.GET(fmt.Sprintf("/:%s/addons/:%s", common.Name, common.AddonName), h.GetAddon)
		}

		// ==================== Node Flavors ====================
		nodeFlavors := authGroup.Group("/nodeflavors")
		{
			nodeFlavors.POST("", middleware.Audit("nodeflavor"), h.CreateNodeFlavor)
			nodeFlavors.DELETE(fmt.Sprintf("/:%s", common.Name), middleware.Audit("nodeflavor"), h.DeleteNodeFlavor)
			nodeFlavors.PATCH(fmt.Sprintf("/:%s", common.Name), middleware.Audit("nodeflavor"), h.PatchNodeFlavor)
			nodeFlavors.GET("", h.ListNodeFlavor)
			nodeFlavors.GET(fmt.Sprintf("/:%s", common.Name), h.GetNodeFlavor)
			nodeFlavors.GET(fmt.Sprintf("/:%s/avail", common.Name), h.GetNodeFlavorAvail)
		}

		// ==================== Ops Jobs ====================
		opsJobs := authGroup.Group("/opsjobs")
		{
			opsJobs.POST("", middleware.Audit("opsjob"), h.CreateOpsJob)
			opsJobs.POST(fmt.Sprintf("/:%s/stop", common.Name), middleware.Audit("opsjob", "stop"), h.StopOpsJob)
			opsJobs.DELETE(fmt.Sprintf("/:%s", common.Name), middleware.Audit("opsjob"), h.DeleteOpsJob)
			opsJobs.GET("", h.ListOpsJob)
			opsJobs.GET(fmt.Sprintf("/:%s", common.Name), h.GetOpsJob)
		}

		// ==================== Users ====================
		users := authGroup.Group("/users")
		{
			users.DELETE(fmt.Sprintf("/:%s", common.Name), middleware.Audit("user"), h.DeleteUser)
			users.PATCH(fmt.Sprintf("/:%s", common.Name), middleware.Audit("user"), h.PatchUser)
			users.GET("", h.ListUser)
			users.GET(fmt.Sprintf("/:%s", common.Name), h.GetUser)
		}

		// ==================== Public Keys ====================
		publicKeys := authGroup.Group("/publickeys")
		{
			publicKeys.POST("", middleware.Audit("publickey"), h.CreatePublicKey)
			publicKeys.DELETE("/:id", middleware.Audit("publickey"), h.DeletePublicKey)
			publicKeys.PATCH("/:id/status", middleware.Audit("publickey"), h.SetPublicKeyStatus)
			publicKeys.PATCH("/:id/description", middleware.Audit("publickey"), h.SetPublicKeyDescription)
			publicKeys.GET("", h.ListPublicKeys)
		}

		// ==================== API Keys ====================
		apiKeys := authGroup.Group("/apikeys")
		{
			apiKeys.POST("", middleware.Audit("apikey"), h.CreateApiKey)
			apiKeys.DELETE("/:id", middleware.Audit("apikey"), h.DeleteApiKey)
			apiKeys.GET("", h.ListApiKey)
			apiKeys.GET("/current", h.GetCurrentApiKey)
		}

		// ==================== Persistent Volumes ====================
		persistentvolumes := authGroup.Group("/persistentvolumes")
		{
			persistentvolumes.GET("", h.ListPersistentVolume)
		}

		authGroup.GET("/addontemplates", h.ListAddonTemplate)
		authGroup.GET(fmt.Sprintf("/addontemplates/:%s", common.Name), h.GetAddonTemplate)

		// ==================== Service Logs (read-only, POST for complex query body) ====================
		authGroup.POST(fmt.Sprintf("/service/:%s/logs", common.Name), h.GetServiceLog)

		// ==================== Audit Logs (read-only) ====================
		authGroup.GET("/auditlogs", h.ListAuditLog)

		// ==================== Logout ====================
		authGroup.POST("/logout", middleware.Audit("auth", "logout"), h.Logout)
	}

	// Public routes without authentication
	noAuthGroup := e.Group(common.PrimusRouterCustomRootPath, middleware.Preprocess())
	{
		noAuthGroup.GET("/clusters", h.ListCluster)
		noAuthGroup.GET(fmt.Sprintf("/clusters/:%s", common.Name), h.GetCluster)
		noAuthGroup.POST("/login", middleware.Audit("auth", "login"), h.Login)
		noAuthGroup.POST("/users", middleware.Audit("user", "register"), h.CreateUser)
		noAuthGroup.GET("/envs", h.GetEnvs)
		noAuthGroup.POST("/auth/verify", authority.VerifyToken)
	}
}
