// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/storage"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// ============================================================================
// Storage Endpoints
// ============================================================================

type StorageStatRequest struct {
	Cluster string `json:"cluster" mcp:"desc=Cluster name (optional)"`
}

type StorageStatResponse struct {
	*model.StorageStat
}

func handleStorageStat(ctx context.Context, req *StorageStatRequest) (*StorageStatResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	stats, err := storage.GetStorageStatWithClientSet(ctx, clients.StorageClientSet)
	if err != nil {
		return nil, err
	}

	return &StorageStatResponse{StorageStat: stats}, nil
}

// ============================================================================
// Unified Registration
// ============================================================================

func init() {
	unified.Register(&unified.EndpointDef[StorageStatRequest, StorageStatResponse]{
		HTTPPath:    "/storage/stat",
		HTTPMethod:  "GET",
		MCPToolName: "lens_storage_stat",
		Description: "Get storage statistics including capacity, usage, and available space",
		Handler:     handleStorageStat,
	})
}
