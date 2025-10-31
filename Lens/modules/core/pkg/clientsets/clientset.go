package clientsets

import (
	"context"
)

func InitClientSets(ctx context.Context, multiCluster bool) error {
	err := initK8SClientSets(ctx, multiCluster)
	if err != nil {
		return err
	}
	err = initStorageClientSets(ctx, multiCluster)
	if err != nil {
		return err
	}
	return nil
}
