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

	// Note: Unified endpoints are registered in router.go via getUnifiedHandler()
	// The unified.GetRegistry().InitGinRoutes() should NOT be called here
	// to avoid duplicate route registration
	log.Infof("Unified registry has %d endpoints available for MCP", unified.GetRegistry().Count())

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
	// Note: GetMCPTools() returns current tools at this moment
	// The health/info endpoints will dynamically get the latest tools
	tools := unified.GetRegistry().GetMCPTools()
	server.RegisterTools(tools)
	log.Infof("MCP Server: Registered %d tools from unified registry", len(tools))
	log.Infof("MCP Server: Total endpoints in registry: %d", unified.GetRegistry().Count())

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

		// Health check for MCP - dynamically get tools count
		mcpGroup.GET("/health", func(c *gin.Context) {
			currentTools := unified.GetRegistry().GetMCPTools()
			c.JSON(200, gin.H{
				"status":    "ok",
				"server":    "Lens MCP Server",
				"version":   mcpserver.MCPVersion,
				"tools":     len(currentTools),
				"endpoints": unified.GetRegistry().Count(),
			})
		})

		// MCP info endpoint - dynamically get tools
		mcpGroup.GET("/", func(c *gin.Context) {
			currentTools := unified.GetRegistry().GetMCPTools()
			toolList := make([]gin.H, 0, len(currentTools))
			for _, tool := range currentTools {
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
				"endpoints":    unified.GetRegistry().Count(),
			})
		})
	}

	log.Infof("MCP Server: Routes registered under %s", basePath)
	log.Infof("MCP Server: SSE endpoint: %s/sse", basePath)
	log.Infof("MCP Server: RPC endpoint: %s/rpc (for testing)", basePath)
}
