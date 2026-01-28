// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

func init() {
	// System Configuration endpoints (GET only)
	unified.Register(&unified.EndpointDef[SysConfigListRequest, []dbmodel.SystemConfig]{
		Name:        "sysconfig_list",
		Description: "List all system configurations with optional category filter",
		HTTPMethod:  "GET",
		HTTPPath:    "/system-config",
		MCPToolName: "lens_sysconfig_list",
		Handler:     handleSysConfigList,
	})

	unified.Register(&unified.EndpointDef[SysConfigGetRequest, dbmodel.SystemConfig]{
		Name:        "sysconfig_get",
		Description: "Get a specific system configuration by key",
		HTTPMethod:  "GET",
		HTTPPath:    "/system-config/:key",
		MCPToolName: "lens_sysconfig_get",
		Handler:     handleSysConfigGet,
	})

	unified.Register(&unified.EndpointDef[SysConfigHistoryRequest, []dbmodel.SystemConfigHistory]{
		Name:        "sysconfig_history",
		Description: "Get history of changes for a specific configuration key",
		HTTPMethod:  "GET",
		HTTPPath:    "/system-config/:key/history",
		MCPToolName: "lens_sysconfig_history",
		Handler:     handleSysConfigHistory,
	})
}

// ======================== Request Types ========================

type SysConfigListRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Category string `json:"category" form:"category" mcp:"description=Filter by category"`
}

type SysConfigGetRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Key     string `json:"key" form:"key" param:"key" binding:"required" mcp:"description=Configuration key,required"`
}

type SysConfigHistoryRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Key     string `json:"key" form:"key" param:"key" binding:"required" mcp:"description=Configuration key,required"`
}

// ======================== Handler Implementations ========================

func handleSysConfigList(ctx context.Context, req *SysConfigListRequest) (*[]dbmodel.SystemConfig, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	mgr := config.NewManagerForCluster(clients.ClusterName)

	var filters []config.ListFilter
	if req.Category != "" {
		filters = append(filters, config.WithCategoryFilter(req.Category))
	}

	configs, err := mgr.List(ctx, filters...)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list configs", errors.CodeDatabaseError)
	}

	return &configs, nil
}

func handleSysConfigGet(ctx context.Context, req *SysConfigGetRequest) (*dbmodel.SystemConfig, error) {
	if req.Key == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("key is required")
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	mgr := config.NewManagerForCluster(clients.ClusterName)
	cfg, err := mgr.GetRaw(ctx, req.Key)
	if err != nil {
		return nil, errors.WrapError(err, "config not found", errors.RequestDataNotExisted)
	}

	return cfg, nil
}

func handleSysConfigHistory(ctx context.Context, req *SysConfigHistoryRequest) (*[]dbmodel.SystemConfigHistory, error) {
	if req.Key == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("key is required")
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	mgr := config.NewManagerForCluster(clients.ClusterName)
	history, err := mgr.GetHistory(ctx, req.Key, 20) // Last 20 versions
	if err != nil {
		return nil, errors.WrapError(err, "failed to get config history", errors.CodeDatabaseError)
	}

	return &history, nil
}
