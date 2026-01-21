// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package server

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	mcpserver "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/gin-gonic/gin"
)

func InitServer(ctx context.Context) error {
	return InitServerWithPreInitFunc(ctx, nil)
}

func InitServerWithPreInitFunc(ctx context.Context, preInit func(ctx context.Context, cfg *config.Config) error) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	err = clientsets.InitClientSets(ctx, cfg.MultiCluster, cfg.LoadK8SClient, cfg.LoadStorageClient)
	if err != nil {
		return err
	}
	if preInit != nil {
		err := preInit(ctx, cfg)
		if err != nil {
			return errors.NewError().WithCode(errors.CodeInitializeError).WithMessage("PreInit Error").WithError(err)
		}
	}
	ginEngine := gin.New()
	ginEngine.Use(gin.Recovery())
	err = router.InitRouter(ginEngine, cfg)
	if err != nil {
		return err
	}

	// Initialize unified API routes (for endpoints using the new framework)
	// This registers any endpoints that were defined using unified.Register()
	v1Group := ginEngine.Group("/v1")
	if err := unified.GetRegistry().InitGinRoutes(v1Group); err != nil {
		log.Warnf("Failed to init unified routes: %v", err)
	} else {
		log.Infof("Initialized %d unified endpoints", unified.GetRegistry().Count())
	}

	// Initialize MCP server routes under /mcp path if enabled
	if cfg.IsMCPEnabled() {
		initMCPRoutes(ginEngine, cfg)
	}

	err = controller.InitControllers(ctx, *cfg)
	if err != nil {
		return err
	}
	InitHealthServer(cfg.HttpPort + 1)

	err = ginEngine.Run(fmt.Sprintf(":%d", cfg.HttpPort))
	if err != nil {
		return err
	}
	return nil
}

// initMCPRoutes initializes MCP server routes under /mcp path
func initMCPRoutes(engine *gin.Engine, cfg *config.Config) {
	// Create MCP server
	server := mcpserver.New()

	// Register tools from unified registry
	tools := unified.GetRegistry().GetMCPTools()
	server.RegisterTools(tools)
	log.Infof("MCP Server: Registered %d tools from unified registry", len(tools))

	// Set instructions
	if instructions := cfg.GetMCPInstructions(); instructions != "" {
		server.SetInstructions(instructions)
	} else {
		server.SetInstructions("Lens API Server - GPU Cluster Management Tools via MCP")
	}

	// Create SSE transport
	sseTransport := mcpserver.NewSSETransport(server)

	// Create simple HTTP transport for testing
	streamableTransport := mcpserver.NewStreamableHTTPTransport(server)

	// Register MCP routes under configured base path (default: /mcp)
	basePath := cfg.GetMCPBasePath()
	mcpGroup := engine.Group(basePath)
	{
		// SSE endpoint for establishing SSE connection
		mcpGroup.GET("/sse", gin.WrapH(sseTransport.Handler()))

		// Message endpoint for sending messages via SSE session
		mcpGroup.POST("/message", gin.WrapH(sseTransport.Handler()))

		// Simple RPC endpoint for testing (no SSE required)
		mcpGroup.POST("/rpc", gin.WrapH(streamableTransport.Handler()))

		// Health check for MCP
		mcpGroup.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":  "ok",
				"server":  "Lens MCP Server",
				"version": mcpserver.MCPVersion,
				"tools":   len(tools),
			})
		})

		// MCP info endpoint
		mcpGroup.GET("/", func(c *gin.Context) {
			toolList := make([]gin.H, 0, len(tools))
			for _, tool := range tools {
				toolList = append(toolList, gin.H{
					"name":        tool.Name,
					"description": tool.Description,
				})
			}
			c.JSON(200, gin.H{
				"server":       "Lens MCP Server",
				"version":      mcpserver.MCPVersion,
				"sse_endpoint": basePath + "/sse",
				"rpc_endpoint": basePath + "/rpc",
				"tools":        toolList,
			})
		})
	}

	log.Infof("MCP Server: Routes registered under %s", basePath)
	log.Infof("MCP Server: SSE endpoint: %s/sse", basePath)
	log.Infof("MCP Server: RPC endpoint: %s/rpc (for testing)", basePath)
}
