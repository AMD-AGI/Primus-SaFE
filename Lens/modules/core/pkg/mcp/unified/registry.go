// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// Registry holds all endpoint definitions and provides methods to register them
// with HTTP (Gin) and MCP servers.
type Registry struct {
	mu        sync.RWMutex
	endpoints []EndpointRegistration
	mcpTools  []*MCPTool
}

var (
	globalRegistry     *Registry
	globalRegistryOnce sync.Once
)

// GetRegistry returns the global registry singleton.
func GetRegistry() *Registry {
	globalRegistryOnce.Do(func() {
		globalRegistry = &Registry{
			endpoints: make([]EndpointRegistration, 0),
			mcpTools:  make([]*MCPTool, 0),
		}
	})
	return globalRegistry
}

// Register registers an endpoint definition to the global registry.
// This function is generic and accepts EndpointDef with any Req/Resp types.
func Register[Req, Resp any](def *EndpointDef[Req, Resp]) {
	GetRegistry().RegisterEndpoint(def)
}

// RegisterEndpoint adds an endpoint definition to the registry.
func (r *Registry) RegisterEndpoint(ep EndpointRegistration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.endpoints = append(r.endpoints, ep)

	// Pre-generate MCP tool if applicable
	if tool := ep.GetMCPTool(); tool != nil {
		r.mcpTools = append(r.mcpTools, tool)
	}

	log.Infof("Registered endpoint: %s (HTTP: %s %s, MCP: %s)",
		ep.GetName(),
		ep.GetHTTPMethod(),
		ep.GetHTTPPath(),
		ep.GetMCPToolName())
}

// InitGinRoutes registers all unified endpoints to a Gin router group.
func (r *Registry) InitGinRoutes(group *gin.RouterGroup) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, ep := range r.endpoints {
		// Skip MCP-only endpoints
		if ep.IsMCPOnly() {
			continue
		}

		handler := ep.GetGinHandler()
		if handler == nil {
			log.Warnf("Endpoint %s has no HTTP handler, skipping", ep.GetName())
			continue
		}

		method := ep.GetHTTPMethod()
		path := ep.GetHTTPPath()

		switch method {
		case "GET":
			group.GET(path, handler)
		case "POST":
			group.POST(path, handler)
		case "PUT":
			group.PUT(path, handler)
		case "DELETE":
			group.DELETE(path, handler)
		case "PATCH":
			group.PATCH(path, handler)
		case "HEAD":
			group.HEAD(path, handler)
		case "OPTIONS":
			group.OPTIONS(path, handler)
		case "Any", "ANY", "any":
			group.Any(path, handler)
		default:
			log.Warnf("Unknown HTTP method %s for endpoint %s", method, ep.GetName())
		}

		log.Infof("Registered HTTP route: %s %s -> %s", method, path, ep.GetName())
	}

	return nil
}

// GetMCPTools returns all registered MCP tools.
func (r *Registry) GetMCPTools() []*MCPTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification
	tools := make([]*MCPTool, len(r.mcpTools))
	copy(tools, r.mcpTools)
	return tools
}

// GetEndpoints returns all registered endpoints.
func (r *Registry) GetEndpoints() []EndpointRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification
	eps := make([]EndpointRegistration, len(r.endpoints))
	copy(eps, r.endpoints)
	return eps
}

// GetEndpointByName returns an endpoint by its name.
func (r *Registry) GetEndpointByName(name string) EndpointRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, ep := range r.endpoints {
		if ep.GetName() == name {
			return ep
		}
	}
	return nil
}

// GetMCPToolByName returns an MCP tool by its name.
func (r *Registry) GetMCPToolByName(name string) *MCPTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, tool := range r.mcpTools {
		if tool.Name == name {
			return tool
		}
	}
	return nil
}

// Clear removes all registered endpoints. Useful for testing.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.endpoints = make([]EndpointRegistration, 0)
	r.mcpTools = make([]*MCPTool, 0)
}

// Count returns the number of registered endpoints.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.endpoints)
}

// MCPToolCount returns the number of registered MCP tools.
func (r *Registry) MCPToolCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.mcpTools)
}
