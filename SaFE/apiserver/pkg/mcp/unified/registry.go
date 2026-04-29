// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"sync"

	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// Registry holds all endpoint definitions for HTTP and MCP registration.
type Registry struct {
	mu        sync.RWMutex
	endpoints []EndpointRegistration
	mcpTools  []*mcpserver.MCPTool
}

var (
	globalRegistry     *Registry
	globalRegistryOnce sync.Once
)

func GetRegistry() *Registry {
	globalRegistryOnce.Do(func() {
		globalRegistry = &Registry{
			endpoints: make([]EndpointRegistration, 0),
			mcpTools:  make([]*mcpserver.MCPTool, 0),
		}
	})
	return globalRegistry
}

// Register registers an endpoint definition to the global registry.
func Register[Req, Resp any](def *EndpointDef[Req, Resp]) {
	GetRegistry().RegisterEndpoint(def)
}

func (r *Registry) RegisterEndpoint(ep EndpointRegistration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.endpoints = append(r.endpoints, ep)

	if tool := ep.GetMCPTool(); tool != nil {
		r.mcpTools = append(r.mcpTools, tool)
	}

	klog.Infof("Registered endpoint: %s (HTTP: %s %s, MCP: %s)",
		ep.GetName(), ep.GetHTTPMethod(), ep.GetHTTPPath(), ep.GetMCPToolName())
}

// InitGinRoutes registers all unified endpoints to a Gin router group.
func (r *Registry) InitGinRoutes(group *gin.RouterGroup) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, ep := range r.endpoints {
		if ep.IsMCPOnly() {
			continue
		}
		handler := ep.GetGinHandler()
		if handler == nil {
			klog.Warningf("Endpoint %s has no HTTP handler, skipping", ep.GetName())
			continue
		}

		method := ep.GetHTTPMethod()
		path := ep.GetHTTPPath()

		registered := true
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
			registered = false
			klog.Warningf("Unknown HTTP method %s for endpoint %s", method, ep.GetName())
		}
		if registered {
			klog.Infof("Registered HTTP route: %s %s -> %s", method, path, ep.GetName())
		}
	}
	return nil
}

func (r *Registry) GetMCPTools() []*mcpserver.MCPTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]*mcpserver.MCPTool, len(r.mcpTools))
	copy(tools, r.mcpTools)
	return tools
}

func (r *Registry) GetMCPToolsByGroup(group string) []*mcpserver.MCPTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []*mcpserver.MCPTool
	for _, ep := range r.endpoints {
		if ep.GetGroup() == group {
			if tool := ep.GetMCPTool(); tool != nil {
				filtered = append(filtered, tool)
			}
		}
	}
	return filtered
}

func (r *Registry) GetEndpoints() []EndpointRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	eps := make([]EndpointRegistration, len(r.endpoints))
	copy(eps, r.endpoints)
	return eps
}

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

func (r *Registry) GetMCPToolByName(name string) *mcpserver.MCPTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, tool := range r.mcpTools {
		if tool.Name == name {
			return tool
		}
	}
	return nil
}

func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endpoints = make([]EndpointRegistration, 0)
	r.mcpTools = make([]*mcpserver.MCPTool, 0)
}

func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.endpoints)
}

func (r *Registry) MCPToolCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.mcpTools)
}

func (r *Registry) GetEndpointByPath(path string) EndpointRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ep := range r.endpoints {
		if ep.GetHTTPPath() == path {
			return ep
		}
	}
	return nil
}

func (r *Registry) GetEndpointByMethodAndPath(method, path string) EndpointRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ep := range r.endpoints {
		if ep.GetHTTPPath() == path && ep.GetHTTPMethod() == method {
			return ep
		}
	}
	for _, ep := range r.endpoints {
		if ep.GetHTTPPath() == path {
			return ep
		}
	}
	return nil
}
