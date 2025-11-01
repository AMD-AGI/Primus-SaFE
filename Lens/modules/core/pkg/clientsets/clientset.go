package clientsets

import (
	"context"
)

// InitClientSets initializes all client sets through ClusterManager
// ClusterManager will handle the initialization of K8S and Storage clients
func InitClientSets(ctx context.Context, multiCluster bool) error {
	return InitClusterManager(ctx, multiCluster)
}
