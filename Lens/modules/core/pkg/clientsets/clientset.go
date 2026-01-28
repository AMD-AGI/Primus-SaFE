// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"context"
)

// InitClientSets initializes all client sets through ClusterManager
// Deprecated: Use InitClusterManager with ComponentDeclaration instead
// This function is kept for backward compatibility
func InitClientSets(ctx context.Context, isControlPlane bool, loadK8SClient bool, loadStorageClient bool) error {
	decl := ComponentDeclaration{
		RequireK8S:     loadK8SClient,
		RequireStorage: loadStorageClient,
	}
	if isControlPlane {
		decl.Type = ComponentTypeControlPlane
	} else {
		decl.Type = ComponentTypeDataPlane
	}
	return InitClusterManager(ctx, decl)
}
