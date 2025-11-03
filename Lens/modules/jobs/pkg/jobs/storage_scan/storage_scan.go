package storage_scan

import (
	"context"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
)

type StorageScanJob struct {
}

func (s *StorageScanJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) error {
	scanner := &Scanner{Targets: []ClusterTarget{
		{
			Name:       "K8S",
			ClientSets: clientSets,
			Extra:      nil,
		},
	}}
	result, err := scanner.Run(ctx)
	if err != nil {
		return err
	}
	for _, report := range result {
		for _, item := range report.BackendItems {
			dbItem := &dbModel.Storage{
				Name: item.BackendName,
				Kind: string(item.BackendKind),
				Config: map[string]interface{}{
					"meta_secret": item.MetaSecret,
				},
				Source:    "scan",
				Status:    string(item.Health),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			existDbItem, err := database.GetFacade().GetStorage().GetStorageByKindAndName(ctx, dbItem.Kind, dbItem.Name)
			if err != nil {
				log.Errorf("Fail to get storage %s/%s: %v", dbItem.Kind, dbItem.Name, err)
				continue
			}
			if existDbItem != nil {
				dbItem.ID = existDbItem.ID
				dbItem.CreatedAt = existDbItem.CreatedAt
				err = database.GetFacade().GetStorage().UpdateStorage(ctx, dbItem)
				if err != nil {
					log.Errorf("Fail to update storage %s/%s: %v", dbItem.Kind, dbItem.Name, err)
					continue
				}
				log.Infof("Storage %s/%s updated", dbItem.Kind, dbItem.Name)
			} else {
				err = database.GetFacade().GetStorage().CreateStorage(ctx, dbItem)
				if err != nil {
					log.Errorf("Fail to create storage %s/%s: %v", dbItem.Kind, dbItem.Name, err)
					continue
				}
				log.Infof("Storage %s/%s created", dbItem.Kind, dbItem.Name)
			}

		}
	}
	return nil
}

func (s *StorageScanJob) Schedule() string {
	return "@every 1m"
}
