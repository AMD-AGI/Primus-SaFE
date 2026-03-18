// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package robust

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
)

// RobustFacade wraps a local Facade and overrides data-plane sub-facades
// with Robust API-backed implementations. Non-data-plane methods (GitHub,
// TraceLens, Detection, etc.) fall through to the embedded local facade.
type RobustFacade struct {
	*database.Facade
	client      *Client
	clusterName string
}

func NewRobustFacade(localFacade *database.Facade, client *Client, clusterName string) *RobustFacade {
	return &RobustFacade{
		Facade:      localFacade,
		client:      client,
		clusterName: clusterName,
	}
}

// WithCluster returns a new RobustFacade for the specified cluster.
func (f *RobustFacade) WithCluster(clusterName string) database.FacadeInterface {
	return &RobustFacade{
		Facade:      f.Facade,
		client:      f.client,
		clusterName: clusterName,
	}
}

// GetClient returns the underlying Robust HTTP client.
func (f *RobustFacade) GetClient() *Client {
	return f.client
}

// Data-plane facade overrides will be added here as adapter implementations
// are completed. For now, all methods fall through to the local facade.
//
// Example (to be implemented per facade):
//
//   func (f *RobustFacade) GetNode() database.NodeFacadeInterface {
//       return NewRobustNodeAdapter(f.client)
//   }
//
// The gradual migration pattern:
// 1. Override GetNode() → RobustNodeAdapter (nodes domain migrated)
// 2. Override GetWorkload() → RobustWorkloadAdapter (workloads domain migrated)
// 3. Override GetPod() → RobustPodAdapter (pods domain migrated)
// 4. ... continue per domain
//
// Domains that stay local (fall through to embedded Facade):
// - GitHub Workflow, TraceLens, Perfetto, Detection, SystemConfig, etc.
